//go:build windows

package main

import (
	"crypto/rand"
	"encoding/binary"
)

const fakeSNI = "www.google.com"

func buildFakeClientHello() []byte {
	random := make([]byte, 32)
	rand.Read(random)
	sessionID := make([]byte, 32)
	rand.Read(sessionID)

	cipherSuites := []byte{
		0x13, 0x01,
		0x13, 0x03,
		0x13, 0x02,
		0xC0, 0x2B,
		0xC0, 0x2F,
		0xCC, 0xA9,
		0xCC, 0xA8,
		0xC0, 0x2C,
		0xC0, 0x30,
		0x00, 0x9C,
		0x00, 0x9D,
		0x00, 0x2F,
		0x00, 0x35,
	}

	exts := fhConcat(
		fhBuildSNIExt([]byte(fakeSNI)),
		[]byte{0x00, 0x17, 0x00, 0x00},
		[]byte{0xFF, 0x01, 0x00, 0x01, 0x00},
		fhBuildExt(0x000A, []byte{0x00, 0x08, 0x00, 0x1D, 0x00, 0x17, 0x00, 0x18, 0x00, 0x19}),
		fhBuildExt(0x000B, []byte{0x01, 0x00}),
		[]byte{0x00, 0x23, 0x00, 0x00},
		fhBuildALPN(),
		[]byte{0x00, 0x05, 0x00, 0x05, 0x01, 0x00, 0x00, 0x00, 0x00},
		fhBuildExt(0x002B, []byte{0x04, 0x03, 0x04, 0x03, 0x03}),
		fhBuildExt(0x000D, []byte{
			0x00, 0x18,
			0x04, 0x03, 0x05, 0x03, 0x06, 0x03,
			0x08, 0x04, 0x08, 0x05, 0x08, 0x06,
			0x04, 0x01, 0x05, 0x01, 0x06, 0x01,
			0x02, 0x03, 0x02, 0x01, 0x02, 0x02,
		}),
		fhBuildExt(0x002D, []byte{0x01, 0x01}),
		fhBuildKeyShare(),
	)

	ch := fhConcat(
		[]byte{0x03, 0x03},
		random,
		[]byte{0x20},
		sessionID,
		fhU16(uint16(len(cipherSuites))),
		cipherSuites,
		[]byte{0x01, 0x00},
		fhU16(uint16(len(exts))),
		exts,
	)

	hs := fhConcat(
		[]byte{0x01},
		fhU24(uint32(len(ch))),
		ch,
	)

	return fhConcat(
		[]byte{0x16, 0x03, 0x01},
		fhU16(uint16(len(hs))),
		hs,
	)
}

func fhBuildSNIExt(sni []byte) []byte {
	entry := fhConcat([]byte{0x00}, fhU16(uint16(len(sni))), sni)
	data := fhConcat(fhU16(uint16(len(entry))), entry)
	return fhBuildExt(0x0000, data)
}

func fhBuildExt(t uint16, data []byte) []byte {
	return fhConcat(fhU16(t), fhU16(uint16(len(data))), data)
}

func fhBuildALPN() []byte {
	list := fhConcat(
		[]byte{0x02, 'h', '2'},
		[]byte{0x08, 'h', 't', 't', 'p', '/', '1', '.', '1'},
	)
	return fhBuildExt(0x0010, fhConcat(fhU16(uint16(len(list))), list))
}

func fhBuildKeyShare() []byte {
	key := make([]byte, 32)
	rand.Read(key)
	entry := fhConcat([]byte{0x00, 0x1D}, fhU16(32), key)
	return fhBuildExt(0x0033, fhConcat(fhU16(uint16(len(entry))), entry))
}

func fhU16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

func fhU24(v uint32) []byte {
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}

func fhConcat(parts ...[]byte) []byte {
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	out := make([]byte, 0, total)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
