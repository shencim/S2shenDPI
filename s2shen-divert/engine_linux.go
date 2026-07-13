//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"sync"

	nfqueue "github.com/florianl/go-nfqueue/v2"
)

const (
	PROTO_TCP = 6
	PROTO_UDP = 17

	TCP_FLAG_SYN = 0x02
	TCP_FLAG_RST = 0x04
	TCP_FLAG_ACK = 0x10

	TLS_HANDSHAKE    = 0x16
	TLS_CLIENT_HELLO = 0x01

	QUIC_LONG_HEADER_FLAG = 0x80
	QUIC_INITIAL_TYPE     = 0x00

	queueTCP  = 100
	queueQUIC = 101
)

type ttlEntry struct {
	hopCount uint8
}

var (
	ttlTable   = make(map[uint32]ttlEntry)
	ttlTableMu sync.Mutex
)

// manageIptables: iptables kurallarını ekler veya kaldırır.
// main() tarafından çağrılır — defer ile temizlik garantilenir.
func manageIptables(add bool, cfg *Config) {
	action := "-A"
	if !add {
		action = "-D"
	}

	// Fwmark ile işaretlenmiş paketleri (raw socket enjeksiyonları) NFQUEUE'dan hariç tut
	// -I ile listenin başına ekle — diğer kurallardan önce işlenir
	markBypass := fmt.Sprintf("0x%x", rawSockMark)
	if add {
		run("iptables", "-I", "OUTPUT", "1", "-m", "mark", "--mark", markBypass, "-j", "ACCEPT")
	} else {
		run("iptables", "-D", "OUTPUT", "-m", "mark", "--mark", markBypass, "-j", "ACCEPT")
	}

	// Outbound TCP 443 — sadece kurulu bağlantıları (ESTABLISHED) yakala
	// conntrack, SYN/SYN-ACK gibi handshake paketlerini otomatik dışlar
	run("iptables", action, "OUTPUT", "-p", "tcp", "--dport", "443",
		"-m", "conntrack", "--ctstate", "ESTABLISHED",
		"-j", "NFQUEUE", "--queue-num", fmt.Sprintf("%d", queueTCP))

	run("iptables", action, "OUTPUT", "-p", "tcp", "--dport", "80",
		"-m", "conntrack", "--ctstate", "ESTABLISHED",
		"-j", "NFQUEUE", "--queue-num", fmt.Sprintf("%d", queueTCP))

	// Inbound SYN+ACK — TTL öğrenmek için (AutoTTL modu)
	run("iptables", action, "INPUT", "-p", "tcp", "--sport", "443",
		"--tcp-flags", "SYN,ACK", "SYN,ACK",
		"-j", "NFQUEUE", "--queue-num", fmt.Sprintf("%d", queueTCP))

	run("iptables", action, "INPUT", "-p", "tcp", "--sport", "80",
		"--tcp-flags", "SYN,ACK", "SYN,ACK",
		"-j", "NFQUEUE", "--queue-num", fmt.Sprintf("%d", queueTCP))

	// Inbound RST'leri drop et (DPI reset enjeksiyonunu engelle)
	run("iptables", action, "INPUT", "-p", "tcp", "--sport", "443",
		"--tcp-flags", "RST", "RST", "-j", "DROP")
	run("iptables", action, "INPUT", "-p", "tcp", "--sport", "80",
		"--tcp-flags", "RST", "RST", "-j", "DROP")

	if cfg.BlockQUIC {
		run("iptables", action, "OUTPUT", "-p", "udp", "--dport", "443",
			"-m", "length", "--length", "1200:65535",
			"-j", "NFQUEUE", "--queue-num", fmt.Sprintf("%d", queueQUIC))
	}
}

var rawSocketWarnOnce sync.Once

// warnRawSocketUnavailableOnce, raw socket olmadan bypass'ın etkin şekilde
// devre dışı kaldığını yalnızca bir kez, açıkça işaretlenmiş şekilde loglar
// (her paket için tekrar tekrar spam basmak yerine).
func warnRawSocketUnavailableOnce() {
	rawSocketWarnOnce.Do(func() {
		fmt.Println("[ENGINE] KRİTİK: raw socket kullanılamıyor, DPI bypass devre dışı — trafik değiştirilmeden geçiriliyor. 'sudo bash scripts/s2shendpi-setup.sh' ile CAP_NET_RAW yetkisini kontrol edin.")
	})
}

func run(cmd string, args ...string) {
	if out, err := exec.Command(cmd, args...).CombinedOutput(); err != nil {
		fmt.Printf("[IPTABLES] %s %v: %v (%s)\n", cmd, args, err, string(out))
	}
}

