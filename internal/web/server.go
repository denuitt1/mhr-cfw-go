package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/denuitt1/mhr-cfw-go/internal/config"
	"github.com/denuitt1/mhr-cfw-go/internal/logger"
	"github.com/denuitt1/mhr-cfw-go/internal/proxy"
)

//go:embed assets
var assetsFS embed.FS

var log = logger.Get("Web")

type Server struct {
	port       int
	configPath string

	mu        sync.RWMutex
	proxy     *proxy.Server
	cfg       *config.Config
	restartCh chan struct{}

	httpSrv  *http.Server
	listener net.Listener

	pendingRestart atomic.Bool
}

func NewServer(cfg *config.Config, configPath string) *Server {
	return &Server{
		port:       cfg.Config.WebPort,
		configPath: configPath,
		cfg:        cfg,
		restartCh:  make(chan struct{}, 1),
	}
}

func (s *Server) SetProxy(p *proxy.Server) {
	s.mu.Lock()
	s.proxy = p
	s.mu.Unlock()
}

func (s *Server) SetConfig(cfg *config.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.port = cfg.Config.WebPort
	s.mu.Unlock()
}

func (s *Server) RestartCh() <-chan struct{} {
	return s.restartCh
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/probe", s.handleProbe)
	mux.HandleFunc("/api/version", s.handleVersion)

	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("web listen %s: %w", addr, err)
	}
	s.listener = ln

	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Infof("Web GUI listening on http://%s", addr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
	}()

	if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    cfg.Name,
		"version": cfg.Version,
		"url":     cfg.URL,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		cfg := s.cfg
		s.mu.RUnlock()
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		s.putConfig(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var incoming config.Config
	if err := json.Unmarshal(body, &incoming); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	if err := validateConfig(&incoming); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := writeYAML(s.configPath, &incoming); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.SetConfig(&incoming)
	s.signalRestart()
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved; proxy restarting"})
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	cfg := s.cfg
	srv := s.proxy
	s.mu.RUnlock()

	report := ProbeReport{}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		report.Google = probeGoogleFront(cfg.Config.GoogleIP, cfg.Config.FrontDomain)
	}()
	go func() {
		defer wg.Done()
		report.GAS = probeGAS(srv)
	}()
	go func() {
		defer wg.Done()
		report.Worker = probeWorker(srv, cfg.Config.ProbeURL, cfg.Config.WorkerURL, cfg.Config.UpstreamForwarderURL)
	}()
	wg.Wait()

	writeJSON(w, http.StatusOK, report)
}

func (s *Server) signalRestart() {
	if s.pendingRestart.Swap(true) {
		return
	}
	select {
	case s.restartCh <- struct{}{}:
	default:
	}
}

func (s *Server) AckRestart() {
	s.pendingRestart.Store(false)
}

func validateConfig(c *config.Config) error {
	if c == nil {
		return errors.New("empty config")
	}
	d := &c.Config
	if d.AuthKey == "" {
		return errors.New("auth_key is required")
	}
	if len(d.DeploymentIDs) == 0 {
		return errors.New("deployment_ids must contain at least one entry")
	}
	if d.ParallelRelay != len(d.DeploymentIDs) {
		return fmt.Errorf("parallel_relay (%d) must equal the number of deployment_ids (%d)", d.ParallelRelay, len(d.DeploymentIDs))
	}
	if d.ListenPort <= 0 || d.ListenPort > 65535 {
		return errors.New("listen_port out of range")
	}
	if d.Socks5Enabled && (d.Socks5Port <= 0 || d.Socks5Port > 65535) {
		return errors.New("socks5_port out of range")
	}
	if d.Socks5Enabled && d.Socks5Port == d.ListenPort {
		return errors.New("listen_port and socks5_port must differ")
	}
	if d.WebPort <= 0 || d.WebPort > 65535 {
		return errors.New("web_port out of range")
	}
	if d.WebPort == d.ListenPort || (d.Socks5Enabled && d.WebPort == d.Socks5Port) {
		return errors.New("web_port collides with another listener")
	}
	return nil
}

func writeYAML(path string, cfg *config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
