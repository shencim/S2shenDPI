//go:build windows

package main

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
)

const (
	regNetInterfaces = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`
	regNameServers   = "NameServer"
	regDhcpNameServers = "DhcpNameServer"
)

func setWindowsDNS(dnsIP string) error {
	if dnsIP == "" {
		return nil
	}

	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regNetInterfaces, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("registry açılamadı: %w", err)
	}
	defer k.Close()

	subkeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return err
	}

	changed := 0
	for _, sub := range subkeys {
		sk, err := registry.OpenKey(k, sub, registry.ALL_ACCESS)
		if err != nil {
			continue
		}

		existing, _, _ := sk.GetStringValue(regNameServers)
		if existing != "" || isDHCPInterface(sk) {
			_ = sk.SetStringValue(regNameServers, dnsIP)
			changed++
		}
		sk.Close()
	}

	if changed == 0 {
		k2, err := registry.OpenKey(registry.LOCAL_MACHINE,
			`SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`, registry.ALL_ACCESS)
		if err == nil {
			_ = k2.SetStringValue(regNameServers, dnsIP)
			k2.Close()
		}
	}

	fmt.Printf("[DNS] Windows DNS → %s (%d arayüz güncellendi)\n", dnsIP, changed)
	return nil
}

func isDHCPInterface(k registry.Key) bool {
	v, _, err := k.GetStringValue(regDhcpNameServers)
	return err == nil && v != ""
}

func restoreWindowsDNS() {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regNetInterfaces, registry.ALL_ACCESS)
	if err != nil {
		return
	}
	defer k.Close()

	subkeys, _ := k.ReadSubKeyNames(-1)
	for _, sub := range subkeys {
		sk, err := registry.OpenKey(k, sub, registry.ALL_ACCESS)
		if err != nil {
			continue
		}
		_ = sk.DeleteValue(regNameServers)
		sk.Close()
	}
}
