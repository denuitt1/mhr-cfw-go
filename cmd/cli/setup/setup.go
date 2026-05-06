package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/denuitt1/mhr-cfw-go/internal/config"
)

const (
	title           = "MHR-CFW Setup"
	ProjectCodeName = "mhr-cfw-go"
	subtitle        = "Guided configuration for the local relay proxy"
)

const (
	configFilePath = "config.yml"
)

func RunInteractiveWizard() error {
	cfg := config.NewConfig()

	reader := bufio.NewReader(os.Stdin)
	ui := newWizardUI()

	ui.Space()
	ui.Title(title)
	ui.Subtitle(subtitle)
	ui.Space()

	if _, err := os.Stat(configFilePath); err == nil {
		if !promptYesNo(reader, ui, "config.yml already exists. Overwrite?", false) {
			ui.Muted("Nothing changed.")
			return nil
		}
	}

	ui.Section("Shared password")
	ui.Muted("Must match AUTH_KEY inside Google Apps Script Project (Code.gs)")
	cfg.Config.AuthKey = prompt(reader, ui, "auth_key", randomAuthKey(32))

	cfg = configureAppsScript(reader, cfg, ui)
	cfg = configureNetwork(reader, cfg, ui)

	if err := writeConfig(configFilePath, cfg, ui); err != nil {
		return err
	}

	ui.Space()
	ui.Ok("wrote " + filepath.Base(configFilePath))
	ui.Space()
	ui.Section("Next step")
	ui.Code(ProjectCodeName)
	ui.Space()
	ui.Warn("AUTH_KEY inside apps_script/Code.gs must match the auth_key you entered")
	return nil
}

func configureAppsScript(r *bufio.Reader, cfg *config.Config, ui *wizardUI) *config.Config {
	ui.Section("Google Apps Script setup")
	ui.Step(1, "Open https://script.google.com -> New project")
	ui.Step(2, "Paste apps_script/Code.gs from this repo into the editor")
	ui.Step(3, "Set AUTH_KEY in Code.gs to the password above")
	ui.Step(4, "Deploy -> New deployment -> Web app")
	ui.Step(5, "Execute as: Me   |   Who has access: Anyone")
	ui.Step(6, "Copy the Deployment ID and paste it here")
	ui.Space()

	idsRaw := prompt(r, ui, "Deployment ID(s) - comma-separated for load balancing", "")
	ids := []string{}
	for _, v := range strings.Split(idsRaw, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			ids = append(ids, v)
		}
	}

	if len(ids) != 0 {
		cfg.Config.DeploymentIDs = ids
	}

	return cfg
}

func configureNetwork(r *bufio.Reader, cfg *config.Config, ui *wizardUI) *config.Config {
	ui.Section("Network settings")
	ui.Muted("Press enter to accept defaults")
	ui.Space()

	lanSharing := promptYesNo(r, ui, "Enable LAN sharing?", boolVal(cfg.Config.LanSharing))
	cfg.Config.LanSharing = lanSharing

	defaultHost := strVal(cfg.Config.ListenHost)
	if lanSharing && defaultHost == "127.0.0.1" {
		defaultHost = "0.0.0.0"
	}
	cfg.Config.ListenHost = prompt(r, ui, "Listen host", defaultHost)

	port := prompt(r, ui, "HTTP proxy port", fmt.Sprintf("%v", cfg.Config.ListenPort))
	cfg.Config.ListenPort = toInt(port, 8085)

	socks := promptYesNo(r, ui, "Enable SOCKS5 proxy?", boolVal(cfg.Config.Socks5Enabled))
	cfg.Config.Socks5Enabled = socks
	if socks {
		sport := prompt(r, ui, "SOCKS5 port", fmt.Sprintf("%v", cfg.Config.Socks5Port))
		cfg.Config.Socks5Port = toInt(sport, 1080)
	}
	return cfg
}

func writeConfig(path string, cfg *config.Config, ui *wizardUI) error {
	if _, err := os.Stat(path); err == nil {
		backup := strings.TrimSuffix(path, ".yml") + ".yml.bak"
		_ = copyFile(path, backup)
		ui.Muted("existing config.yml backed up to " + filepath.Base(backup))
	}
	return cfg.Flush()
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

func prompt(r *bufio.Reader, ui *wizardUI, question, def string) string {
	for {
		if def != "" {
			ui.Prompt(question, def)
		} else {
			ui.Prompt(question, "")
		}
		raw, _ := r.ReadString('\n')
		raw = strings.TrimSpace(raw)
		if raw == "" && def != "" {
			return def
		}
		if raw != "" {
			return raw
		}
		ui.Error("value required")
	}
}

func promptYesNo(r *bufio.Reader, ui *wizardUI, question string, def bool) bool {
	hint := "Y/n"
	if !def {
		hint = "y/N"
	}
	for {
		ui.Prompt(question, hint)
		raw, _ := r.ReadString('\n')
		raw = strings.TrimSpace(strings.ToLower(raw))
		if raw == "" {
			return def
		}
		if raw == "y" || raw == "yes" {
			return true
		}
		if raw == "n" || raw == "no" {
			return false
		}
	}
}

func randomAuthKey(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	out := make([]byte, length)
	seed := time.Now().UnixNano()
	for i := range out {
		seed = (seed*1664525 + 1013904223) & 0x7fffffff
		out[i] = alphabet[int(seed)%len(alphabet)]
	}
	return string(out)
}

func boolVal(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func strVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toInt(s string, def int) int {
	i, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return i
}
