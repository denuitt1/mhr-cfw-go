package fronter

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/denuitt1/mhr-cfw-go/internal/codec"
	"github.com/denuitt1/mhr-cfw-go/internal/config"
	"github.com/denuitt1/mhr-cfw-go/internal/constants"
	"github.com/denuitt1/mhr-cfw-go/internal/h2"
	"github.com/denuitt1/mhr-cfw-go/internal/logger"
)

var log = logger.Get("Fronter")

type HostStat struct {
	Requests       int
	CacheHits      int
	Bytes          int
	TotalLatencyNs int64
	Errors         int
}

type DomainFronter struct {
	connectHost string
	sniHost     string
	sniHosts    []string
	sniIdx      int
	httpHost    string
	scriptIDs   []string
	scriptIdx   int
	devAvail    bool

	parallelRelay int
	sidBlacklist  map[string]time.Time
	blacklistTTL  time.Duration

	perSite map[string]*HostStat

	authKey   string
	verifySSL bool
	relayTO   time.Duration
	tlsTO     time.Duration
	maxResp   int

	h2 *h2.Transport

	poolMu sync.Mutex
	pool   []pooledConn

	batchMu      sync.Mutex
	batchPending []batchItem
	batchTimer   *time.Timer

	coalesceMu sync.Mutex
	coalesce   map[string][]chan []byte

	statsStop chan struct{}
}

type pooledConn struct {
	conn    net.Conn
	created time.Time
}

type batchItem struct {
	payload map[string]any
	respCh  chan []byte
}

func New(cfg *config.Config) *DomainFronter {
	frontDomain := cfg.Config.FrontDomain
	fronts := buildSNIPool(frontDomain, constants.FrontSNIPoolGoogle)
	ids := cfg.Config.DeploymentIDs
	if len(ids) == 0 {
		ids = []string{""}
	}
	parallel := cfg.Config.ParallelRelay
	if parallel < 1 {
		parallel = 1
	}
	if parallel > len(ids) {
		parallel = len(ids)
	}

	f := &DomainFronter{
		connectHost:   cfg.Config.GoogleIP,
		sniHost:       frontDomain,
		sniHosts:      fronts,
		httpHost:      "script.google.com",
		scriptIDs:     ids,
		sidBlacklist:  map[string]time.Time{},
		blacklistTTL:  time.Duration(constants.ScriptBlacklistTTL * float64(time.Second)),
		perSite:       map[string]*HostStat{},
		authKey:       cfg.Config.AuthKey,
		verifySSL:     cfg.Config.VerifySSL,
		relayTO:       time.Duration(cfg.Config.RelayTimeout) * time.Second,
		tlsTO:         time.Duration(cfg.Config.TLSConnectTimeout) * time.Second,
		maxResp:       cfg.Config.MaxResponseBodyBytes,
		parallelRelay: parallel,
		coalesce:      map[string][]chan []byte{},
		statsStop:     make(chan struct{}),
	}

	if len(fronts) > 1 {
		log.Infof("SNI rotation pool (%d): %s", len(fronts), strings.Join(fronts, ", "))
	}
	if parallel > 1 {
		log.Infof("Fan-out relay: %d parallel Apps Script instances per request", parallel)
	}
	log.Infof("Response codecs: %s", codec.SupportedEncodings())

	f.h2 = h2.New(f.connectHost, f.sniHosts, f.verifySSL)
	go f.statsLoop()
	return f
}

