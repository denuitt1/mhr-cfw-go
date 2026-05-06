package lan

import (
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/denuitt1/mhr-cfw-go/internal/logger"
)

var log = logger.Get("LAN")

func GetNetworkInterfaces() map[string][]string {
	out := map[string][]string{}
	seen := map[string]bool{}

	add := func(label, ip string) {
		if ip == "" || seen[ip] || strings.HasPrefix(ip, "127.") {
			return
		}
		seen[ip] = true
		out[label] = append(out[label], ip)
	}

	if ip := primaryIPv4(); ip != "" {
		add("primary", ip)
	}

	host, _ := os.Hostname()
	if host != "" {
		if addrs, err := net.LookupIP(host); err == nil {
			for _, a := range addrs {
				if a4 := a.To4(); a4 != nil {
					add("host", a4.String())
				}
			}
		}
	}

	return out
}

func GetLANIPs(port int) []string {
	ifaces := GetNetworkInterfaces()
	var lan []string
	seen := map[string]bool{}
	for _, ips := range ifaces {
		for _, ip := range ips {
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				continue
			}
			if addr.IsLoopback() || addr.IsUnspecified() {
				continue
			}
			if addr.IsPrivate() || addr.IsLinkLocalUnicast() {
				addrStr := ip + ":" + strconv.Itoa(port)
				if !seen[addrStr] {
					seen[addrStr] = true
					lan = append(lan, addrStr)
				}
			}
		}
	}
	return lan
}

func LogLANAccess(port int, socksPort *int) {
	lanHTTP := GetLANIPs(port)
	if len(lanHTTP) > 0 {
		log.Infof("LAN HTTP proxy   : %s", strings.Join(lanHTTP, ", "))
	} else {
		log.Warnf("No LAN IP addresses detected for HTTP proxy")
	}
	if socksPort != nil {
		lanSocks := GetLANIPs(*socksPort)
		if len(lanSocks) > 0 {
			log.Infof("LAN SOCKS5 proxy : %s", strings.Join(lanSocks, ", "))
		} else {
			log.Warnf("No LAN IP addresses detected for SOCKS5 proxy")
		}
	}
}

func primaryIPv4() string {
	conn, err := net.Dial("udp", "192.0.2.1:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	local := conn.LocalAddr().(*net.UDPAddr)
	return local.IP.String()
}
