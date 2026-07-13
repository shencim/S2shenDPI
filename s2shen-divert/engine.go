//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"sync"
	"syscall"
	"time"
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
)

type ttlEntry struct {
	hopCount uint8
}

var (
	ttlTable   = make(map[uint32]ttlEntry)
	ttlTableMu sync.Mutex
)

const (
	filterNoLocal = "!impostor and !loopback"

	filterOutTCP = "outbound and " + filterNoLocal +
		" and tcp and (tcp.DstPort == 443 or tcp.DstPort == 80) and tcp.PayloadLength > 0"

	filterInSYNACK = "inbound and " + filterNoLocal +
		" and tcp and (tcp.SrcPort == 443 or tcp.SrcPort == 80) and tcp.Syn and tcp.Ack"

	filterDropRST = "inbound and " + filterNoLocal +
		" and tcp and (tcp.SrcPort == 443 or tcp.SrcPort == 80) and tcp.Rst"

	filterQUIC = "outbound and " + filterNoLocal +
		" and udp and udp.DstPort == 443 and udp.PayloadLength >= 1200 and udp.Payload[0] >= 0xC0"
)

func runEngine(cfg *Config) {
	var wg sync.WaitGroup

	// RST dropper, gelen TÜM inbound RST paketlerini (443/80, hangi uygulamaya
	// ait olursa olsun) düşürür — SNI bazlı hedeflenemez (RST paketinin payload'ı
	// yoktur, hangi bağlantıya ait olduğu bağlantı takibi olmadan bilinemez).
	// Discord Split (SNIFilter) modunda "yalnızca Discord'a dokun" ilkesini
	// bozmamak için bu global davranış devre dışı bırakılır.
	if !cfg.SNIFilter {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runRSTDropper()
		}()
	}

	if cfg.BlockQUIC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runQUICBlocker()
		}()
	}

	if cfg.AutoTTL || cfg.WrongChksum || cfg.WrongSeq || cfg.SNIFilter {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runTCPEngine(cfg)
		}()
	}

	wg.Wait()
}

