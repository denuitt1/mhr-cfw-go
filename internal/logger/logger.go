package logger

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

type Logger struct {
	name string
}

var (
	mu        sync.RWMutex
	globalLvl           = Info
	colorOn             = false
	outWriter io.Writer = os.Stderr
)

func Configure(level string) {
	lvl := Info
	switch strings.ToUpper(level) {
	case "DEBUG":
		lvl = Debug
	case "WARNING", "WARN":
		lvl = Warn
	case "ERROR":
		lvl = Error
	default:
		lvl = Info
	}
	mu.Lock()
	globalLvl = lvl
	outWriter = os.Stderr
	colorOn = supportsColor(os.Stderr)
	mu.Unlock()
}

func Get(name string) *Logger {
	return &Logger{name: name}
}

func (l *Logger) Debugf(format string, args ...any) {
	l.log(Debug, format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.log(Info, format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.log(Warn, format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.log(Error, format, args...)
}

func (l *Logger) log(level Level, format string, args ...any) {
	mu.RLock()
	if level < globalLvl {
		mu.RUnlock()
		return
	}
	out := outWriter
	useColor := colorOn
	mu.RUnlock()

	now := time.Now()
	ts := now.Format("15:04:05")
	levelLabel := levelText(level)
	line := fmt.Sprintf(format, args...)
	component := l.name
	if len(component) > 8 {
		component = component[:8]
	}
	component = fmt.Sprintf("%-8s", component)

	if useColor {
		ts = color("90", ts)
		levelLabel = color(levelColor(level), levelLabel)
		component = color(componentColor(l.name), "["+component+"]")
	} else {
		component = "[" + component + "]"
	}

	fmt.Fprintf(out, "%s  %s  %s  %s\n", ts, levelLabel, component, line)
}

func levelText(level Level) string {
	switch level {
	case Debug:
		return "DBG"
	case Info:
		return "INF"
	case Warn:
		return "WRN"
	case Error:
		return "ERR"
	default:
		return "INF"
	}
}

func levelColor(level Level) string {
	switch level {
	case Debug:
		return "38;5;245"
	case Info:
		return "38;5;39"
	case Warn:
		return "38;5;214"
	case Error:
		return "38;5;203"
	default:
		return "38;5;39"
	}
}

func componentColor(name string) string {
	switch name {
	case "Main":
		return "38;5;81"
	case "Proxy":
		return "38;5;75"
	case "Fronter":
		return "38;5;141"
	case "H2":
		return "38;5;87"
	case "MITM":
		return "38;5;208"
	case "Cert":
		return "38;5;177"
	case "LAN":
		return "38;5;80"
	case "Scanner":
		return "38;5;45"
	default:
		return "38;5;245"
	}
}

func color(code, text string) string {
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func bold(s string) string   { return "\x1b[1m" + s + "\x1b[0m" }
func dim(s string) string    { return "\x1b[2m" + s + "\x1b[0m" }
func teal(s string) string   { return "\x1b[1;38;5;45m" + s + "\x1b[0m" }
func faint(s string) string  { return "\x1b[38;5;250m" + s + "\x1b[0m" }
func amber(s string) string  { return "\x1b[38;5;214m" + s + "\x1b[0m" }
func violet(s string) string { return "\x1b[38;5;141m" + s + "\x1b[0m" }

func supportsColor(stream *os.File) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("DFT_NO_COLOR") == "1" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("DFT_FORCE_COLOR") != "" {
		return true
	}
	info, err := stream.Stat()
	if err != nil || (info.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	if runtime.GOOS != "windows" {
		return true
	}
	return enableVirtualTerminal(stream)
}

// PowerShell + Windows Terminal already support ANSI; conservative default
func enableVirtualTerminal(stream *os.File) bool {
	if runtime.GOOS != "windows" {
		return true
	}
	return true
}

// ---------------------------------------------------------

const (
	bannerTitle    string = "MHR-CFW Go Version"
	bannerSubtitle string = "Domain-Fronted Relay Suite"
	bannerCredit   string = "https://github.com/denuitt1"
)

func PrintBanner(version string) {
	versionTag := "v" + version

	innerWidth := max(76, max(len(bannerTitle), max(len(bannerSubtitle), len(bannerCredit)))+8)
	line := strings.Repeat("═", innerWidth)
	borderTop := "╔ " + line + " ╗"
	borderMid := "║" + strings.Repeat(" ", innerWidth) + "║"
	borderBot := "╚ " + line + " ╝"

	centerLine := func(text string) string {
		pad := innerWidth - len(text)
		left := pad / 2
		right := pad - left
		return "║" + strings.Repeat(" ", left) + text + strings.Repeat(" ", right) + "║"
	}

	if colorOn {
		fmt.Fprintln(outWriter)
		fmt.Fprintln(outWriter, borderTop)
		fmt.Fprintln(outWriter, borderMid)
		outLine := "║" + bold(teal(centerLine(bannerTitle))) + "║"
		fmt.Fprintln(outWriter, outLine)
		outLine = "║" + faint(centerLine(bannerSubtitle)) + "║"
		fmt.Fprintln(outWriter, outLine)
		outLine = "║" + amber(centerLine(versionTag)) + "║"
		fmt.Fprintln(outWriter, outLine)
		outLine = "║" + violet(centerLine(bannerCredit)) + "║"
		fmt.Fprintln(outWriter, outLine)
		fmt.Fprintln(outWriter, borderMid)
		fmt.Fprintln(outWriter, borderBot)
		return
	}

	fmt.Println()
	fmt.Println(borderTop)
	fmt.Println(borderMid)
	fmt.Println(centerLine(bannerTitle))
	fmt.Println(centerLine(bannerSubtitle))
	fmt.Println(centerLine(versionTag))
	fmt.Println(centerLine(bannerCredit))
	fmt.Println(borderMid)
	fmt.Println(borderBot)
}
