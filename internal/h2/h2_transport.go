package h2

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"

	"github.com/denuitt1/mhr-cfw-go/internal/codec"
	"github.com/denuitt1/mhr-cfw-go/internal/logger"
)

var log = logger.Get("H2")

type Transport struct {
	connectHost string
	verifySSL   bool
	sniHosts    []string
	sniIdx      uint32

	mu     sync.Mutex
	h2     *http2.Transport
	client *http.Client
}

func New(connectHost string, sniHosts []string, verifySSL bool) *Transport {
	if len(sniHosts) == 0 {
		sniHosts = []string{"www.google.com"}
	}
	return &Transport{
		connectHost: connectHost,
		verifySSL:   verifySSL,
		sniHosts:    sniHosts,
	}
}

func (t *Transport) ensure() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.h2 != nil && t.client != nil {
		return
	}
	tr := &http2.Transport{
		AllowHTTP: false,
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			sni := t.nextSNI()
			tlsCfg := &tls.Config{
				ServerName:         sni,
				InsecureSkipVerify: !t.verifySSL,
				NextProtos:         []string{"h2", "http/1.1"},
			}
			dialer := &net.Dialer{Timeout: 15 * time.Second}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(t.connectHost, "443"))
			if err != nil {
				return nil, err
			}
			if tcp, ok := conn.(*net.TCPConn); ok {
				_ = tcp.SetNoDelay(true)
			}
			tlsConn := tls.Client(conn, tlsCfg)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, err
			}
			if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
				_ = tlsConn.Close()
				return nil, errors.New("h2 ALPN negotiation failed")
			}
			return tlsConn, nil
		},
	}
	client := &http.Client{Transport: tr}
	t.h2 = tr
	t.client = client
	log.Infof("H2 transport ready -> %s", t.connectHost)
}

func (t *Transport) Request(ctx context.Context, method, path, host string, headers map[string]string, body []byte, timeout time.Duration) (int, map[string]string, []byte, error) {
	t.ensure()
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := &url.URL{Scheme: "https", Host: host, Path: path}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("accept-encoding", codec.SupportedEncodings())
	req.Host = host

	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, err
	}
	respHeaders := map[string]string{}
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[strings.ToLower(k)] = v[0]
		}
	}
	if enc := respHeaders["content-encoding"]; enc != "" {
		data = codec.Decode(data, enc)
	}
	return resp.StatusCode, respHeaders, data, nil
}

func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.h2 != nil {
		t.h2.CloseIdleConnections()
	}
	t.h2 = nil
	t.client = nil
	return nil
}

func (t *Transport) nextSNI() string {
	idx := atomic.AddUint32(&t.sniIdx, 1)
	return t.sniHosts[int(idx)%len(t.sniHosts)]
}
