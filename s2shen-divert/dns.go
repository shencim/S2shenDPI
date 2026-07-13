//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	regNetInterfaces   = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`
	regNameServers     = "NameServer"
	regDhcpNameServers = "DhcpNameServer"
)

// dnsBackupPath, orijinal statik DNS değerlerinin restore edilebilmesi için
// interface adı -> orijinal NameServer değeri eşlemesini tutan yedek dosyasının yolu.
// Değer boş string ise, orijinalde NameServer hiç ayarlı değildi (restore'da silinmeli).
func dnsBackupPath() string {
	return filepath.Join(os.TempDir(), "s2shendpi_dns_backup.json")
}

// backupWindowsDNS, mevcut NameServer değerlerini (varsa) diske yazar.
// Zaten bir yedek varsa üzerine yazmaz — böylece art arda setWindowsDNS
// çağrılarında orijinal (S2shenDPI öncesi) değer kaybolmaz.
func backupWindowsDNS(k registry.Key, subkeys []string) {
	if _, err := os.Stat(dnsBackupPath()); err == nil {
		return
	}

	backup := make(map[string]string, len(subkeys))
	for _, sub := range subkeys {
		sk, err := registry.OpenKey(k, sub, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		existing, _, _ := sk.GetStringValue(regNameServers)
		backup[sub] = existing
		sk.Close()
	}

	data, err := json.Marshal(backup)
	if err != nil {
		return
	}
	_ = os.WriteFile(dnsBackupPath(), data, 0600)
}

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

	backupWindowsDNS(k, subkeys)

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

// restoreWindowsDNS, backupWindowsDNS tarafından kaydedilen orijinal NameServer
// değerlerini geri yazar. Yedek dosyası yoksa (örn. eski sürümden geçiş,
// ya da hiç değiştirilmemişse) eski davranışa döner: NameServer değerini siler
// (arayüz DHCP DNS'ine düşer) — statik DNS kaybı riski sadece bu durumda kalır.
func restoreWindowsDNS() {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regNetInterfaces, registry.ALL_ACCESS)
	if err != nil {
		return
	}
	defer k.Close()

	backup := loadWindowsDNSBackup()
	subkeys, _ := k.ReadSubKeyNames(-1)
	for _, sub := range subkeys {
		sk, err := registry.OpenKey(k, sub, registry.ALL_ACCESS)
		if err != nil {
			continue
		}

		if original, ok := backup[sub]; ok {
			if original == "" {
				_ = sk.DeleteValue(regNameServers)
			} else {
				_ = sk.SetStringValue(regNameServers, original)
			}
		} else {
			_ = sk.DeleteValue(regNameServers)
		}
		sk.Close()
	}

	if backup != nil {
		_ = os.Remove(dnsBackupPath())
	}
}

func loadWindowsDNSBackup() map[string]string {
	data, err := os.ReadFile(dnsBackupPath())
	if err != nil {
		return nil
	}
	var backup map[string]string
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil
	}
	return backup
}
