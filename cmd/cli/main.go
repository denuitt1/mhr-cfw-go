package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/denuitt1/mhr-cfw-go/cmd/cli/setup"
	"github.com/denuitt1/mhr-cfw-go/cmd/cli/tui"
	"github.com/denuitt1/mhr-cfw-go/internal/cert"
	"github.com/denuitt1/mhr-cfw-go/internal/config"
	"github.com/denuitt1/mhr-cfw-go/internal/constants"
	"github.com/denuitt1/mhr-cfw-go/internal/lan"
	"github.com/denuitt1/mhr-cfw-go/internal/logger"
	"github.com/denuitt1/mhr-cfw-go/internal/mitm"
	"github.com/denuitt1/mhr-cfw-go/internal/proxy"
	"github.com/denuitt1/mhr-cfw-go/internal/scanner"
	"github.com/denuitt1/mhr-cfw-go/internal/web"
)

const configFilePath = "config.yml"

var placeholderAuthKeys = map[string]bool{
	"":                             true,
	"CHANGE_ME_TO_A_STRONG_SECRET": true,
	"your-secret-password-here":    true,
}

type args struct {
	configPath    string
	port          int
	host          string
	socksPort     int
	disableSocks  bool
	logLevel      string
	installCert   bool
	uninstallCert bool
	noCertCheck   bool
	scan          bool
}

func parseArgs() (*args, error) {
	a := &args{}
	flag.StringVar(&a.configPath, "config", envOr("DFT_CONFIG", "config.json"), "Path to config file (default: config.json, env: DFT_CONFIG)")
	flag.IntVar(&a.port, "port", 0, "Override listen port (env: DFT_PORT)")
	flag.StringVar(&a.host, "host", "", "Override listen host (env: DFT_HOST)")
	flag.IntVar(&a.socksPort, "socks5-port", 0, "Override SOCKS5 listen port (env: DFT_SOCKS5_PORT)")
	flag.BoolVar(&a.disableSocks, "disable-socks5", false, "Disable the built-in SOCKS5 listener")
	flag.StringVar(&a.logLevel, "log-level", "", "Override log level (env: DFT_LOG_LEVEL)")
	flag.BoolVar(&a.installCert, "install-cert", false, "Install the MITM CA certificate as a trusted root and exit")
	flag.BoolVar(&a.uninstallCert, "uninstall-cert", false, "Remove the MITM CA certificate from trusted roots and exit")
	flag.BoolVar(&a.noCertCheck, "no-cert-check", false, "Skip the certificate installation check on startup")
	flag.BoolVar(&a.scan, "scan", false, "Scan Google IPs to find the fastest reachable one and exit")
	setupFlag := flag.Bool("setup", false, "Run interactive setup wizard and exit")
	noMenu := flag.Bool("no-menu", false, "Run without the interactive TUI menu")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("domainfront-tunnel %s\n", constants.Version)
		os.Exit(0)
	}
	if *setupFlag {
		if err := setup.RunInteractiveWizard(); err != nil {
			fmt.Println("Setup failed:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if !*noMenu && isTTY(os.Stdin) {
		if err := runMenu(a); err != nil {
			fmt.Println("Menu error:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	return a, nil
}

func main() {
	a, err := parseArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, "args error:", err)
		os.Exit(2)
	}
	if a.installCert {
		logger.Configure("INFO")
		if !fileExists(mitm.CACertFile) {
			_ = mitm.NewManager()
		}
		if cert.InstallCA(mitm.CACertFile, cert.DefaultCertName) {
			fmt.Println("[OK] CA installed")
			return
		}
		fmt.Fprintln(os.Stderr, "CA install failed")
		os.Exit(1)
	}
	if a.uninstallCert {
		logger.Configure("INFO")
		if cert.UninstallCA(mitm.CACertFile, cert.DefaultCertName) {
			fmt.Println("[OK] CA removed")
			return
		}
		fmt.Fprintln(os.Stderr, "CA removal failed")
		os.Exit(1)
	}
	if a.scan {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "config error:", err)
			os.Exit(1)
		}
		logger.Configure("INFO")
		if !scanner.ScanSync(cfg.Config.FrontDomain) {
			os.Exit(1)
		}
		return
	}
	if err := runProxy(a); err != nil {
		fmt.Fprintln(os.Stderr, "proxy error:", err)
		os.Exit(1)
	}
}