func buildSNIPool(frontDomain string, overrides []string) []string {
	if len(overrides) > 0 {
		seen := map[string]bool{}
		out := []string{}
		for _, item := range overrides {
			host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(item), "."))
			if host != "" && !seen[host] {
				seen[host] = true
				out = append(out, host)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	fd := strings.ToLower(strings.TrimSuffix(frontDomain, "."))
	if strings.HasSuffix(fd, ".google.com") || fd == "google.com" {
		pool := []string{fd}
		for _, h := range constants.FrontSNIPoolGoogle {
			if h != fd {
				pool = append(pool, h)
			}
		}
		return pool
	}
	if fd == "" {
		return []string{"www.google.com"}
	}
	return []string{fd}
}

func (f *DomainFronter) Close() error {
	close(f.statsStop)
	if f.h2 != nil {
		_ = f.h2.Close()
	}
	f.poolMu.Lock()
	for _, pc := range f.pool {
		_ = pc.conn.Close()
	}
	f.pool = nil
	f.poolMu.Unlock()
	return nil
}

func (f *DomainFronter) Relay(method, urlStr string, headers map[string]string, body []byte) []byte {
	payload := f.buildPayload(method, urlStr, headers, body)
	start := time.Now()
	err := false
	var raw []byte
	defer func() {
		f.recordSite(urlStr, len(raw), time.Since(start), err)
	}()

	if f.isStatefulRequest(method, urlStr, headers, body) {
		resp, e := f.relaySingle(payload)
		if e != nil {
			err = true
			return f.errorResponse(502, e.Error())
		}
		return resp
	}

	key := f.coalesceKey(urlStr, headers)
	if strings.ToUpper(method) == "GET" && len(body) == 0 {
		if v := headerValue(headers, "range"); v == "" {
			if resp, ok := f.tryCoalesce(key, payload); ok {
				return resp
			}
		}
	}

	resp, e := f.batchSubmit(payload)
	if e != nil {
		err = true
		return f.errorResponse(502, e.Error())
	}
	return resp
}

func (f *DomainFronter) tryCoalesce(key string, payload map[string]any) ([]byte, bool) {
	f.coalesceMu.Lock()
	if waiters, ok := f.coalesce[key]; ok {
		ch := make(chan []byte, 1)
		f.coalesce[key] = append(waiters, ch)
		f.coalesceMu.Unlock()
		resp := <-ch
		return resp, true
	}
	f.coalesce[key] = []chan []byte{}
	f.coalesceMu.Unlock()

	resp, err := f.batchSubmit(payload)
	if err != nil {
		resp = f.errorResponse(502, err.Error())
	}

	f.coalesceMu.Lock()
	waiters := f.coalesce[key]
	delete(f.coalesce, key)
	f.coalesceMu.Unlock()
	for _, ch := range waiters {
		ch <- resp
	}
	return resp, true
}

func (f *DomainFronter) batchSubmit(payload map[string]any) ([]byte, error) {
	respCh := make(chan []byte, 1)
	item := batchItem{payload: payload, respCh: respCh}

	f.batchMu.Lock()
	f.batchPending = append(f.batchPending, item)
	if len(f.batchPending) >= constants.BatchMax {
		pending := f.batchPending
		f.batchPending = nil
		if f.batchTimer != nil {
			f.batchTimer.Stop()
			f.batchTimer = nil
		}
		f.batchMu.Unlock()
		go f.flushBatch(pending)
		return <-respCh, nil
	}
	if f.batchTimer == nil {
		f.batchTimer = time.AfterFunc(time.Duration(constants.BatchWindowMicro*float64(time.Second)), func() {
			f.batchMu.Lock()
			pending := f.batchPending
			f.batchPending = nil
			f.batchTimer = nil
			f.batchMu.Unlock()
			if len(pending) > 0 {
				f.flushBatch(pending)
			}
		})
	}
	f.batchMu.Unlock()
	return <-respCh, nil
}

func (f *DomainFronter) flushBatch(batch []batchItem) {
	if len(batch) == 1 {
		resp, err := f.relaySingle(batch[0].payload)
		if err != nil {
			resp = f.errorResponse(502, err.Error())
		}
		batch[0].respCh <- resp
		return
	}
	results, err := f.relayBatch(batch)
	if err != nil {
		for _, item := range batch {
			item.respCh <- f.errorResponse(502, err.Error())
		}
		return
	}
	for i, item := range batch {
		item.respCh <- results[i]
	}
}

func (f *DomainFronter) relaySingle(payload map[string]any) ([]byte, error) {
	full := map[string]any{}
	for k, v := range payload {
		full[k] = v
	}
	full["k"] = f.authKey
	jsonBody, _ := json.Marshal(full)
	path := f.execPath(payload["u"])

	_, _, body, err := f.h2.Request(context.Background(), "POST", path, f.httpHost, map[string]string{"content-type": "application/json"}, jsonBody, f.relayTO)
	if err == nil {
		return f.parseRelayResponse(body), nil
	}

	resp, err := f.relayHTTP1(path, jsonBody)
	if err != nil {
		return nil, err
	}
	return f.parseRelayResponse(resp), nil
}

func (f *DomainFronter) relayBatch(batch []batchItem) ([][]byte, error) {
	payloads := []map[string]any{}
	for _, item := range batch {
		payloads = append(payloads, item.payload)
	}
	full := map[string]any{
		"k": f.authKey,
		"q": payloads,
	}
	jsonBody, _ := json.Marshal(full)
	path := f.execPath(payloads[0]["u"])

	_, _, body, err := f.h2.Request(context.Background(), "POST", path, f.httpHost, map[string]string{"content-type": "application/json"}, jsonBody, 30*time.Second)
	if err == nil {
		return f.parseBatchBody(body, len(batch))
	}
	resp, err := f.relayHTTP1(path, jsonBody)
	if err != nil {
		return nil, err
	}
	return f.parseBatchBody(resp, len(batch))
}

func (f *DomainFronter) relayHTTP1(path string, body []byte) ([]byte, error) {
	conn, err := f.acquire()
	if err != nil {
		return nil, err
	}
	defer f.release(conn)

	req := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nAccept-Encoding: gzip\r\nConnection: keep-alive\r\n\r\n", path, f.httpHost, len(body))
	if _, err := conn.Write([]byte(req)); err != nil {
		return nil, err
	}
	if _, err := conn.Write(body); err != nil {
		return nil, err
	}

	status, headers, respBody, err := readHTTPResponse(conn, f.maxResp)
	if err != nil {
		return nil, err
	}

	if status >= 300 && status < 400 {
		loc := headers["location"]
		if loc != "" {
			parsed, _ := url.Parse(loc)
			rpath := parsed.Path
			if parsed.RawQuery != "" {
				rpath += "?" + parsed.RawQuery
			}
			return f.relayHTTP1(rpath, body)
		}
	}
	return respBody, nil
}

func readHTTPResponse(conn net.Conn, maxBody int) (int, map[string]string, []byte, error) {
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return 0, nil, nil, err
	}
	status := 0
	if m := regexp.MustCompile(`\d{3}`).FindString(statusLine); m != "" {
		status, _ = strconv.Atoi(m)
	}
	headers := map[string]string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return status, headers, nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
		}
	}

	cl := 0
	if v := headers["content-length"]; v != "" {
		cl, _ = strconv.Atoi(v)
	}
	body := []byte{}
	if cl > 0 {
		if cl > maxBody {
			return status, headers, nil, errors.New("response exceeds cap")
		}
		buf := make([]byte, cl)
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			return status, headers, nil, err
		}
		body = buf
	} else {
		buf, _ := io.ReadAll(reader)
		body = buf
	}
	if enc := headers["content-encoding"]; enc != "" {
		body = codec.Decode(body, enc)
	}
	return status, headers, body, nil
}

