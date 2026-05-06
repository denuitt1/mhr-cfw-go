package web

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/denuitt1/mhr-cfw-go/internal/proxy"
)

const probeTimeout = 8 * time.Second

type ProbeResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type ProbeReport struct {
	Google ProbeResult `json:"google"`
	GAS    ProbeResult `json:"gas"`
	Worker ProbeResult `json:"worker"`
}

func probeGoogleFront(googleIP, sni string) ProbeResult {
	start := time.Now()
	dialer := &net.Dialer{Timeout: probeTimeout}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(googleIP, "443"))
	if err != nil {
		return ProbeResult{Error: classifyNetError(err.Error())}
	}
	defer conn.Close()
	tlsConn := tls.Client(conn, &tls.Config{ServerName: sni})
	_ = tlsConn.SetDeadline(time.Now().Add(probeTimeout))
	if err := tlsConn.Handshake(); err != nil {
		return ProbeResult{Error: "TLS handshake failed: " + err.Error()}
	}
	_ = tlsConn.Close()
	return ProbeResult{OK: true, LatencyMs: time.Since(start).Milliseconds()}
}

func probeGAS(srv *proxy.Server) ProbeResult {
	if srv == nil {
		return ProbeResult{Error: "Proxy is not running"}
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	body, err := srv.Fronter().ProbeGAS(ctx, probeTimeout)
	if err != nil {
		return ProbeResult{Error: classifyNetError(err.Error())}
	}
	if strings.Contains(body, "unauthorized") {
		return ProbeResult{OK: true, LatencyMs: time.Since(start).Milliseconds()}
	}
	return ProbeResult{Error: classifyGASBody(body)}
}

func probeWorker(srv *proxy.Server, probeURL, workerURL, upstreamURL string) ProbeResult {
	if workerURL != "" {
		return probeWorkerDirect(workerURL, upstreamURL)
	}
	if srv == nil {
		return ProbeResult{Error: "Proxy is not running"}
	}
	if probeURL == "" {
		return ProbeResult{Error: "probe_url is not configured"}
	}
	start := time.Now()
	status, errMsg := srv.Fronter().ProbeWorker(probeURL)
	latency := time.Since(start).Milliseconds()
	if status >= 200 && status < 400 {
		return ProbeResult{OK: true, LatencyMs: latency}
	}
	return ProbeResult{LatencyMs: latency, Error: classifyWorkerError(status, errMsg)}
}

func probeWorkerDirect(workerURL, upstreamURL string) ProbeResult {
	start := time.Now()
	status, body, err := httpGET(workerURL)
	if err != nil {
		return ProbeResult{LatencyMs: time.Since(start).Milliseconds(), Error: "worker: " + classifyNetError(err.Error())}
	}
	if status != 200 {
		return ProbeResult{LatencyMs: time.Since(start).Milliseconds(), Error: fmt.Sprintf("worker returned status %d", status)}
	}
	if !strings.Contains(string(body), "Relay is Active") {
		return ProbeResult{LatencyMs: time.Since(start).Milliseconds(), Error: "worker reachable but did not return the expected relay marker — check worker.js is deployed"}
	}
	if upstreamURL != "" {
		probeURL := stripPath(upstreamURL)
		ustatus, _, uerr := httpGET(probeURL)
		if uerr != nil {
			return ProbeResult{LatencyMs: time.Since(start).Milliseconds(), Error: "upstream forwarder: " + classifyNetError(uerr.Error())}
		}
		if ustatus < 200 || ustatus >= 400 {
			return ProbeResult{LatencyMs: time.Since(start).Milliseconds(), Error: fmt.Sprintf("upstream forwarder returned status %d", ustatus)}
		}
	}
	return ProbeResult{OK: true, LatencyMs: time.Since(start).Milliseconds()}
}

func httpGET(rawURL string) (int, []byte, error) {
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	return resp.StatusCode, body, nil
}

func stripPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Path = "/"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func classifyGASBody(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "Apps Script returned an empty response"
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") {
		return "Apps Script deployment not found — verify deployment_ids and that the script is deployed as a web app"
	}
	if strings.HasPrefix(trimmed, "{") {
		var data map[string]any
		if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
			if e, ok := data["e"].(string); ok && e != "" {
				return "Apps Script error: " + e
			}
		}
	}
	return "Unexpected Apps Script response: " + shortSnippet(trimmed)
}

func classifyWorkerError(status int, msg string) string {
	msg = strings.TrimSpace(msg)
	lower := strings.ToLower(msg)
	switch {
	case strings.HasPrefix(lower, "bad json"), strings.HasPrefix(lower, "no json"):
		return "Apps Script returned non-JSON — deployment is unreachable or misconfigured"
	case strings.HasPrefix(lower, "empty response"):
		return "Apps Script returned an empty response"
	case strings.Contains(lower, "relay error: unauthorized"):
		return "auth_key does not match Apps Script AUTH_KEY"
	case strings.Contains(lower, "self-fetch blocked"):
		return "Worker rejected target as self-fetch — check WORKER_URL constant in worker.js"
	case strings.Contains(lower, "self-forward blocked"):
		return "Worker rejected upstream forwarder as self-forward"
	case strings.Contains(lower, "loop detected"):
		return "Loop detected (x-relay-hop / x-fwd-hop) — request bounced back to the Worker"
	case strings.Contains(lower, "upstream forwarder failed"):
		return "Upstream forwarder unreachable: " + strings.TrimPrefix(msg, "upstream forwarder failed: ")
	case strings.Contains(lower, "exceeds cap"):
		return "Response exceeds max_response_body_bytes"
	case strings.HasPrefix(lower, "relay error:"):
		return "Apps Script error: " + strings.TrimSpace(strings.TrimPrefix(msg, "Relay error:"))
	case msg != "":
		return shortSnippet(msg)
	case status >= 500:
		return fmt.Sprintf("relay returned status %d", status)
	default:
		return fmt.Sprintf("target returned status %d", status)
	}
}

func classifyNetError(s string) string {
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "i/o timeout"), strings.Contains(lower, "deadline exceeded"):
		return "Connection timed out — check google_ip and that outbound 443 is open"
	case strings.Contains(lower, "no such host"):
		return "DNS lookup failed"
	case strings.Contains(lower, "connection refused"):
		return "Connection refused"
	case strings.Contains(lower, "network is unreachable"):
		return "Network unreachable"
	case strings.Contains(lower, "tls"), strings.Contains(lower, "x509"):
		return "TLS error: " + s
	default:
		return s
	}
}

func shortSnippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}