func runMenu(a *args) error {
	menu := &tui.Menu{
		Title: "MHR-CFW",
		Options: []tui.Option{
			{Key: 1, Label: "Start proxy", Handler: func() error { return runProxy(a) }},
			{Key: 2, Label: "Setup wizard", Handler: func() error { return setup.RunInteractiveWizard() }},
			{Key: 3, Label: "Install CA certificate", Handler: func() error {
				logger.Configure("INFO")
				if !fileExists(mitm.CACertFile) {
					_ = mitm.NewManager()
				}
				if cert.InstallCA(mitm.CACertFile, cert.DefaultCertName) {
					fmt.Println("[OK] CA installed")
					return nil
				}
				return errors.New("CA install failed")
			}},
			{Key: 4, Label: "Uninstall CA certificate", Handler: func() error {
				logger.Configure("INFO")
				if cert.UninstallCA(mitm.CACertFile, cert.DefaultCertName) {
					fmt.Println("[OK] CA removed")
					return nil
				}
				return errors.New("CA removal failed")
			}},
			{Key: 5, Label: "Scan Google IPs", Handler: func() error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				logger.Configure("INFO")
				frontDomain := cfg.Config.FrontDomain
				fmt.Println("\nScanning... this can take a minute on slow networks.")
				ok := scanner.ScanSync(frontDomain)
				if !ok {
					return errors.New("no reachable IPs")
				}
				return nil
			}},
			{Key: 0, Label: "Exit", Handler: nil},
		},
	}
	return menu.Run()
}

