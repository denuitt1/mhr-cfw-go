package scanner

import (
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/denuitt1/mhr-cfw-go/internal/constants"
)

type ProbeResult struct {
	IP        string
	LatencyMS *int
	Error     string
}

func (r ProbeResult) OK() bool {
	return r.LatencyMS != nil
}

func ScanSync(frontDomain string) bool {
	results := run(frontDomain)

	okCount := 0
	fmt.Printf("\nIP                  LATENCY      STATUS\n")
	fmt.Printf("------------------- ----------- -------------------------\n")
	for _, r := range results {
		if r.OK() {
			fmt.Printf("%-19s %8dms   OK\n", r.IP, *r.LatencyMS)
			okCount++
		} else {
			fmt.Printf("%-19s %-11s %s\n", r.IP, "-", r.Error)
		}
	}
	fmt.Printf("\nResult: %d / %d reachable\n", okCount, len(results))

	if okCount == 0 {
		fmt.Println("No Google IPs reachable from this network.\n")
		return false
	}

	fastest := []ProbeResult{}
	for _, r := range results {
		if r.OK() {
			fastest = append(fastest, r)
		}
		if len(fastest) == 3 {
			break
		}
	}
	fmt.Println("\nTop 3 fastest IPs:")
	for i, r := range fastest {
		fmt.Printf("  %d. %s (%dms)\n", i+1, r.IP, *r.LatencyMS)
	}
	fmt.Printf("\nRecommended: Set \"google_ip\": \"%s\" in config.json\n\n", fastest[0].IP)
	return true
}

func run(frontDomain string) []ProbeResult {
	timeout := time.Duration(constants.GoogleScannerTimeout) * time.Second
	sem := make(chan struct{}, constants.GoogleScannerConcurrency)
	results := make([]ProbeResult, 0, len(constants.CandidateIPs))
	ch := make(chan ProbeResult, len(constants.CandidateIPs))

	for _, ip := range constants.CandidateIPs {
		ip := ip
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			ch <- probeIP(ip, frontDomain, timeout)
		}()
	}

	for i := 0; i < len(constants.CandidateIPs); i++ {
		results = append(results, <-ch)
	}

	sort.Slice(results, func(i, j int) bool {
		ri, rj := results[i], results[j]
		if ri.OK() != rj.OK() {
			return ri.OK()
		}
		if !ri.OK() {
			return ri.IP < rj.IP
		}
		return *ri.LatencyMS < *rj.LatencyMS
	})
	return results
}

func probeIP(ip, sni string, timeout time.Duration) ProbeResult {
	start := time.Now()
	dialer := &net.Dialer{Timeout: timeout}
	raw, err := dialer.Dial("tcp", net.JoinHostPort(ip, "443"))
	if err != nil {
		return ProbeResult{IP: ip, Error: "network error"}
	}
	defer raw.Close()

	cfg := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
	}
	conn := tls.Client(raw, cfg)
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := conn.Handshake(); err != nil {
		return ProbeResult{IP: ip, Error: "handshake failed"}
	}

	req := fmt.Sprintf("HEAD / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", sni)
	if _, err := conn.Write([]byte(req)); err != nil {
		return ProbeResult{IP: ip, Error: "write failed"}
	}
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return ProbeResult{IP: ip, Error: "empty response"}
	}
	if !strings.HasPrefix(string(buf[:n]), "HTTP/") {
		return ProbeResult{IP: ip, Error: "invalid response"}
	}
	ms := int(time.Since(start).Milliseconds())
	return ProbeResult{IP: ip, LatencyMS: &ms}
}
