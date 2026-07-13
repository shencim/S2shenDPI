//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func setLinuxDNS(dnsIP string) error {
	if dnsIP == "" {
		return nil
	}

	ifaces, err := getActiveInterfaces()
	if err != nil || len(ifaces) == 0 {
		return writeDNSToResolvConf(dnsIP)
	}

	changed := 0
	for _, iface := range ifaces {
		out, err := exec.Command("resolvectl", "dns", iface, dnsIP).CombinedOutput()
		if err != nil {
			fmt.Printf("[DNS] resolvectl %s hatası: %v (%s)\n", iface, err, strings.TrimSpace(string(out)))
			continue
		}
		changed++
	}

	if changed == 0 {
		return writeDNSToResolvConf(dnsIP)
	}

	fmt.Printf("[DNS] Linux DNS → %s (%d arayüz güncellendi)\n", dnsIP, changed)
	return nil
}

func restoreLinuxDNS() {
	ifaces, _ := getActiveInterfaces()
	for _, iface := range ifaces {
		_ = exec.Command("resolvectl", "revert", iface).Run()
	}
	// resolv.conf yedeklenmiş ise geri yükle
	if _, err := os.Stat("/tmp/s2shendpi_resolv.bak"); err == nil {
		data, err := os.ReadFile("/tmp/s2shendpi_resolv.bak")
		if err == nil {
			_ = os.WriteFile("/etc/resolv.conf", data, 0644)
			_ = os.Remove("/tmp/s2shendpi_resolv.bak")
		}
	}
}

func getActiveInterfaces() ([]string, error) {
	out, err := exec.Command("sh", "-c",
		"ip -o link show up | awk -F': ' '{print $2}' | grep -v lo | grep -v docker | grep -v veth | head -5",
	).Output()
	if err != nil {
		return nil, err
	}
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}

func writeDNSToResolvConf(dnsIP string) error {
	// Mevcut resolv.conf'u yedekle
	if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		_ = os.WriteFile("/tmp/s2shendpi_resolv.bak", data, 0644)
	}
	content := fmt.Sprintf("nameserver %s\n", dnsIP)
	return os.WriteFile("/etc/resolv.conf", []byte(content), 0644)
}
