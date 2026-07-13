//go:build linux

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

type Config struct {
	Mode        string
	AutoTTL     bool
	BlockQUIC   bool
	WrongChksum bool
	WrongSeq    bool
	DNSRedirect bool
	DNSAddr     string
	// ProxyPort şu an paket filtrelemede kullanılmıyor: bkz. main.go (Windows) açıklaması.
	ProxyPort int
	ChunkSize int
	DpiTier   string
}

func main() {
	mode := flag.String("mode", "game", "game veya super")
	autoTTL := flag.Bool("auto-ttl", true, "auto-ttl etkinleştir")
	blockQUIC := flag.Bool("block-quic", true, "QUIC/HTTP3 engelle")
	wrongChksum := flag.Bool("wrong-chksum", true, "sahte yanlış-checksum paketi gönder")
	wrongSeq := flag.Bool("wrong-seq", true, "sahte yanlış-seq paketi gönder")
	dnsRedirect := flag.Bool("dns-redirect", false, "sistem DNS'ini değiştir")
	dnsAddr := flag.String("dns-addr", "1.1.1.1", "DNS sunucu adresi")
	proxyPort := flag.Int("proxy-port", 0, "SpoofDPI proxy portu")
	chunkSize := flag.Int("chunk-size", 2, "TLS ClientHello bölünme noktası (bayt, Dengeli/Güçlü mod)")
	dpiTier := flag.String("dpi-tier", "1", "0=Turbo (yalnızca AutoTTL), 1=Dengeli (split), 2=Güçlü (split+fake)")
	pidFile := flag.String("pid-file", "", "PID dosyası yolu")
	flag.Parse()

	if *pidFile != "" {
		_ = os.WriteFile(*pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
		defer os.Remove(*pidFile)
	}

	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "[ERROR] s2shen-divert root olarak çalışmalıdır (sudo veya CAP_NET_ADMIN)")
		os.Exit(1)
	}

	if *dnsRedirect {
		if err := setLinuxDNS(*dnsAddr); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] DNS ayarlanamadı: %v\n", err)
		}
	}

	if *chunkSize < 1 {
		*chunkSize = 1
	}

	cfg := &Config{
		Mode:        *mode,
		AutoTTL:     *autoTTL,
		BlockQUIC:   *blockQUIC,
		WrongChksum: *wrongChksum,
		WrongSeq:    *wrongSeq,
		DNSRedirect: *dnsRedirect,
		DNSAddr:     *dnsAddr,
		ProxyPort:   *proxyPort,
		ChunkSize:   *chunkSize,
		DpiTier:     *dpiTier,
	}

	fmt.Printf("[S2shenDivert] Mod: %s | DpiTier: %s | ChunkSize: %d | AutoTTL: %v | BlockQUIC: %v | WrongChksum: %v | WrongSeq: %v\n",
		cfg.Mode, cfg.DpiTier, cfg.ChunkSize, cfg.AutoTTL, cfg.BlockQUIC, cfg.WrongChksum, cfg.WrongSeq)

	// iptables kurallarını ekle — defer ile temizlik garantilenir
	// (signal, panic veya normal çıkış → defer her zaman çalışır)
	manageIptables(true, cfg)
	defer manageIptables(false, cfg)

	// Sinyal alındığında ctx iptal edilir → runEngine durur → main devam eder → defer çalışır
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runEngine(ctx, cfg)

	if *dnsRedirect {
		restoreLinuxDNS()
	}

	fmt.Println("[S2shenDivert] Durduruldu.")
}
