package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/denuitt1/mhr-cfw-go/internal/config"
	"github.com/denuitt1/mhr-cfw-go/internal/constants"
	"github.com/denuitt1/mhr-cfw-go/internal/fronter"
	"github.com/denuitt1/mhr-cfw-go/internal/logger"
	"github.com/denuitt1/mhr-cfw-go/internal/mitm"
)

var log = logger.Get("Proxy")

var maxAgeRegex = regexp.MustCompile(`max-age=(\d+)`)

type ResponseCache struct {
	mu     sync.Mutex
	store  map[string]cacheEntry
	order  []string
	size   int
	max    int
	Hits   int
	Misses int
}

type cacheEntry struct {
	raw     []byte
	expires time.Time
}

func NewResponseCache(maxMB int) *ResponseCache {
	return &ResponseCache{store: map[string]cacheEntry{}, order: []string{}, max: maxMB * 1024 * 1024}
}

func (c *ResponseCache) Get(url string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.store[url]
	if !ok {
		c.Misses++
		return nil
	}
	if time.Now().After(entry.expires) {
		c.size -= len(entry.raw)
		delete(c.store, url)
		for i, u := range c.order {
			if u == url {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
		c.Misses++
		return nil
	}
	c.Hits++
	return entry.raw
}

func (c *ResponseCache) Put(url string, raw []byte, ttl int) {
	if len(raw) == 0 {
		return
	}
	size := len(raw)
	if size > c.max/4 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.size+size > c.max && len(c.store) > 0 {
		oldURL := c.order[0]
		c.size -= len(c.store[oldURL].raw)
		delete(c.store, oldURL)
		c.order = c.order[1:]
	}
	if old, ok := c.store[url]; ok {
		for i, u := range c.order {
			if u == url {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
		c.size -= len(old.raw)
	}
	c.store[url] = cacheEntry{raw: raw, expires: time.Now().Add(time.Duration(ttl) * time.Second)}
	c.order = append(c.order, url)
	c.size += size
}

func (c *ResponseCache) ParseTTL(raw []byte, urlStr string) int {
	sep := []byte("\r\n\r\n")
	idx := bytes.Index(raw, sep)
	if idx < 0 {
		return 0
	}
	head := strings.ToLower(string(raw[:idx]))
	if !strings.HasPrefix(string(raw[:20]), "HTTP/1.1 200") {
		return 0
	}
	if strings.Contains(head, "no-store") || strings.Contains(head, "private") || strings.Contains(head, "set-cookie:") {
		return 0
	}
	if m := maxAgeRegex.FindStringSubmatch(head); len(m) == 2 {
		v, _ := strconv.Atoi(m[1])
		if v > constants.CacheTTLMax {
			return constants.CacheTTLMax
		}
		return v
	}
	path := strings.ToLower(strings.Split(urlStr, "?")[0])
	for _, ext := range constants.StaticExts {
		if strings.HasSuffix(path, ext) {
			return constants.CacheTTLStaticLong
		}
	}
	if strings.Contains(head, "image/") || strings.Contains(head, "font/") {
		return constants.CacheTTLStaticLong
	}
	if strings.Contains(head, "text/css") || strings.Contains(head, "javascript") {
		return constants.CacheTTLStaticMed
	}
	if strings.Contains(head, "text/html") || strings.Contains(head, "application/json") {
		return 0
	}
	return 0
}

type Server struct {
	host         string
	port         int
	socksEnabled bool
	socksHost    string
	socksPort    int

	fronter *fronter.DomainFronter
	mitm    *mitm.Manager
	cache   *ResponseCache

	directFailUntil map[string]time.Time
	mu              sync.Mutex

	servers []net.Listener
	wg      sync.WaitGroup
	ctx     context.Context
}

func (s *Server) Fronter() *fronter.DomainFronter {
	return s.fronter
}

func NewServer(cfg *config.Config) (*Server, error) {
	host := cfg.Config.ListenHost
	port := cfg.Config.ListenPort
	socksEnabled := cfg.Config.Socks5Enabled
	socksHost := cfg.Config.ListenHost
	socksPort := cfg.Config.Socks5Port
	if socksEnabled && socksHost == host && socksPort == port {
		return nil, fmt.Errorf("listen_port and socks5_port must differ on the same host (both set to %d on %s)", port, host)
	}

	return &Server{
		host:            host,
		port:            port,
		socksEnabled:    socksEnabled,
		socksHost:       socksHost,
		socksPort:       socksPort,
		fronter:         fronter.New(cfg),
		mitm:            mitm.NewManager(),
		cache:           NewResponseCache(constants.CacheMaxMB),
		directFailUntil: map[string]time.Time{},
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.ctx = ctx
	ln, err := net.Listen("tcp", net.JoinHostPort(s.host, strconv.Itoa(s.port)))
	if err != nil {
		return err
	}
	s.servers = append(s.servers, ln)
	log.Infof("HTTP proxy listening on %s:%d", s.host, s.port)

	if s.socksEnabled {
		socksLn, err := net.Listen("tcp", net.JoinHostPort(s.socksHost, strconv.Itoa(s.socksPort)))
		if err != nil {
			log.Errorf("SOCKS5 listener failed on %s:%d: %v", s.socksHost, s.socksPort, err)
		} else {
			s.servers = append(s.servers, socksLn)
			log.Infof("SOCKS5 proxy listening on %s:%d", s.socksHost, s.socksPort)
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.acceptLoop(socksLn, s.handleSocksConn)
			}()
		}
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ln, s.handleHTTPConn)
	}()

	<-ctx.Done()
	for _, l := range s.servers {
		_ = l.Close()
	}
	_ = s.fronter.Close()
	s.wg.Wait()
	log.Infof("Server stopped")
	return nil
}

func (s *Server) acceptLoop(ln net.Listener, handler func(net.Conn)) {
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			handler(conn)
		}()
	}
}

func (s *Server) handleHTTPConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	headers := []string{line}
	for {
		ln, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		headers = append(headers, ln)
		if ln == "\r\n" || ln == "\n" {
			break
		}
		if sumLen(headers) > constants.MaxHeaderBytes {
			return
		}
	}

	parts := strings.Split(strings.TrimSpace(line), " ")
	if len(parts) < 2 {
		return
	}
	method := strings.ToUpper(parts[0])
	if method == "CONNECT" {
		s.handleConnect(conn, reader, parts[1])
		return
	}
	s.handlePlainHTTP(conn, reader, headers)
}

