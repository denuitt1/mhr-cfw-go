package tui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	title     = "MHR-CFW Go v0.1.0"
	githubURL = "https://github.com/denuitt1"
)

type Option struct {
	Key     int
	Label   string
	Handler func() error
}

type Menu struct {
	Title   string
	Options []Option
}

func (m *Menu) Run() error {
	reader := bufio.NewReader(os.Stdin)
	for {
		clearScreen()
		m.render()
		fmt.Print("Select an option: ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx, err := strconv.Atoi(line)
		if err != nil {
			fmt.Println("Invalid selection.")
			continue
		}
		for _, opt := range m.Options {
			if opt.Key == idx {
				if opt.Handler == nil {
					return nil
				}
				if err := opt.Handler(); err != nil {
					fmt.Println("Error:", err)
				}
				fmt.Print("\nPress Enter to return to menu...")
				_, _ = reader.ReadString('\n')
				break
			}
		}
	}
}

func (m *Menu) render() {
	useColor := supportsColor()
	width := max(70, len(m.Title)+16)
	borderTop := "╔ " + strings.Repeat("═", width) + " ╗"
	borderMid := "╠" + strings.Repeat("═", width+2) + "╣"
	borderBot := "╚ " + strings.Repeat("═", width) + " ╝"
	inner := "║" + strings.Repeat(" ", width+2) + "║"
	tag := title
	link := githubURL

	centerText := func(text string) string {
		pad := width + 2 - len(text)
		left := pad / 2
		right := pad - left
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
	}

	fmt.Println()
	fmt.Println(borderTop)
	fmt.Println(inner)

	if useColor {
		fmt.Println(cyan(centerText(tag)))
		fmt.Println(faint(centerText(link)))
	} else {
		fmt.Println(centerText(tag))
		fmt.Println(centerText(link))
	}

	fmt.Println(inner)
	fmt.Println(borderMid)

	for _, opt := range m.Options {
		label := fmt.Sprintf("%d) %s", opt.Key, opt.Label)
		if useColor {
			fmt.Println("  " + violet(">") + " " + bold(ice(fmt.Sprintf("%d)", opt.Key))) + " " + label[3:])
		} else {
			fmt.Println("  * " + label)
		}
	}

	if useColor {
		fmt.Println(dim(borderBot))
	} else {
		fmt.Println(borderBot)
	}
	fmt.Println()
}

func supportsColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("DFT_NO_COLOR") == "1" {
		return false
	}
	if !isTTY(os.Stdout) {
		return false
	}
	return true
}

func clearScreen() {
	if !supportsColor() {
		return
	}
	fmt.Print("\x1b[2J\x1b[H")
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func bold(s string) string   { return "\x1b[1m" + s + "\x1b[0m" }
func dim(s string) string    { return "\x1b[2m" + s + "\x1b[0m" }
func faint(s string) string  { return "\x1b[38;5;250m" + s + "\x1b[0m" }
func teal(s string) string   { return "\x1b[1;38;5;45m" + s + "\x1b[0m" }
func ice(s string) string    { return "\x1b[1;38;5;81m" + s + "\x1b[0m" }
func violet(s string) string { return "\x1b[38;5;141m" + s + "\x1b[0m" }
func cyan(s string) string   { return "\x1b[1;36m" + s + "\x1b[0m" }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
