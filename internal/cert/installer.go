package cert

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/denuitt1/mhr-cfw-go/internal/logger"
)

const DefaultCertName = "MHR-CFW"

var log = logger.Get("Cert")

func InstallCA(certPath, certName string) bool {
	if _, err := os.Stat(certPath); err != nil {
		log.Errorf("Certificate file not found: %s", certPath)
		return false
	}
	switch runtime.GOOS {
	case "windows":
		ok := installWindows(certPath)
		installFirefox(certPath, certName)
		return ok
	case "darwin":
		ok := installMacOS(certPath)
		installFirefox(certPath, certName)
		return ok
	case "linux":
		ok := installLinux(certPath, certName)
		installFirefox(certPath, certName)
		return ok
	default:
		log.Errorf("Unsupported platform: %s", runtime.GOOS)
		return false
	}
}

func UninstallCA(certPath, certName string) bool {
	switch runtime.GOOS {
	case "windows":
		ok := uninstallWindows(certPath, certName)
		uninstallFirefox(certName)
		return ok
	case "darwin":
		ok := uninstallMacOS(certName)
		uninstallFirefox(certName)
		return ok
	case "linux":
		ok := uninstallLinux(certPath, certName)
		uninstallFirefox(certName)
		return ok
	default:
		log.Errorf("Unsupported platform: %s", runtime.GOOS)
		return false
	}
}

func IsCATrusted(certPath, certName string) bool {
	switch runtime.GOOS {
	case "windows":
		return isTrustedWindows(certPath)
	case "darwin":
		return isTrustedMacOS(certName)
	case "linux":
		return isTrustedLinux(certPath, certName)
	default:
		return false
	}
}

func run(cmd []string, check bool) ([]byte, error) {
	c := exec.Command(cmd[0], cmd[1:]...)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	if err != nil && check {
		return buf.Bytes(), err
	}
	return buf.Bytes(), err
}

func installWindows(certPath string) bool {
	if _, err := run([]string{"certutil", "-addstore", "-user", "Root", certPath}, true); err == nil {
		log.Infof("Certificate installed in Windows user Trusted Root store.")
		return true
	}
	if _, err := run([]string{"certutil", "-addstore", "Root", certPath}, true); err == nil {
		log.Infof("Certificate installed in Windows system Trusted Root store.")
		return true
	}
	ps := "Import-Certificate -FilePath '" + certPath + "' -CertStoreLocation Cert:\\CurrentUser\\Root"
	if _, err := run([]string{"powershell", "-NoProfile", "-Command", ps}, true); err == nil {
		log.Infof("Certificate installed via PowerShell.")
		return true
	}
	return false
}

func isTrustedWindows(certPath string) bool {
	out, err := run([]string{"certutil", "-user", "-store", "Root"}, true)
	if err != nil {
		return false
	}
	thumb := certThumbprint(certPath)
	if thumb == "" {
		return false
	}
	return strings.Contains(strings.ToUpper(string(out)), thumb)
}

func uninstallWindows(certPath, certName string) bool {
	thumb := certThumbprint(certPath)
	target := certName
	if thumb != "" {
		target = thumb
	}
	if _, err := run([]string{"certutil", "-delstore", "-user", "Root", target}, true); err == nil {
		log.Infof("Certificate removed from Windows user Trusted Root store.")
		return true
	}
	if _, err := run([]string{"certutil", "-delstore", "Root", target}, true); err == nil {
		log.Infof("Certificate removed from Windows system Trusted Root store.")
		return true
	}
	return false
}