func runProxy(a *args) error {
	if a.installCert || a.uninstallCert {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	applyOverrides(cfg, a)
	if err := validateForStart(cfg); err != nil {
		return err
	}

	logger.Configure(cfg.Config.LogLevel)
	log := logger.Get("Main")
	logger.PrintBanner(constants.Version)
	log.Infof("DomainFront Tunnel starting (Apps Script relay)")
	logCfgSummary(log, cfg)

	if !fileExists(mitm.CACertFile) {
		_ = mitm.NewManager()
	}
	if !a.noCertCheck {
		if !cert.IsCATrusted(mitm.CACertFile, cert.DefaultCertName) {
			log.Warnf("MITM CA is not trusted - attempting automatic installation...")
			if cert.InstallCA(mitm.CACertFile, cert.DefaultCertName) {
				log.Infof("CA certificate installed. You may need to restart your browser.")
			} else {
				log.Errorf("Auto-install failed. Run with --install-cert or install ca/ca.crt manually.")
			}
		} else {
			log.Infof("MITM CA is already trusted.")
		}
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-signals
		fmt.Fprintf(os.Stderr, "\nReceived %v, shutting down...\n", sig)
		signal.Stop(signals)
		rootCancel()
		go func() {
			time.Sleep(3 * time.Second)
			fmt.Fprintf(os.Stderr, "Force exit after timeout\n")
			os.Exit(1)
		}()
	}()

	var webSrv *web.Server
	var restartCh <-chan struct{}
	if cfg.Config.WebEnabled {
		webSrv = web.NewServer(cfg, configFilePath)
		restartCh = webSrv.RestartCh()
		go func() {
			if err := webSrv.Start(rootCtx); err != nil {
				log.Errorf("web GUI: %v", err)
			}
		}()
	}

	for {
		applyLANSharing(cfg, log)

		server, err := proxy.NewServer(cfg)
		if err != nil {
			return err
		}
		if webSrv != nil {
			webSrv.SetConfig(cfg)
			webSrv.SetProxy(server)
		}

		proxyCtx, proxyCancel := context.WithCancel(rootCtx)
		proxyDone := make(chan error, 1)
		go func() { proxyDone <- server.Start(proxyCtx) }()

		select {
		case <-rootCtx.Done():
			proxyCancel()
			<-proxyDone
			log.Infof("Stopped")
			return nil
		case <-restartCh:
			if webSrv != nil {
				webSrv.AckRestart()
			}
			log.Infof("Config saved — restarting proxy")
			proxyCancel()
			<-proxyDone
			fresh, err := config.Load()
			if err != nil {
				return err
			}
			applyOverrides(fresh, a)
			if err := validateForStart(fresh); err != nil {
				return err
			}
			logger.Configure(fresh.Config.LogLevel)
			cfg = fresh
		case err := <-proxyDone:
			proxyCancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			log.Infof("Stopped")
			return nil
		}
	}
}

func applyOverrides(cfg *config.Config, a *args) {
	if v := os.Getenv("DFT_AUTH_KEY"); v != "" {
		cfg.Config.AuthKey = v
	}
	if v := os.Getenv("DFT_SCRIPT_ID"); v != "" {
		vals := strings.Split(v, ",")
		if len(vals) != 0 {
			cfg.Config.DeploymentIDs = vals
		}
	}
	if v := os.Getenv("DFT_LOG_LEVEL"); v != "" {
		cfg.Config.LogLevel = v
	}
	if a.host != "" {
		cfg.Config.ListenHost = a.host
	}
	if a.port != 0 {
		cfg.Config.ListenPort = a.port
	}
	if a.socksPort != 0 {
		cfg.Config.Socks5Port = a.socksPort
	}
	if a.disableSocks {
		cfg.Config.Socks5Enabled = false
	}
	if a.logLevel != "" {
		cfg.Config.LogLevel = a.logLevel
	}
}

func validateForStart(cfg *config.Config) error {
	if placeholderAuthKeys[strings.TrimSpace(cfg.Config.AuthKey)] {
		return errors.New("refusing to start: auth_key is unset or placeholder")
	}
	if len(cfg.Config.DeploymentIDs) == 0 || cfg.Config.DeploymentIDs[0] == "YOUR_APPS_SCRIPT_DEPLOYMENT_ID" {
		return errors.New("missing script_id in config")
	}
	return nil
}

func logCfgSummary(log *logger.Logger, cfg *config.Config) {
	log.Infof("Apps Script relay : SNI=%s -> script.google.com", cfg.Config.FrontDomain)
	if ids := cfg.Config.DeploymentIDs; len(ids) > 0 {
		if len(ids) > 1 {
			log.Infof("Script IDs        : %d scripts (sticky per-host)", len(ids))
			for i, id := range ids {
				log.Infof("  [%d] %s", i+1, id)
			}
		} else {
			log.Infof("Script ID         : %s", ids[0])
		}
	}
}

func applyLANSharing(cfg *config.Config, log *logger.Logger) {
	if cfg.Config.LanSharing && cfg.Config.ListenHost == "127.0.0.1" {
		cfg.Config.ListenHost = "0.0.0.0"
		log.Infof("LAN sharing enabled - listening on all interfaces")
	}
	lanMode := cfg.Config.LanSharing || cfg.Config.ListenHost == "0.0.0.0" || cfg.Config.ListenHost == "::"
	if lanMode {
		var socksPort *int
		if cfg.Config.Socks5Enabled {
			p := cfg.Config.Socks5Port
			socksPort = &p
		}
		lan.LogLANAccess(cfg.Config.ListenPort, socksPort)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func exeDir() string {
	exe, _ := os.Executable()
	return filepath.Dir(exe)
}