func (f *DomainFronter) acquire() (net.Conn, error) {
	f.poolMu.Lock()
	for len(f.pool) > 0 {
		pc := f.pool[len(f.pool)-1]
		f.pool = f.pool[:len(f.pool)-1]
		if time.Since(pc.created) < time.Duration(constants.ConnTTL*float64(time.Second)) {
			f.poolMu.Unlock()
			return pc.conn, nil
		}
		_ = pc.conn.Close()
	}
	f.poolMu.Unlock()

	dialer := &net.Dialer{Timeout: f.tlsTO}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(f.connectHost, "443"))
	if err != nil {
		return nil, err
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	tlsConn := tls.Client(conn, &tls.Config{ServerName: f.nextSNI(), InsecureSkipVerify: !f.verifySSL})
	err = tlsConn.Handshake()
	if err != nil {
		return nil, err
	}
	defer tlsConn.Close()
	return tlsConn, nil
}

func (f *DomainFronter) release(conn net.Conn) {
	f.poolMu.Lock()
	defer f.poolMu.Unlock()
	if len(f.pool) >= constants.PoolMax {
		_ = conn.Close()
		return
	}
	f.pool = append(f.pool, pooledConn{conn: conn, created: time.Now()})
}

func (f *DomainFronter) nextSNI() string {
	sni := f.sniHosts[f.sniIdx%len(f.sniHosts)]
	f.sniIdx++
	return sni
}

func (f *DomainFronter) execPath(urlOrHost any) string {
	sid := f.scriptIDForKey(hostKey(fmt.Sprint(urlOrHost)))
	if f.devAvail {
		return "/macros/s/" + sid + "/dev"
	}
	return "/macros/s/" + sid + "/exec"
}

func hostKey(urlOrHost string) string {
	if urlOrHost == "" {
		return ""
	}
	if strings.Contains(urlOrHost, "://") {
		parsed, err := url.Parse(urlOrHost)
		if err == nil {
			return strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
		}
	}
	return strings.ToLower(strings.TrimSuffix(urlOrHost, "."))
}

