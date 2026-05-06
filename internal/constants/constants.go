package constants

const AppTitle = "MHR-CFW Go"
const ProjectURL = "https://github.com/denuitt1/mhr-cfw-go"
const Version = "0.1.0"

const (
	DefaultMode                       = "relay"
	DefaultParallelRelay              = 1
	DefaultListenHost                 = "127.0.0.1"
	DefaultListenPort                 = 8080
	DefaultSocks5Enabled              = true
	DefaultSocks5Port                 = 1080
	DefaultLanSharing                 = false
	DefaultFrontDomain                = "www.google.com"
	DefaultGoogleIP                   = "216.239.38.120"
	DefaultVerifySSL                  = true
	DefaultLogLevel                   = "INFO"
	DefaultRelayTimeout               = RelayTimeout
	DefaultTCPConnectTimeout          = TCPConnectTimeout
	DefaultTLSConnectTimeout          = TLSConnectTimeout
	DefaultMaxRequestBodyBytes        = MaxRequestBodyBytes
	DefaultMaxResponseBodyBytes       = MaxResponseBodyBytes
	DefaultChunkedDownloadChunkSize   = ChunkedDownloadChunkSize
	DefaultChunkedDownloadMaxChunks   = ChunkedDownloadMaxChunks
	DefaultChunkedDownloadMaxParallel = ChunkedDownloadMaxParallel
	DefaultChunkedDownloadMinSize     = ChunkedDownloadMinSize
)

const (
	MaxRequestBodyBytes  = 100 * 1024 * 1024
	MaxResponseBodyBytes = 200 * 1024 * 1024
	MaxHeaderBytes       = 64 * 1024
)

const (
	ChunkedDownloadChunkSize   = 512 * 1024
	ChunkedDownloadMaxChunks   = 256
	ChunkedDownloadMaxParallel = 8
	ChunkedDownloadMinSize     = 10 * 512 * 1024
)

// timeouts in time.Millisecond
const (
	ClientIdleTimeout = 120 * 1000
	RelayTimeout      = 25 * 1000
	TLSConnectTimeout = 15 * 1000
	TCPConnectTimeout = 10 * 1000
)

const (
	GoogleScannerTimeout     = 4
	GoogleScannerConcurrency = 8
)

var CandidateIPs = []string{
	"216.239.32.120",
	"216.239.34.120",
	"216.239.36.120",
	"216.239.38.120",
	"142.250.80.142",
	"142.250.80.138",
	"142.250.179.110",
	"142.250.185.110",
	"142.250.184.206",
	"142.250.190.238",
	"142.250.191.78",
	"172.217.1.206",
	"172.217.14.206",
	"172.217.16.142",
	"172.217.22.174",
	"172.217.164.110",
	"172.217.168.206",
	"172.217.169.206",
	"34.107.221.82",
	"142.251.32.110",
	"142.251.33.110",
	"142.251.46.206",
	"142.251.46.238",
	"142.250.80.170",
	"142.250.72.206",
	"142.250.64.206",
	"142.250.72.110",
}

const (
	CacheMaxMB         = 50
	CacheTTLStaticLong = 3600
	CacheTTLStaticMed  = 1800
	CacheTTLMax        = 86400
)

const (
	PoolMax       = 50
	PoolMinIdle   = 15
	ConnTTL       = 45.0
	SemaphoreMax  = 50
	WarmPoolCount = 30
)

const (
	BatchWindowMicro = 0.005
	BatchWindowMacro = 0.050
	BatchMax         = 50
)

const (
	ScriptBlacklistTTL = 600.0
)

var FrontSNIPoolGoogle = []string{
	"www.google.com",
	"mail.google.com",
	"accounts.google.com",
}

const (
	StatsLogInterval = 300.0
	StatsLogTopN     = 10
)

var GoogleDirectExactExclude = map[string]struct{}{
	"gemini.google.com":     {},
	"aistudio.google.com":   {},
	"notebooklm.google.com": {},
	"labs.google.com":       {},
	"meet.google.com":       {},
	"accounts.google.com":   {},
	"ogs.google.com":        {},
	"mail.google.com":       {},
	"calendar.google.com":   {},
	"drive.google.com":      {},
	"docs.google.com":       {},
	"chat.google.com":       {},
	"photos.google.com":     {},
	"maps.google.com":       {},
	"myaccount.google.com":  {},
	"contacts.google.com":   {},
	"classroom.google.com":  {},
	"keep.google.com":       {},
	"play.google.com":       {},
	"translate.google.com":  {},
	"assistant.google.com":  {},
	"lens.google.com":       {},
}

var GoogleDirectSuffixExclude = []string{
	".meet.google.com",
}

var GoogleDirectAllowExact = map[string]struct{}{
	"www.google.com":          {},
	"google.com":              {},
	"safebrowsing.google.com": {},
}

var GoogleDirectAllowSuffixes = []string{}

var GoogleOwnedSuffixes = []string{
	".google.com", ".google.co",
	".googleapis.com", ".gstatic.com",
	".googleusercontent.com",
}
var GoogleOwnedExact = map[string]struct{}{
	"google.com":     {},
	"gstatic.com":    {},
	"googleapis.com": {},
}

var SNIRewriteSuffixes = []string{
	"youtube.com",
	"youtu.be",
	"youtube-nocookie.com",
	"ytimg.com",
	"ggpht.com",
	"gvt1.com",
	"gvt2.com",
	"doubleclick.net",
	"googlesyndication.com",
	"googleadservices.com",
	"google-analytics.com",
	"googletagmanager.com",
	"googletagservices.com",
	"fonts.googleapis.com",
	"script.google.com",
}

var TraceHostSuffixes = []string{
	"chatgpt.com",
	"openai.com",
	"gemini.google.com",
	"google.com",
	"cloudflare.com",
	"challenges.cloudflare.com",
	"turnstile",
}

var StaticExts = []string{
	".css", ".js", ".mjs", ".woff", ".woff2", ".ttf", ".eot",
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico",
	".mp3", ".mp4", ".webm", ".wasm", ".avif",
}

var LargeFileExts = map[string]struct{}{
	".bin": {},
	".zip": {}, ".tar": {}, ".gz": {}, ".bz2": {}, ".xz": {}, ".7z": {}, ".rar": {},
	".exe": {}, ".msi": {}, ".dmg": {}, ".deb": {}, ".rpm": {}, ".apk": {},
	".iso": {}, ".img": {},
	".mp4": {}, ".mkv": {}, ".avi": {}, ".mov": {}, ".webm": {},
	".mp3": {}, ".flac": {}, ".wav": {}, ".aac": {},
	".pdf": {}, ".doc": {}, ".docx": {}, ".ppt": {}, ".pptx": {},
	".wasm": {},
}

var StatefulHeaderNames = []string{
	"cookie", "authorization", "proxy-authorization",
	"origin", "referer", "if-none-match", "if-modified-since",
	"cache-control", "pragma",
}

var UncacheableHeaderNames = []string{
	"cookie", "authorization", "proxy-authorization", "range",
	"if-none-match", "if-modified-since", "cache-control", "pragma",
}