// runEngine: iptables'ı yönetmez — o main()'in sorumluluğu.
// Sadece NFQUEUE consumer'ları çalıştırır, ctx iptal edilince durur.
func runEngine(ctx context.Context, cfg *Config) {
	if err := openRawSocket(); err != nil {
		fmt.Printf("[ENGINE] KRİTİK: raw socket açılamadı: %v — DPI bypass devre dışı, trafik değiştirilmeden geçirilecek\n", err)
	} else {
		defer closeRawSocket()
	}

	// Kernel conntrack'in out-of-window segment'leri düşürmemesi için
	_ = exec.Command("sysctl", "-w", "net.netfilter.nf_conntrack_tcp_be_liberal=1").Run()

	fmt.Println("[ENGINE] NFQUEUE engine başlatılıyor...")

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		runTCPQueue(ctx, cfg)
	}()

	if cfg.BlockQUIC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runQUICQueue(ctx)
		}()
	}

	wg.Wait()
}

func runTCPQueue(ctx context.Context, cfg *Config) {
	config := nfqueue.Config{
		NfQueue:      queueTCP,
		MaxPacketLen: 65535,
		MaxQueueLen:  512,
		Copymode:     nfqueue.NfQnlCopyPacket,
	}

	nf, err := nfqueue.Open(&config)
	if err != nil {
		fmt.Printf("[NFQUEUE] TCP queue açılamadı: %v\n", err)
		return
	}
	defer nf.Close()

	fmt.Printf("[NFQUEUE] TCP queue %d aktif\n", queueTCP)

	err = nf.RegisterWithErrorFunc(ctx, func(a nfqueue.Attribute) int {
		if a.PacketID == nil || a.Payload == nil {
			return 0
		}
		id := *a.PacketID
		pkt := make([]byte, len(*a.Payload))
		copy(pkt, *a.Payload)

		if isInboundSYNACK(pkt) {
			processTCPInbound(pkt)
			_ = nf.SetVerdict(id, nfqueue.NfAccept)
			return 0
		}

		processTCPOutbound(nf, id, pkt, cfg)
		return 0
	}, func(err error) int {
		fmt.Printf("[NFQUEUE] TCP hata: %v\n", err)
		return 0
	})

	if err != nil {
		fmt.Printf("[NFQUEUE] TCP register hatası: %v\n", err)
		return
	}

	<-ctx.Done()
}

func runQUICQueue(ctx context.Context) {
	config := nfqueue.Config{
		NfQueue:      queueQUIC,
		MaxPacketLen: 65535,
		MaxQueueLen:  256,
		Copymode:     nfqueue.NfQnlCopyPacket,
	}

	nf, err := nfqueue.Open(&config)
	if err != nil {
		fmt.Printf("[NFQUEUE] QUIC queue açılamadı: %v\n", err)
		return
	}
	defer nf.Close()

	fmt.Printf("[NFQUEUE] QUIC blocker queue %d aktif\n", queueQUIC)

	err = nf.RegisterWithErrorFunc(ctx, func(a nfqueue.Attribute) int {
		if a.PacketID == nil || a.Payload == nil {
			return 0
		}
		id := *a.PacketID
		payload := udpPayload(*a.Payload)
		if isQUICInitial(payload) {
			_ = nf.SetVerdict(id, nfqueue.NfDrop)
		} else {
			_ = nf.SetVerdict(id, nfqueue.NfAccept)
		}
		return 0
	}, func(err error) int {
		fmt.Printf("[NFQUEUE] QUIC hata: %v\n", err)
		return 0
	})

	if err != nil {
		fmt.Printf("[NFQUEUE] QUIC register hatası: %v\n", err)
		return
	}

	<-ctx.Done()
}

// SYN+ACK bayrağı kombinasyonu: sunucudan gelen el sıkışma yanıtı (inbound)
func isInboundSYNACK(pkt []byte) bool {
	if pktProto(pkt) != PROTO_TCP {
		return false
	}
	flags := tcpFlags(pkt)
	return flags&TCP_FLAG_SYN != 0 && flags&TCP_FLAG_ACK != 0
}

func processTCPInbound(pkt []byte) {
	ttl := pktTTL(pkt)
	if ttl == 0 {
		return
	}
	hop := estimateHopCount(ttl)
	srcIP := pktSrcIPv4(pkt)
	ttlTableMu.Lock()
	ttlTable[srcIP] = ttlEntry{hopCount: hop}
	ttlTableMu.Unlock()
}