func (f *DomainFronter) scriptIDForKey(key string) string {
	if len(f.scriptIDs) == 1 {
		return f.scriptIDs[0]
	}
	if key == "" {
		f.scriptIdx = (f.scriptIdx + 1) % len(f.scriptIDs)
		return f.scriptIDs[f.scriptIdx]
	}
	h := sha1.Sum([]byte(key))
	idx := int(h[0]) % len(f.scriptIDs)
	return f.scriptIDs[idx]
}

func (f *DomainFronter) buildPayload(method, urlStr string, headers map[string]string, body []byte) map[string]any {
	p := map[string]any{
		"m": method,
		"u": urlStr,
		"r": false,
	}
	if headers != nil {
		p["h"] = headers
	}
	if len(body) > 0 {
		p["b"] = base64.StdEncoding.EncodeToString(body)
		if ct := headerValue(headers, "content-type"); ct != "" {
			p["ct"] = ct
		}
	}
	return p
}

func (f *DomainFronter) parseRelayResponse(body []byte) []byte {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return f.errorResponse(502, "Empty response from relay")
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		m := regexp.MustCompile(`\{.*\}`).FindString(text)
		if m == "" {
			return f.errorResponse(502, "No JSON: "+truncate(text, 200))
		}
		if err := json.Unmarshal([]byte(m), &data); err != nil {
			return f.errorResponse(502, "Bad JSON: "+truncate(text, 200))
		}
	}
	return f.parseRelayJSON(data)
}

func (f *DomainFronter) errorResponse(status int, message string) []byte {
	body := fmt.Sprintf("<html><body><h1>%d</h1><p>%s</p></body></html>", status, message)
	resp := fmt.Sprintf("HTTP/1.1 %d Error\r\nContent-Type: text/html\r\nContent-Length: %d\r\n\r\n%s", status, len(body), body)
	return []byte(resp)
}

func (f *DomainFronter) parseRelayJSON(data map[string]any) []byte {
	if e, ok := data["e"]; ok {
		return f.errorResponse(502, fmt.Sprintf("Relay error: %v", e))
	}
	status := intVal(data["s"], 200)
	headers := map[string]any{}
	if h, ok := data["h"].(map[string]any); ok {
		headers = h
	}
	bodyRaw := ""
	if b, ok := data["b"].(string); ok {
		bodyRaw = b
	}
	body, _ := base64.StdEncoding.DecodeString(bodyRaw)
	if len(body) > f.maxResp {
		return f.errorResponse(502, "Relay response exceeds cap")
	}
	statusText := "OK"
	switch status {
	case 206:
		statusText = "Partial Content"
	case 301:
		statusText = "Moved"
	case 302:
		statusText = "Found"
	case 304:
		statusText = "Not Modified"
	case 400:
		statusText = "Bad Request"
	case 403:
		statusText = "Forbidden"
	case 404:
		statusText = "Not Found"
	case 500:
		statusText = "Internal Server Error"
	}

	buf := bytes.NewBufferString(fmt.Sprintf("HTTP/1.1 %d %s\r\n", status, statusText))
	skip := map[string]bool{
		"transfer-encoding": true,
		"connection":        true,
		"keep-alive":        true,
		"content-length":    true,
		"content-encoding":  true,
	}
	for k, v := range headers {
		lk := strings.ToLower(k)
		if skip[lk] {
			continue
		}
		switch val := v.(type) {
		case []any:
			for _, item := range val {
				buf.WriteString(fmt.Sprintf("%s: %v\r\n", k, item))
			}
		default:
			buf.WriteString(fmt.Sprintf("%s: %v\r\n", k, val))
		}
	}
	buf.WriteString(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)))
	buf.Write(body)
	return buf.Bytes()
}

func (f *DomainFronter) parseBatchBody(body []byte, expected int) ([][]byte, error) {
	text := strings.TrimSpace(string(body))
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, err
	}
	if e, ok := data["e"]; ok {
		return nil, fmt.Errorf("Batch error: %v", e)
	}
	arr, ok := data["q"].([]any)
	if !ok || len(arr) != expected {
		return nil, errors.New("batch size mismatch")
	}
	results := make([][]byte, 0, len(arr))
	for _, item := range arr {
		if obj, ok := item.(map[string]any); ok {
			results = append(results, f.parseRelayJSON(obj))
		}
	}
	return results, nil
}

