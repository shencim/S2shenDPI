//go:build linux

package main

import "encoding/binary"

// Paket parse yardımcıları — windivert.go'nun Linux karşılığı

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