func openWithRetry(filter string, flags int) (syscall.Handle, error) {
	var h syscall.Handle
	var err error
	for i := 0; i < 6; i++ {
		h, err = divertOpen(filter, WINDIVERT_LAYER_NETWORK, 0, flags)
		if err == nil {
			return h, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return syscall.InvalidHandle, err
}

func runRSTDropper() {
	h, err := openWithRetry(filterDropRST, WINDIVERT_FLAG_DROP)
	if err != nil {
		fmt.Printf("[DIVERT] RST dropper açılamadı: %v\n", err)
		return
	}
	defer divertClose(h)
	fmt.Println("[DIVERT] Pasif DPI savunması aktif (RST drop)")

	pkt := make([]byte, MAX_PACKET_SIZE)
	addr := make([]byte, WINDIVERT_ADDR_SIZE)
	for {
		_, err := divertRecv(h, pkt, addr)
		if err != nil {
			continue
		}
	}
}

func runQUICBlocker() {
	h, err := openWithRetry(filterQUIC, WINDIVERT_FLAG_DEFAULT)
	if err != nil {
		fmt.Printf("[DIVERT] QUIC blocker açılamadı: %v\n", err)
		return
	}
	defer divertClose(h)
	fmt.Println("[DIVERT] QUIC/HTTP3 blocker aktif (UDP:443)")

	pkt := make([]byte, MAX_PACKET_SIZE)
	addr := make([]byte, WINDIVERT_ADDR_SIZE)

	for {
		n, err := divertRecv(h, pkt, addr)
		if err != nil {
			continue
		}
		payload := udpPayload(pkt[:n])
		if isQUICInitial(payload) {
			continue
		}
		_ = divertSend(h, pkt[:n], addr)
	}
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

func runTCPEngine(cfg *Config) {
	filter := filterOutTCP
	if cfg.AutoTTL {
		filter = "(" + filterOutTCP + ") or (" + filterInSYNACK + ")"
	}

	h, err := openWithRetry(filter, WINDIVERT_FLAG_DEFAULT)
	if err != nil {
		fmt.Printf("[DIVERT] TCP engine açılamadı: %v\n", err)
		return
	}
	defer divertClose(h)

	fmt.Println("[DIVERT] TCP engine aktif (TLS bypass + fragmentation)")

	pkt := make([]byte, MAX_PACKET_SIZE)
	addr := make([]byte, WINDIVERT_ADDR_SIZE)

	for {
		n, err := divertRecv(h, pkt, addr)
		if err != nil {
			continue
		}

		data := pkt[:n]

		if addrIsOutbound(addr) {
			processTCPOutbound(h, data, addr, cfg)
		} else {
			processTCPInbound(data, addr)
			_ = divertSend(h, data, addr)
		}
	}
}

func processTCPInbound(pkt []byte, addr []byte) {
	if pktProto(pkt) != PROTO_TCP {
		return
	}
	flags := tcpFlags(pkt)
	if flags&TCP_FLAG_SYN == 0 || flags&TCP_FLAG_ACK == 0 {
		return
	}
	ttl := pktTTL(pkt)
	if ttl == 0 {
		return
	}

	hop := estimateHopCount(ttl)
	dstIP := pktSrcIPv4(pkt)

	ttlTableMu.Lock()
	ttlTable[dstIP] = ttlEntry{hopCount: hop}
	ttlTableMu.Unlock()
}

func processTCPOutbound(h syscall.Handle, pkt []byte, addr []byte, cfg *Config) {
	if pktProto(pkt) != PROTO_TCP {
		_ = divertSend(h, pkt, addr)
		return
	}

	payload := tcpPayload(pkt)
	if len(payload) < 6 {
		_ = divertSend(h, pkt, addr)
		return
	}

	isTLS := payload[0] == TLS_HANDSHAKE && len(payload) > 5 && payload[5] == TLS_CLIENT_HELLO

	if !isTLS {
		if cfg.AutoTTL {
			applyAutoTTL(pkt, addr)
			divertCalcChecksums(pkt, addr, 0)
		}
		_ = divertSend(h, pkt, addr)
		return
	}

	// Discord Split (SNIFilter): yalnızca SNI'si allowlist'te olan bağlantılara
	// dokunulur. SNI parse edilemezse (segmentli/anomali ClientHello) ya da
	// allowlist'te değilse, paket hiç değiştirilmeden geçirilir — bu sayede
	// oyun/tarayıcı trafiği bu modda kesinlikle etkilenmez.
	if cfg.SNIFilter {
		hostname, ok := parseSNI(payload)
		if !ok || !matchesSNIAllowlist(hostname, discordSNIAllowlist) {
			_ = divertSend(h, pkt, addr)
			return
		}
	}

	// Turbo (DpiTier "0"): en düşük gecikme için fragmentation/fake paket
	// enjeksiyonu yapılmaz, yalnızca pasif TTL ayarı uygulanır.
	if cfg.DpiTier == "0" {
		if cfg.AutoTTL {
			applyAutoTTL(pkt, addr)
			divertCalcChecksums(pkt, addr, 0)
		}
		_ = divertSend(h, pkt, addr)
		return
	}

	if cfg.WrongChksum {
		sendFakeWrongChksum(h, pkt, addr)
	}

	if cfg.WrongSeq {
		sendFakeWrongSeq(h, pkt, addr)
	}

	if cfg.AutoTTL {
		applyAutoTTL(pkt, addr)
	}

	sendFragmented(h, pkt, addr, cfg)
}

func sendFakeWrongChksum(h syscall.Handle, pkt []byte, addr []byte) {
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

	fakeAddr := make([]byte, WINDIVERT_ADDR_SIZE)
	copy(fakeAddr, addr)

	applyFakeTTL(fake, addr)
	divertCalcChecksums(fake, fakeAddr, 0)

	if len(fake) >= ihl+18 {
		chk := binary.BigEndian.Uint16(fake[ihl+16 : ihl+18])
		binary.BigEndian.PutUint16(fake[ihl+16:ihl+18], chk-1)
	}

	_ = divertSend(h, fake, fakeAddr)
}

func sendFakeWrongSeq(h syscall.Handle, pkt []byte, addr []byte) {
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

	fakeAddr := make([]byte, WINDIVERT_ADDR_SIZE)
	copy(fakeAddr, addr)

	seqOffset := ihl + 4
	ackOffset := ihl + 8
	origSeq := binary.BigEndian.Uint32(pkt[seqOffset : seqOffset+4])
	origAck := binary.BigEndian.Uint32(pkt[ackOffset : ackOffset+4])
	binary.BigEndian.PutUint32(fake[seqOffset:seqOffset+4], origSeq-10000)
	binary.BigEndian.PutUint32(fake[ackOffset:ackOffset+4], origAck-66000)

	applyFakeTTL(fake, addr)
	divertCalcChecksums(fake, fakeAddr, 0)
	_ = divertSend(h, fake, fakeAddr)
}

func applyFakeTTL(pkt []byte, addr []byte) {
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

func sendFragmented(h syscall.Handle, pkt []byte, addr []byte, cfg *Config) {
	ihl := ipv4HeaderLen(pkt)
	thl := tcpDataOffset(pkt)
	headerSize := ihl + thl
	payload := pkt[headerSize:]

	splitAt := cfg.ChunkSize
	if splitAt < 1 {
		splitAt = 1
	}

	if len(payload) <= splitAt || headerSize+splitAt > len(pkt) {
		divertCalcChecksums(pkt, addr, 0)
		_ = divertSend(h, pkt, addr)
		return
	}

	frag1 := make([]byte, headerSize+splitAt)
	copy(frag1, pkt[:headerSize])
	copy(frag1[headerSize:], payload[:splitAt])
	binary.BigEndian.PutUint16(frag1[2:4], uint16(headerSize+splitAt))
	divertCalcChecksums(frag1, addr, 0)

	remaining := payload[splitAt:]
	frag2 := make([]byte, headerSize+len(remaining))
	copy(frag2, pkt[:headerSize])
	copy(frag2[headerSize:], remaining)
	binary.BigEndian.PutUint16(frag2[2:4], uint16(headerSize+len(remaining)))
	seqOff := ihl + 4
	origSeq := binary.BigEndian.Uint32(pkt[seqOff : seqOff+4])
	binary.BigEndian.PutUint32(frag2[seqOff:seqOff+4], origSeq+uint32(splitAt))
	divertCalcChecksums(frag2, addr, 0)

	_ = divertSend(h, frag2, addr)
	_ = divertSend(h, frag1, addr)
}

func applyAutoTTL(pkt []byte, addr []byte) {
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

func pktDstIPv4(pkt []byte) uint32 {
	if pktIPVersion(pkt) != 4 || len(pkt) < 20 {
		return 0
	}
	return binary.BigEndian.Uint32(pkt[16:20])
}

func pktSrcIPv4(pkt []byte) uint32 {
	if pktIPVersion(pkt) != 4 || len(pkt) < 16 {
		return 0
	}
	return binary.BigEndian.Uint32(pkt[12:16])
}
