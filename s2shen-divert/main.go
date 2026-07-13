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
	// ProxyPort şu an paket filtrelemede kullanılmıyor: SpoofDPI proxy'sinin
	// upstream bağlantıları rastgele ephemeral portlardan çıktığı için basit
	// port filtresiyle ayırt edilemiyor (gerçek ayrım için WINDIVERT_LAYER_SOCKET
	// tabanlı process-id filtrelemesi gerekir). Rust tarafıyla CLI sözleşmesini
	// korumak için parametre kabul ediliyor, ileride process-aware filtreleme
	// eklendiğinde kullanılacak.
	ProxyPort int
	// ChunkSize, TLS ClientHello'nun kaçıncı bayttan ikiye bölüneceğini belirler
	// (Dengeli/Güçlü mod). Turbo modunda (DpiTier "0") fragmentation hiç uygulanmaz.
	ChunkSize int
	// DpiTier, arayüzdeki Turbo(0)/Dengeli(1)/Güçlü(2) seçimini taşır.
	// "0": yalnızca AutoTTL, fragmentation/fake paket yok (en düşük gecikme).
	// "1"/"2": fragmentation + (Güçlü'de ayrıca) fake paket enjeksiyonu.
	DpiTier string
	// SNIFilter true ise ("Discord Split" modu) yalnızca SNI'si discordSNIAllowlist
	// ile eşleşen TLS ClientHello'lar işlenir; geri kalan tüm trafik (oyun dahil)
	// hiç dokunulmadan geçirilir.
	SNIFilter bool
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
	chunkSize := flag.Int("chunk-size", 2, "TLS ClientHello split point in bytes (Dengeli/Guclu mode)")
	dpiTier := flag.String("dpi-tier", "1", "0=Turbo (AutoTTL only), 1=Dengeli (split), 2=Guclu (split+fake)")
	sniFilter := flag.Bool("sni-filter", false, "only touch traffic whose SNI matches the built-in Discord allowlist")
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
		SNIFilter:   *sniFilter,
	}

	fmt.Printf("[S2shenDivert] Mod: %s | DpiTier: %s | ChunkSize: %d | SNIFilter: %v | AutoTTL: %v | BlockQUIC: %v | WrongChksum: %v | WrongSeq: %v\n",
		cfg.Mode, cfg.DpiTier, cfg.ChunkSize, cfg.SNIFilter, cfg.AutoTTL, cfg.BlockQUIC, cfg.WrongChksum, cfg.WrongSeq)

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

	fmt.Println("[S2shenDivert] Durduruldu.")
}