func processTCPOutbound(nf *nfqueue.Nfqueue, id uint32, pkt []byte, cfg *Config) {
	if pktProto(pkt) != PROTO_TCP {
		_ = nf.SetVerdict(id, nfqueue.NfAccept)
		return
	}

	payload := tcpPayload(pkt)
	if len(payload) < 6 {
		_ = nf.SetVerdict(id, nfqueue.NfAccept)
		return
	}

	isTLS := payload[0] == TLS_HANDSHAKE && len(payload) > 5 && payload[5] == TLS_CLIENT_HELLO

	if !isTLS {
		if cfg.AutoTTL {
			applyAutoTTL(pkt)
			recalcIPChecksum(pkt)
			recalcTCPChecksum(pkt)
			_ = nf.SetVerdictModPacket(id, nfqueue.NfAccept, pkt)
		} else {
			_ = nf.SetVerdict(id, nfqueue.NfAccept)
		}
		return
	}

	// Raw socket kullanılamıyorsa (örn. izin sorunu), enjeksiyon yapılamaz —
	// orijinal paketi olduğu gibi geçir. Aksi halde paketi drop edip hiç
	// yeniden gönderemeyiz ve bağlantı sessizce kopar.
	if !rawSocketReady() {
		warnRawSocketUnavailableOnce()
		_ = nf.SetVerdict(id, nfqueue.NfAccept)
		return
	}

	// Turbo (DpiTier "0"): en düşük gecikme için fragmentation/fake paket
	// enjeksiyonu yapılmaz, yalnızca pasif TTL ayarı uygulanır.
	if cfg.DpiTier == "0" {
		if cfg.AutoTTL {
			applyAutoTTL(pkt)
			recalcIPChecksum(pkt)
			recalcTCPChecksum(pkt)
			_ = nf.SetVerdictModPacket(id, nfqueue.NfAccept, pkt)
		} else {
			_ = nf.SetVerdict(id, nfqueue.NfAccept)
		}
		return
	}

	// TLS ClientHello yakalandı — orijinali drop et, parçalanmış versiyonu inject et
	_ = nf.SetVerdict(id, nfqueue.NfDrop)

	// Sahte paketleri raw socket ile gönder (SO_MARK ile işaretli → NFQUEUE'ya girmez)
	if cfg.WrongChksum {
		sendFakeWrongChksum(pkt)
	}
	if cfg.WrongSeq {
		sendFakeWrongSeq(pkt)
	}

	// Parçalanmış asıl paketi gönder
	sendFragmented(pkt, cfg)
}

func sendFakeWrongChksum(pkt []byte) {
	ihl := ipv4HeaderLen(pkt)
	thl := tcpDataOffset(pkt)
	headerSize := ihl + thl
	if headerSize <= 0 || headerSize > len(pkt) {
		return
	}

	fakePayload := buildFakeClientHello()
	fake := make([]byte, headerSize+len(fakePayload))
	copy(fake, pkt[:headerSize])
	copy(fake[headerSize:], fakePayload)
	binary.BigEndian.PutUint16(fake[2:4], uint16(headerSize+len(fakePayload)))

	applyFakeTTL(fake)
	recalcIPChecksum(fake)
	recalcTCPChecksum(fake)

	// TCP checksum'ı bozuk yap
	if len(fake) >= ihl+18 {
		chk := binary.BigEndian.Uint16(fake[ihl+16 : ihl+18])
		binary.BigEndian.PutUint16(fake[ihl+16:ihl+18], chk-1)
	}

	if err := rawSend(fake); err != nil {
		fmt.Printf("[FAKE-CHKSUM] rawSend hatası: %v\n", err)
	}
}

func sendFakeWrongSeq(pkt []byte) {
	ihl := ipv4HeaderLen(pkt)
	thl := tcpDataOffset(pkt)
	headerSize := ihl + thl
	if headerSize <= 0 || headerSize > len(pkt) || len(pkt) < ihl+12 {
		return
	}

	fakePayload := buildFakeClientHello()
	fake := make([]byte, headerSize+len(fakePayload))
	copy(fake, pkt[:headerSize])
	copy(fake[headerSize:], fakePayload)
	binary.BigEndian.PutUint16(fake[2:4], uint16(headerSize+len(fakePayload)))

	seqOffset := ihl + 4
	ackOffset := ihl + 8
	origSeq := binary.BigEndian.Uint32(pkt[seqOffset : seqOffset+4])
	origAck := binary.BigEndian.Uint32(pkt[ackOffset : ackOffset+4])
	binary.BigEndian.PutUint32(fake[seqOffset:seqOffset+4], origSeq-10000)
	binary.BigEndian.PutUint32(fake[ackOffset:ackOffset+4], origAck-66000)

	applyFakeTTL(fake)
	recalcIPChecksum(fake)
	recalcTCPChecksum(fake)

	if err := rawSend(fake); err != nil {
		fmt.Printf("[FAKE-SEQ] rawSend hatası: %v\n", err)
	}
}

