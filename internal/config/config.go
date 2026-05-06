package config

import (
	"errors"
	"os"

	"github.com/denuitt1/mhr-cfw-go/internal/constants"
	"gopkg.in/yaml.v3"
)

const (
	configFilePath = "config.yml"
)

type Config struct {
	Name    string     `yaml:"name" json:"name"`
	URL     string     `yaml:"url" json:"url"`
	Version string     `yaml:"version" json:"version"`
	Config  ConfigData `yaml:"config" json:"config"`
}

type ConfigData struct {
	Mode                       string   `yaml:"mode" json:"mode"`
	AuthKey                    string   `yaml:"auth_key" json:"auth_key"`
	DeploymentIDs              []string `yaml:"deployment_ids" json:"deployment_ids"`
	ParallelRelay              int      `yaml:"parallel_relay" json:"parallel_relay"`
	ListenHost                 string   `yaml:"listen_host" json:"listen_host"`
	ListenPort                 int      `yaml:"listen_port" json:"listen_port"`
	Socks5Enabled              bool     `yaml:"socks5_enabled" json:"socks5_enabled"`
	Socks5Port                 int      `yaml:"socks5_port" json:"socks5_port"`
	LanSharing                 bool     `yaml:"lan_sharing" json:"lan_sharing"`
	FrontDomain                string   `yaml:"front_domain" json:"front_domain"`
	GoogleIP                   string   `yaml:"google_ip" json:"google_ip"`
	VerifySSL                  bool     `yaml:"verify_ssl" json:"verify_ssl"`
	Hosts                      []string `yaml:"hosts" json:"hosts"`
	LogLevel                   string   `yaml:"log_level" json:"log_level"`
	RelayTimeout               int      `yaml:"relay_timeout" json:"relay_timeout"`
	TCPConnectTimeout          int      `yaml:"tcp_connect_timeout" json:"tcp_connect_timeout"`
	TLSConnectTimeout          int      `yaml:"tls_connect_timeout" json:"tls_connect_timeout"`
	MaxRequestBodyBytes        int      `yaml:"max_request_body_bytes" json:"max_request_body_bytes"`
	MaxResponseBodyBytes       int      `yaml:"max_response_body_bytes" json:"max_response_body_bytes"`
	ChunkedDownloadChunkSize   int      `yaml:"chunked_download_chunk_size" json:"chunked_download_chunk_size"`
	ChunkedDownloadMaxParallel int      `yaml:"chunked_download_max_parallel" json:"chunked_download_max_parallel"`
	ChunkedDownloadMinSize     int      `yaml:"chunked_download_min_size" json:"chunked_download_min_size"`
	WebEnabled                 bool     `yaml:"web_enabled" json:"web_enabled"`
	WebPort                    int      `yaml:"web_port" json:"web_port"`
	ProbeURL                   string   `yaml:"probe_url" json:"probe_url"`
	WorkerURL                  string   `yaml:"worker_url" json:"worker_url"`
	UpstreamForwarderURL       string   `yaml:"upstream_forwarder_url" json:"upstream_forwarder_url"`
}

// NewConfig returns a new Config instance with default values
func NewConfig() *Config {
	return &Config{
		Name:    constants.AppTitle,
		URL:     constants.ProjectURL,
		Version: constants.Version,
		Config: ConfigData{
			Mode:                       constants.DefaultMode,
			AuthKey:                    "",
			DeploymentIDs:              []string{},
			ParallelRelay:              constants.DefaultParallelRelay,
			ListenHost:                 constants.DefaultListenHost,
			ListenPort:                 constants.DefaultListenPort,
			Socks5Enabled:              constants.DefaultSocks5Enabled,
			Socks5Port:                 constants.DefaultSocks5Port,
			LanSharing:                 constants.DefaultLanSharing,
			FrontDomain:                constants.DefaultFrontDomain,
			GoogleIP:                   constants.DefaultGoogleIP,
			VerifySSL:                  constants.DefaultVerifySSL,
			Hosts:                      []string{},
			LogLevel:                   constants.DefaultLogLevel,
			RelayTimeout:               constants.DefaultRelayTimeout,
			TCPConnectTimeout:          constants.DefaultTCPConnectTimeout,
			TLSConnectTimeout:          constants.DefaultTLSConnectTimeout,
			MaxRequestBodyBytes:        constants.DefaultMaxRequestBodyBytes,
			MaxResponseBodyBytes:       constants.DefaultMaxResponseBodyBytes,
			ChunkedDownloadChunkSize:   constants.DefaultChunkedDownloadChunkSize,
			ChunkedDownloadMaxParallel: constants.DefaultChunkedDownloadMaxParallel,
			ChunkedDownloadMinSize:     constants.DefaultChunkedDownloadMinSize,
			WebEnabled:                 constants.DefaultWebEnabled,
			WebPort:                    constants.DefaultWebPort,
			ProbeURL:                   constants.DefaultProbeURL,
			WorkerURL:                  constants.DefaultWorkerURL,
			UpstreamForwarderURL:       constants.DefaultUpstreamForwarderURL,
		},
	}
}

// InitializeConfigFile - makes a new config.yml with default values provided by NewConfig
func InitializeConfigFile() (*Config, error) {
	// try removing the file in case it exists
	_ = os.Remove(configFilePath)

	cfg := NewConfig()

	err := cfg.Flush()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// Load - loads configuration from config.yml
func Load() (*Config, error) {
	f, err := os.Open(configFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InitializeConfigFile()
		}
		return nil, err
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	var config Config
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// Flush - saves the current configuration instance into config.yml
func (cfg *Config) Flush() error {
	f, err := os.OpenFile(configFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(cfg)
}