func (f *DomainFronter) isStatefulRequest(method, urlStr string, headers map[string]string, body []byte) bool {
	method = strings.ToUpper(method)
	if method != "GET" && method != "HEAD" {
		return true
	}
	if len(body) > 0 {
		return true
	}
	for _, name := range constants.StatefulHeaderNames {
		if headerValue(headers, name) != "" {
			return true
		}
	}
	accept := strings.ToLower(headerValue(headers, "accept"))
	if strings.Contains(accept, "text/html") || strings.Contains(accept, "application/json") {
		return true
	}
	fetchMode := strings.ToLower(headerValue(headers, "sec-fetch-mode"))
	if fetchMode == "navigate" || fetchMode == "cors" {
		return true
	}
	return !isStaticAssetURL(urlStr)
}

func isStaticAssetURL(urlStr string) bool {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.Path)
	for _, ext := range constants.StaticExts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func (f *DomainFronter) coalesceKey(urlStr string, headers map[string]string) string {
	key := []string{urlStr}
	if headers != nil {
		for _, name := range []string{"accept", "accept-language", "user-agent", "sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site"} {
			if v := headerValue(headers, name); v != "" {
				key = append(key, name+"="+v)
			}
		}
	}
	return strings.Join(key, "\n")
}

func (f *DomainFronter) recordSite(urlStr string, bytes int, latency time.Duration, errored bool) {
	host := hostKey(urlStr)
	if host == "" {
		return
	}
	stat, ok := f.perSite[host]
	if !ok {
		stat = &HostStat{}
		f.perSite[host] = stat
	}
	stat.Requests++
	stat.Bytes += bytes
	stat.TotalLatencyNs += latency.Nanoseconds()
	if errored {
		stat.Errors++
	}
}

func (f *DomainFronter) statsLoop() {
	ticker := time.NewTicker(time.Duration(constants.StatsLogInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-f.statsStop:
			return
		case <-ticker.C:
			f.logStats()
		}
	}
}

func (f *DomainFronter) logStats() {
	if len(f.perSite) == 0 {
		return
	}
	type statEntry struct {
		host string
		stat *HostStat
	}
	entries := make([]statEntry, 0, len(f.perSite))
	for host, stat := range f.perSite {
		entries = append(entries, statEntry{host: host, stat: stat})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].stat.Bytes > entries[j].stat.Bytes
	})
	count := constants.StatsLogTopN
	if count > len(entries) {
		count = len(entries)
	}
	log.Debugf("-- Per-host stats (top %d by bytes) --", count)
	for i := 0; i < count; i++ {
		e := entries[i]
		avgLatency := time.Duration(0)
		if e.stat.Requests > 0 {
			avgLatency = time.Duration(e.stat.TotalLatencyNs / int64(e.stat.Requests))
		}
		log.Debugf("  %s: %d reqs, %.2fMB, %s avg, %d errs",
			e.host, e.stat.Requests, float64(e.stat.Bytes)/1024/1024, avgLatency, e.stat.Errors)
	}
}

func (f *DomainFronter) ProbeGAS(ctx context.Context, timeout time.Duration) (string, error) {
	payload := map[string]any{"k": "__probe_wrong_key__"}
	jsonBody, _ := json.Marshal(payload)
	path := f.execPath("")
	_, _, body, err := f.h2.Request(ctx, "POST", path, f.httpHost, map[string]string{"content-type": "application/json"}, jsonBody, timeout)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func (f *DomainFronter) ProbeWorker(probeURL string) (int, string) {
	response := f.Relay("GET", probeURL, nil, nil)
	nl := bytes.IndexByte(response, '\n')
	if nl < 0 {
		return 0, "no status line"
	}
	parts := strings.Fields(string(response[:nl]))
	if len(parts) < 2 {
		return 0, "bad status line"
	}
	status, _ := strconv.Atoi(parts[1])
	if idx := bytes.Index(response, []byte("\r\n\r\n")); idx > 0 {
		body := response[idx+4:]
		if pStart := bytes.Index(body, []byte("<p>")); pStart >= 0 {
			rest := body[pStart+3:]
			if pEnd := bytes.Index(rest, []byte("</p>")); pEnd >= 0 {
				return status, strings.TrimSpace(string(rest[:pEnd]))
			}
		}
	}
	return status, ""
}

func headerValue(headers map[string]string, name string) string {
	for k, v := range headers {
		if strings.ToLower(k) == name {
			return v
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func intVal(v any, def int) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		if i, err := strconv.Atoi(t); err == nil {
			return i
		}
	}
	return def
}
