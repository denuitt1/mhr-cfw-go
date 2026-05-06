package setup

import (
	"fmt"
	"os"
	"strings"
)

type wizardUI struct {
	color bool
}

func newWizardUI() *wizardUI {
	color := supportsColor()
	return &wizardUI{color: color}
}

func (w *wizardUI) Space() {
	fmt.Println()
}

func (w *wizardUI) Title(text string) {
	line := strings.Repeat("─", max(48, len(text)+12))
	if w.color {
		fmt.Println(dim(line))
		fmt.Println(bold(cyan("  " + text + "  ")))
		fmt.Println(dim(line))
		return
	}
	fmt.Println(line)
	fmt.Println("  " + text)
	fmt.Println(line)
}

func (w *wizardUI) Subtitle(text string) {
	if w.color {
		fmt.Println(dim(text))
		return
	}
	fmt.Println(text)
}

func (w *wizardUI) Section(text string) {
	if w.color {
		fmt.Println(bold(cyan(text)))
		return
	}
	fmt.Println(text)
}

func (w *wizardUI) Step(n int, text string) {
	label := fmt.Sprintf("%d.", n)
	if w.color {
		fmt.Println(dim(label), text)
		return
	}
	fmt.Println(label, text)
}

func (w *wizardUI) Code(text string) {
	if w.color {
		fmt.Println(dim("  $"), bold(text))
		return
	}
	fmt.Println("  $", text)
}

func (w *wizardUI) Prompt(question, hint string) {
	if hint != "" {
		if w.color {
			fmt.Printf("%s %s %s: ", cyan("?"), question, dim("["+hint+"]"))
			return
		}
		fmt.Printf("? %s [%s]: ", question, hint)
		return
	}
	if w.color {
		fmt.Printf("%s %s: ", cyan("?"), question)
		return
	}
	fmt.Printf("? %s: ", question)
}

func (w *wizardUI) Ok(text string) {
	if w.color {
		fmt.Println(green("[OK]"), text)
		return
	}
	fmt.Println("[OK]", text)
}

func (w *wizardUI) Warn(text string) {
	if w.color {
		fmt.Println(yellow("!"), text)
		return
	}
	fmt.Println("!", text)
}

func (w *wizardUI) Error(text string) {
	if w.color {
		fmt.Println(red("!"), text)
		return
	}
	fmt.Println("!", text)
}

func (w *wizardUI) Muted(text string) {
	if w.color {
		fmt.Println(dim(text))
		return
	}
	fmt.Println(text)
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

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func bold(s string) string   { return "\x1b[1m" + s + "\x1b[0m" }
func dim(s string) string    { return "\x1b[2m" + s + "\x1b[0m" }
func cyan(s string) string   { return "\x1b[36m" + s + "\x1b[0m" }
func green(s string) string  { return "\x1b[32m" + s + "\x1b[0m" }
func yellow(s string) string { return "\x1b[33m" + s + "\x1b[0m" }
func red(s string) string    { return "\x1b[31m" + s + "\x1b[0m" }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