func (s *Server) handleConnect(conn net.Conn, reader *bufio.Reader, target string) {
	host, port := splitHostPort(target, 443)
	log.Infof("CONNECT -> %s:%d", host, port)
	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	s.handleTunnel(host, port, conn, reader)
}

func (s *Server) handleTunnel(host string, port int, conn net.Conn, reader *bufio.Reader) {
	if port == 443 {
		cfg, err := s.mitm.GetServerTLSConfig(host)
		if err != nil {
			return
		}
		tlsConn := tls.Server(conn, cfg)
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		s.relayHTTPStream(host, port, tlsConn)
		return
	}
	s.relayHTTPStream(host, port, conn)
}

func (s *Server) relayHTTPStream(host string, port int, conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		conn.SetDeadline(time.Now().Add(time.Duration(constants.ClientIdleTimeout) * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if line == "\r\n" || line == "\n" {
			continue
		}
		headers := []string{line}
		for {
			ln, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			headers = append(headers, ln)
			if ln == "\r\n" || ln == "\n" {
				break
			}
			if sumLen(headers) > constants.MaxHeaderBytes {
				return
			}
		}
		method, path := parseRequestLine(line)
		body, err := readBody(reader, headers)
		if err != nil {
			return
		}
		headerMap := parseHeaders(headers[1:])

		urlStr := normalizeURL(host, port, path)
		log.Infof("MITM -> %s %s", method, urlStr)

		origin := headerValue(headerMap, "origin")
		acrMethod := headerValue(headerMap, "access-control-request-method")
		acrHeaders := headerValue(headerMap, "access-control-request-headers")
		if strings.ToUpper(method) == "OPTIONS" && acrMethod != "" {
			resp := corsPreflight(origin, acrMethod, acrHeaders)
			_, _ = conn.Write(resp)
			continue
		}

		response := s.fronter.Relay(method, urlStr, headerMap, body)
		if origin != "" {
			response = injectCORSHeaders(response, origin)
		}
		_, _ = conn.Write(response)
	}
}

func (s *Server) handlePlainHTTP(conn net.Conn, reader *bufio.Reader, headers []string) {
	method, path := parseRequestLine(headers[0])
	body, err := readBody(reader, headers)
	if err != nil {
		return
	}
	headerMap := parseHeaders(headers[1:])

	origin := headerValue(headerMap, "origin")
	acrMethod := headerValue(headerMap, "access-control-request-method")
	acrHeaders := headerValue(headerMap, "access-control-request-headers")
	if strings.ToUpper(method) == "OPTIONS" && acrMethod != "" {
		resp := corsPreflight(origin, acrMethod, acrHeaders)
		_, _ = conn.Write(resp)
		return
	}

	urlStr := path
	response := s.fronter.Relay(method, urlStr, headerMap, body)
	if origin != "" {
		response = injectCORSHeaders(response, origin)
	}
	_, _ = conn.Write(response)
}

func (s *Server) handleSocksConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}
	if buf[0] != 5 {
		return
	}
	methods := make([]byte, int(buf[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	conn.Write([]byte{0x05, 0x00})

	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil {
		return
	}
	if request[0] != 5 || request[1] != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	addrType := request[3]
	var host string
	switch addrType {
	case 0x01:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 0x03:
		ln := make([]byte, 1)
		if _, err := io.ReadFull(conn, ln); err != nil {
			return
		}
		name := make([]byte, int(ln[0]))
		if _, err := io.ReadFull(conn, name); err != nil {
			return
		}
		host = string(name)
	case 0x04:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	port := int(portBuf[0])<<8 | int(portBuf[1])

	log.Infof("SOCKS5 CONNECT -> %s:%d", host, port)
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	s.handleTunnel(host, port, conn, bufio.NewReader(conn))
}

func sumLen(lines []string) int {
	count := 0
	for _, l := range lines {
		count += len(l)
	}
	return count
}

func parseRequestLine(line string) (string, string) {
	parts := strings.Split(strings.TrimSpace(line), " ")
	if len(parts) < 2 {
		return "GET", "/"
	}
	return parts[0], parts[1]
}

func parseHeaders(lines []string) map[string]string {
	h := map[string]string{}
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r\n")
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		h[key] = val
	}
	return h
}

func readBody(reader *bufio.Reader, headers []string) ([]byte, error) {
	cl := 0
	for _, ln := range headers {
		if strings.HasPrefix(strings.ToLower(ln), "content-length:") {
			v := strings.TrimSpace(strings.TrimPrefix(ln, "Content-Length:"))
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return nil, errors.New("invalid Content-Length")
			}
			cl = n
		}
	}
	if cl > constants.MaxRequestBodyBytes {
		return nil, errors.New("request body too large")
	}
	if cl == 0 {
		return nil, nil
	}
	buf := make([]byte, cl)
	_, err := io.ReadFull(reader, buf)
	return buf, err
}

func normalizeURL(host string, port int, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	scheme := "http"
	if port == 443 {
		scheme = "https"
	}
	if port == 80 || port == 443 {
		return fmt.Sprintf("%s://%s%s", scheme, host, path)
	}
	return fmt.Sprintf("%s://%s:%d%s", scheme, host, port, path)
}

func headerValue(headers map[string]string, name string) string {
	for k, v := range headers {
		if strings.ToLower(k) == name {
			return v
		}
	}
	return ""
}

func corsPreflight(origin, acrMethod, acrHeaders string) []byte {
	allowOrigin := origin
	if allowOrigin == "" {
		allowOrigin = "*"
	}
	allowMethods := "GET, POST, PUT, DELETE, PATCH, OPTIONS"
	if acrMethod != "" {
		allowMethods = acrMethod + ", " + allowMethods
	}
	allowHeaders := acrHeaders
	if allowHeaders == "" {
		allowHeaders = "*"
	}
	resp := "HTTP/1.1 204 No Content\r\n" +
		"Access-Control-Allow-Origin: " + allowOrigin + "\r\n" +
		"Access-Control-Allow-Methods: " + allowMethods + "\r\n" +
		"Access-Control-Allow-Headers: " + allowHeaders + "\r\n" +
		"Access-Control-Allow-Credentials: true\r\n" +
		"Access-Control-Max-Age: 86400\r\n" +
		"Vary: Origin\r\n" +
		"Content-Length: 0\r\n\r\n"
	return []byte(resp)
}

func injectCORSHeaders(response []byte, origin string) []byte {
	sep := []byte("\r\n\r\n")
	idx := bytes.Index(response, sep)
	if idx < 0 {
		return response
	}
	head := string(response[:idx])
	body := response[idx+4:]
	lines := strings.Split(head, "\r\n")
	filtered := []string{}
	for _, ln := range lines {
		low := strings.ToLower(ln)
		if strings.HasPrefix(low, "access-control-") {
			continue
		}
		filtered = append(filtered, ln)
	}
	allowOrigin := origin
	if allowOrigin == "" {
		allowOrigin = "*"
	}
	filtered = append(filtered,
		"Access-Control-Allow-Origin: "+allowOrigin,
		"Access-Control-Allow-Credentials: true",
		"Access-Control-Allow-Methods: GET, POST, PUT, DELETE, PATCH, OPTIONS",
		"Access-Control-Allow-Headers: *",
		"Access-Control-Expose-Headers: *",
		"Vary: Origin",
	)
	newHead := strings.Join(filtered, "\r\n") + "\r\n\r\n"
	return append([]byte(newHead), body...)
}

func splitHostPort(target string, defPort int) (string, int) {
	if strings.Contains(target, ":") {
		parts := strings.Split(target, ":")
		if len(parts) >= 2 {
			port, _ := strconv.Atoi(parts[len(parts)-1])
			return strings.Join(parts[:len(parts)-1], ":"), port
		}
	}
	return target, defPort
}
