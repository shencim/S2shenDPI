//go:build windows

package main

import (
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
	ProxyPort   int
}

func main() {
	mode := flag.String("mode", "game", "game or super")
	autoTTL := flag.Bool("auto-ttl", true, "enable auto-ttl")
	blockQUIC := flag.Bool("block-quic", true, "block QUIC/HTTP3")
	wrongChksum := flag.Bool("wrong-chksum", true, "send fake wrong-checksum packet")
	wrongSeq := flag.Bool("wrong-seq", true, "send fake wrong-seq packet")
	dnsRedirect := flag.Bool("dns-redirect", false, "change system DNS")
	dnsAddr := flag.String("dns-addr", "1.1.1.1", "DNS server address")
	proxyPort := flag.Int("proxy-port", 0, "SpoofDPI proxy port (super mode)")
	pidFile := flag.String("pid-file", "", "write PID to this file")
	flag.Parse()

	if *pidFile != "" {
		_ = os.WriteFile(*pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
		defer os.Remove(*pidFile)
	}

	if err := loadWinDivert(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] WinDivert yuklenemedi: %v\n", err)
		os.Exit(1)
	}

	if *dnsRedirect {
		if err := setWindowsDNS(*dnsAddr); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] DNS ayarlanamadi: %v\n", err)
		}
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
	}

	fmt.Printf("[DarknesDivert] Mod: %s | AutoTTL: %v | BlockQUIC: %v | WrongChksum: %v | WrongSeq: %v\n",
		cfg.Mode, cfg.AutoTTL, cfg.BlockQUIC, cfg.WrongChksum, cfg.WrongSeq)

	done := make(chan struct{})
	go func() {
		runEngine(cfg)
		close(done)
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sig:
	case <-done:
	}

	if *dnsRedirect {
		restoreWindowsDNS()
	}

	fmt.Println("[DarknesDivert] Durduruldu.")
}
