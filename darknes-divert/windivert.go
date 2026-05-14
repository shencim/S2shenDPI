//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const (
	WINDIVERT_LAYER_NETWORK = 0

	WINDIVERT_FLAG_DEFAULT  = 0
	WINDIVERT_FLAG_SNIFF    = 1
	WINDIVERT_FLAG_DROP     = 2

	WINDIVERT_ADDR_SIZE = 80
	MAX_PACKET_SIZE     = 65535

	WINDIVERT_DIRECTION_OUTBOUND = 0
	WINDIVERT_DIRECTION_INBOUND  = 1
)

type WinDivert struct {
	open      *syscall.Proc
	recv      *syscall.Proc
	send      *syscall.Proc
	close     *syscall.Proc
	parse     *syscall.Proc
	checksum  *syscall.Proc
}

var wd *WinDivert

func loadWinDivert() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	exeDir := filepath.Dir(exePath)
	candidates := []string{
		filepath.Join(exeDir, "WinDivert.dll"),
		filepath.Join(exeDir, "binaries", "WinDivert.dll"),
		filepath.Join(exeDir, "..", "WinDivert.dll"),
		"WinDivert.dll",
	}

	var dll *syscall.DLL
	for _, p := range candidates {
		dll, err = syscall.LoadDLL(p)
		if err == nil {
			break
		}
	}
	if dll == nil {
		return fmt.Errorf("WinDivert.dll bulunamadı: %w", err)
	}

	mustProc := func(name string) *syscall.Proc {
		p, e := dll.FindProc(name)
		if e != nil {
			panic(fmt.Sprintf("%s bulunamadı: %v", name, e))
		}
		return p
	}

	wd = &WinDivert{
		open:     mustProc("WinDivertOpen"),
		recv:     mustProc("WinDivertRecv"),
		send:     mustProc("WinDivertSend"),
		close:    mustProc("WinDivertClose"),
		parse:    mustProc("WinDivertHelperParsePacket"),
		checksum: mustProc("WinDivertHelperCalcChecksums"),
	}
	return nil
}

func divertOpen(filter string, layer, priority, flags int) (syscall.Handle, error) {
	filterPtr, _ := syscall.BytePtrFromString(filter)
	r, _, e := wd.open.Call(
		uintptr(unsafe.Pointer(filterPtr)),
		uintptr(layer),
		uintptr(priority),
		uintptr(flags),
	)
	h := syscall.Handle(r)
	if h == syscall.InvalidHandle {
		return syscall.InvalidHandle, fmt.Errorf("WinDivertOpen: %w", e)
	}
	return h, nil
}

func divertRecv(h syscall.Handle, pkt []byte, addr []byte) (uint32, error) {
	var recvLen uint32
	r, _, e := wd.recv.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&pkt[0])),
		uintptr(len(pkt)),
		uintptr(unsafe.Pointer(&recvLen)),
		uintptr(unsafe.Pointer(&addr[0])),
	)
	if r == 0 {
		return 0, e
	}
	return recvLen, nil
}

func divertSend(h syscall.Handle, pkt []byte, addr []byte) error {
	var sent uint32
	r, _, e := wd.send.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&pkt[0])),
		uintptr(len(pkt)),
		uintptr(unsafe.Pointer(&sent)),
		uintptr(unsafe.Pointer(&addr[0])),
	)
	if r == 0 {
		return e
	}
	return nil
}

func divertClose(h syscall.Handle) {
	if h != syscall.InvalidHandle {
		wd.close.Call(uintptr(h))
	}
}

func divertCalcChecksums(pkt []byte, addr []byte, flags uint64) {
	wd.checksum.Call(
		uintptr(unsafe.Pointer(&pkt[0])),
		uintptr(len(pkt)),
		uintptr(unsafe.Pointer(&addr[0])),
		uintptr(flags),
	)
}

func addrIsOutbound(addr []byte) bool {
	return addr[10]&0x02 != 0
}

func addrIsIPv6(addr []byte) bool {
	return addr[10]&0x10 != 0
}

func pktIPVersion(pkt []byte) uint8 {
	if len(pkt) < 1 {
		return 0
	}
	return pkt[0] >> 4
}

func pktProto(pkt []byte) uint8 {
	ver := pktIPVersion(pkt)
	switch ver {
	case 4:
		if len(pkt) < 10 {
			return 0
		}
		return pkt[9]
	case 6:
		if len(pkt) < 7 {
			return 0
		}
		return pkt[6]
	}
	return 0
}

func ipv4HeaderLen(pkt []byte) int {
	if len(pkt) < 1 {
		return 0
	}
	return int(pkt[0]&0x0F) * 4
}

func pktTTL(pkt []byte) uint8 {
	if pktIPVersion(pkt) != 4 || len(pkt) < 9 {
		return 0
	}
	return pkt[8]
}

func setPktTTL(pkt []byte, ttl uint8) {
	if pktIPVersion(pkt) == 4 && len(pkt) >= 9 {
		pkt[8] = ttl
	}
}

func tcpDstPort(pkt []byte) uint16 {
	ihl := ipv4HeaderLen(pkt)
	if len(pkt) < ihl+4 {
		return 0
	}
	return uint16(pkt[ihl+2])<<8 | uint16(pkt[ihl+3])
}

func udpDstPort(pkt []byte) uint16 {
	ihl := ipv4HeaderLen(pkt)
	if len(pkt) < ihl+4 {
		return 0
	}
	return uint16(pkt[ihl+2])<<8 | uint16(pkt[ihl+3])
}

func tcpFlags(pkt []byte) uint8 {
	ihl := ipv4HeaderLen(pkt)
	if len(pkt) < ihl+14 {
		return 0
	}
	return pkt[ihl+13]
}

func tcpDataOffset(pkt []byte) int {
	ihl := ipv4HeaderLen(pkt)
	if len(pkt) < ihl+13 {
		return 0
	}
	return int(pkt[ihl+12]>>4) * 4
}

func tcpPayload(pkt []byte) []byte {
	ihl := ipv4HeaderLen(pkt)
	thl := tcpDataOffset(pkt)
	start := ihl + thl
	if start >= len(pkt) {
		return nil
	}
	return pkt[start:]
}

func udpPayload(pkt []byte) []byte {
	ihl := ipv4HeaderLen(pkt)
	start := ihl + 8
	if start >= len(pkt) {
		return nil
	}
	return pkt[start:]
}