func applyFakeTTL(pkt []byte) {
	dstIP := pktDstIPv4(pkt)
	ttlTableMu.Lock()
	entry, ok := ttlTable[dstIP]
	ttlTableMu.Unlock()

	if !ok || entry.hopCount < 3 {
		setPktTTL(pkt, 1)
		return
	}
	hop := entry.hopCount
	var fakeTTL uint8
	if hop > 4 {
		fakeTTL = hop - 4
	} else {
		fakeTTL = 1
	}
	if fakeTTL < 1 {
		fakeTTL = 1
	}
	setPktTTL(pkt, fakeTTL)
}

func sendFragmented(pkt []byte, cfg *Config) {
	ihl := ipv4HeaderLen(pkt)
	thl := tcpDataOffset(pkt)
	headerSize := ihl + thl
	payload := pkt[headerSize:]

	splitAt := cfg.ChunkSize
	if splitAt < 1 {
		splitAt = 1
	}

	if len(payload) <= splitAt {
		if cfg.AutoTTL {
			applyAutoTTL(pkt)
		}
		recalcIPChecksum(pkt)
		recalcTCPChecksum(pkt)
		if err := rawSend(pkt); err != nil {
			fmt.Printf("[FRAG] rawSend hatası: %v\n", err)
		}
		return
	}

	// Fragment 1: ilk splitAt byte
	frag1 := make([]byte, headerSize+splitAt)
	copy(frag1, pkt[:headerSize])
	copy(frag1[headerSize:], payload[:splitAt])
	binary.BigEndian.PutUint16(frag1[2:4], uint16(headerSize+splitAt))

	// Fragment 2: kalan payload
	remaining := payload[splitAt:]
	frag2 := make([]byte, headerSize+len(remaining))
	copy(frag2, pkt[:headerSize])
	copy(frag2[headerSize:], remaining)
	binary.BigEndian.PutUint16(frag2[2:4], uint16(headerSize+len(remaining)))
	seqOff := ihl + 4
	origSeq := binary.BigEndian.Uint32(pkt[seqOff : seqOff+4])
	binary.BigEndian.PutUint32(frag2[seqOff:seqOff+4], origSeq+uint32(splitAt))

	if cfg.AutoTTL {
		applyAutoTTL(frag1)
		applyAutoTTL(frag2)
	}

	recalcIPChecksum(frag1)
	recalcTCPChecksum(frag1)
	recalcIPChecksum(frag2)
	recalcTCPChecksum(frag2)

	// frag2 önce → Windows engine ile aynı sıra
	if err := rawSend(frag2); err != nil {
		fmt.Printf("[FRAG2] rawSend hatası: %v\n", err)
	}
	if err := rawSend(frag1); err != nil {
		fmt.Printf("[FRAG1] rawSend hatası: %v\n", err)
	}
}

func applyAutoTTL(pkt []byte) {
	dstIP := pktDstIPv4(pkt)
	ttlTableMu.Lock()
	entry, ok := ttlTable[dstIP]
	ttlTableMu.Unlock()

	if !ok {
		return
	}
	hop := entry.hopCount
	var base uint8 = 128
	if hop <= 32 {
		base = 64
	}
	var newTTL uint8
	if hop+2 < base {
		newTTL = base - hop - 2
	} else {
		newTTL = 1
	}
	if newTTL < 1 {
		newTTL = 1
	}
	setPktTTL(pkt, newTTL)
}

func estimateHopCount(ttl uint8) uint8 {
	var base uint8
	switch {
	case ttl <= 32:
		base = 32
	case ttl <= 64:
		base = 64
	case ttl <= 128:
		base = 128
	default:
		base = 255
	}
	return base - ttl
}

func isQUICInitial(payload []byte) bool {
	if len(payload) < 5 {
		return false
	}
	firstByte := payload[0]
	if firstByte&QUIC_LONG_HEADER_FLAG == 0 {
		return false
	}
	pktType := (firstByte & 0x30) >> 4
	if pktType != QUIC_INITIAL_TYPE {
		return false
	}
	version := binary.BigEndian.Uint32(payload[1:5])
	return version == 0x00000001 || version == 0xff00001d ||
		version == 0x6b3343cf || version == 0xff000020 || version == 0xff00001e
}