func installMacOS(certPath string) bool {
	login := filepath.Join(os.Getenv("HOME"), "Library/Keychains/login.keychain-db")
	if _, err := run([]string{"security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", login, certPath}, true); err == nil {
		log.Infof("Certificate installed in macOS login keychain.")
		return true
	}
	if _, err := run([]string{"sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certPath}, true); err == nil {
		log.Infof("Certificate installed in macOS system keychain.")
		return true
	}
	return false
}

func isTrustedMacOS(certName string) bool {
	out, err := run([]string{"security", "find-certificate", "-a", "-c", certName}, true)
	return err == nil && len(bytes.TrimSpace(out)) > 0
}

func uninstallMacOS(certName string) bool {
	login := filepath.Join(os.Getenv("HOME"), "Library/Keychains/login.keychain-db")
	if _, err := run([]string{"security", "delete-certificate", "-c", certName, login}, true); err == nil {
		log.Infof("Certificate removed from macOS login keychain.")
		return true
	}
	if _, err := run([]string{"sudo", "security", "delete-certificate", "-c", certName, "/Library/Keychains/System.keychain"}, true); err == nil {
		log.Infof("Certificate removed from macOS system keychain.")
		return true
	}
	return false
}

func installLinux(certPath, certName string) bool {
	distro := detectLinuxDistro()
	log.Infof("Detected Linux distro family: %s", distro)

	switch distro {
	case "debian":
		dest := "/usr/local/share/ca-certificates/" + strings.ReplaceAll(certName, " ", "_") + ".crt"
		if _, err := run([]string{"cp", certPath, dest}, true); err == nil {
			_, _ = run([]string{"update-ca-certificates"}, true)
			log.Infof("Certificate installed via update-ca-certificates.")
			return true
		}
	case "rhel":
		dest := "/etc/pki/ca-trust/source/anchors/" + strings.ReplaceAll(certName, " ", "_") + ".crt"
		if _, err := run([]string{"cp", certPath, dest}, true); err == nil {
			_, _ = run([]string{"update-ca-trust", "extract"}, true)
			log.Infof("Certificate installed via update-ca-trust.")
			return true
		}
	case "arch":
		dest := "/etc/ca-certificates/trust-source/anchors/" + strings.ReplaceAll(certName, " ", "_") + ".crt"
		if _, err := run([]string{"cp", certPath, dest}, true); err == nil {
			_, _ = run([]string{"trust", "extract-compat"}, true)
			log.Infof("Certificate installed via trust extract-compat.")
			return true
		}
	}
	log.Warnf("Unknown Linux distro. Manually install %s as a trusted root CA.", certPath)
	return false
}

func isTrustedLinux(certPath, certName string) bool {
	target := strings.ReplaceAll(certName, " ", "_") + ".crt"
	paths := []string{
		"/usr/local/share/ca-certificates/" + target,
		"/etc/pki/ca-trust/source/anchors/" + target,
		"/etc/ca-certificates/trust-source/anchors/" + target,
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func uninstallLinux(certPath, certName string) bool {
	distro := detectLinuxDistro()
	log.Infof("Detected Linux distro family: %s", distro)

	switch distro {
	case "debian":
		dest := "/usr/local/share/ca-certificates/" + strings.ReplaceAll(certName, " ", "_") + ".crt"
		_ = os.Remove(dest)
		_, _ = run([]string{"update-ca-certificates"}, true)
		log.Infof("Certificate removed via update-ca-certificates.")
		return true
	case "rhel":
		dest := "/etc/pki/ca-trust/source/anchors/" + strings.ReplaceAll(certName, " ", "_") + ".crt"
		_ = os.Remove(dest)
		_, _ = run([]string{"update-ca-trust", "extract"}, true)
		log.Infof("Certificate removed via update-ca-trust.")
		return true
	case "arch":
		dest := "/etc/ca-certificates/trust-source/anchors/" + strings.ReplaceAll(certName, " ", "_") + ".crt"
		_ = os.Remove(dest)
		_, _ = run([]string{"trust", "extract-compat"}, true)
		log.Infof("Certificate removed via trust extract-compat.")
		return true
	}
	log.Warnf("Unknown Linux distro. Manually remove %s from trusted CAs.", certName)
	return false
}

func detectLinuxDistro() string {
	if fileExists("/etc/debian_version") || fileExists("/etc/ubuntu") {
		return "debian"
	}
	if fileExists("/etc/redhat-release") || fileExists("/etc/fedora-release") {
		return "rhel"
	}
	if fileExists("/etc/arch-release") {
		return "arch"
	}
	return "unknown"
}

func installFirefox(certPath, certName string) {
	if _, err := exec.LookPath("certutil"); err != nil {
		return
	}
	profiles := firefoxProfiles()
	for _, profile := range profiles {
		db := "sql:" + profile
		if !fileExists(filepath.Join(profile, "cert9.db")) {
			db = "dbm:" + profile
		}
		_, _ = run([]string{"certutil", "-D", "-n", certName, "-d", db}, false)
		_, _ = run([]string{"certutil", "-A", "-n", certName, "-t", "CT,,", "-i", certPath, "-d", db}, true)
	}
}

func uninstallFirefox(certName string) {
	if _, err := exec.LookPath("certutil"); err != nil {
		return
	}
	profiles := firefoxProfiles()
	for _, profile := range profiles {
		db := "sql:" + profile
		if !fileExists(filepath.Join(profile, "cert9.db")) {
			db = "dbm:" + profile
		}
		_, _ = run([]string{"certutil", "-D", "-n", certName, "-d", db}, false)
	}
}

func firefoxProfiles() []string {
	var out []string
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata != "" {
			out = append(out, glob(filepath.Join(appdata, "Mozilla", "Firefox", "Profiles", "*"))...)
		}
	case "darwin":
		out = append(out, glob(filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Firefox", "Profiles", "*"))...)
	default:
		out = append(out, glob(filepath.Join(os.Getenv("HOME"), ".mozilla", "firefox", "*.default*"))...)
		out = append(out, glob(filepath.Join(os.Getenv("HOME"), ".mozilla", "firefox", "*.release*"))...)
	}
	return out
}

func glob(pattern string) []string {
	m, _ := filepath.Glob(pattern)
	return m
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func certThumbprint(certPath string) string {
	raw, err := os.ReadFile(certPath)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
