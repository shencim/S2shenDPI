//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
)

// SO_MARK değeri: raw socket ile gönderilen paketleri NFQUEUE'dan hariç tutmak için
const rawSockMark = 0x4449 // "DI" = S2shenInjected

var rawSock int = -1

func openRawSocket() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		return fmt.Errorf("raw socket açılamadı (root gerekli): %w", err)
	}
	// IP_HDRINCL: kendi IP başlığımızı biz yazıyoruz
	if err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
		syscall.Close(fd)
		return fmt.Errorf("IP_HDRINCL ayarlanamadı: %w", err)
	}
	// SO_MARK: iptables bypass işareti — bu paketler NFQUEUE kurallarına takılmaz
	if err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_MARK, rawSockMark); err != nil {
		syscall.Close(fd)
		return fmt.Errorf("SO_MARK ayarlanamadı: %w", err)
	}
	rawSock = fd
	return nil
}

func closeRawSocket() {
	if rawSock >= 0 {
		syscall.Close(rawSock)
		rawSock = -1
	}
}

// rawSocketReady, sahte paket/parçalama enjeksiyonu için raw socket'in
// kullanılabilir olup olmadığını bildirir. openRawSocket başarısız olduysa
// (örn. yetersiz izin) false döner ve çağıran taraf orijinal paketi
// değiştirmeden (bypass uygulamadan) geçirmelidir — aksi halde paket
// sessizce kaybolur ve bağlantı tamamen kopar.
func rawSocketReady() bool {
	return rawSock >= 0
}

func rawSend(pkt []byte) error {
	if rawSock < 0 {
		return fmt.Errorf("raw socket açık değil")
	}
	if len(pkt) < 20 {
		return fmt.Errorf("paket çok kısa")
	}
	dstIP := net.IP(pkt[16:20])
	addr := &syscall.SockaddrInet4{}
	copy(addr.Addr[:], dstIP.To4())
	return syscall.Sendto(rawSock, pkt, 0, addr)
}

// IP checksum hesapla ve pakete yaz
func recalcIPChecksum(pkt []byte) {
	if len(pkt) < 20 {
		return
	}
	ihl := int(pkt[0]&0x0F) * 4
	if len(pkt) < ihl {
		return
	}
	pkt[10] = 0
	pkt[11] = 0
	var sum uint32
	for i := 0; i < ihl; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(pkt[i : i+2]))
	}
	for sum>>16 != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	binary.BigEndian.PutUint16(pkt[10:12], ^uint16(sum))
}

// TCP checksum hesapla ve pakete yaz
func recalcTCPChecksum(pkt []byte) {
	ihl := int(pkt[0]&0x0F) * 4
	if len(pkt) < ihl+20 {
		return
	}
	tcpLen := len(pkt) - ihl
	tcpSeg := pkt[ihl:]

	tcpSeg[16] = 0
	tcpSeg[17] = 0

	// Pseudo header: src IP + dst IP + 0x00 + proto(6) + tcp length
	var sum uint32
	for i := 12; i < 20; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(pkt[i : i+2]))
	}
	sum += uint32(syscall.IPPROTO_TCP)
	sum += uint32(tcpLen)

	for i := 0; i+1 < len(tcpSeg); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(tcpSeg[i : i+2]))
	}
	if len(tcpSeg)%2 == 1 {
		sum += uint32(tcpSeg[len(tcpSeg)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	binary.BigEndian.PutUint16(tcpSeg[16:18], ^uint16(sum))
}
