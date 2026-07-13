package main

import "encoding/binary"

// parseSNI, bir TLS ClientHello TCP payload'ından (record header dahil) SNI
// (Server Name Indication) hostname'ini çıkarır. Her adım kendi sınırını
// kontrol eder — hatalı/kısa/segmentli bir payload asla panic'e yol açmaz,
// yalnızca ok=false döner. ClientHello TCP segmentasyonu nedeniyle tek pakette
// tam gelmemişse (büyük ClientHello'larda nadiren olur) de ok=false döner;
// çağıran taraf bu durumda pakete dokunmamalıdır (fail-open — belirsizlikte
// paketi olduğu gibi bırakmak, yanlışlıkla ilgisiz trafiği etkilemekten iyidir).
func parseSNI(payload []byte) (hostname string, ok bool) {
	// TLS record header: type(1) + version(2) + length(2) = 5 byte
	if len(payload) < 5 || payload[0] != TLS_HANDSHAKE {
		return "", false
	}
	recLen := int(binary.BigEndian.Uint16(payload[3:5]))
	hs := payload[5:]
	if recLen > len(hs) {
		recLen = len(hs)
	}
	hs = hs[:recLen]

	// Handshake header: type(1) + length(3) = 4 byte
	if len(hs) < 4 || hs[0] != TLS_CLIENT_HELLO {
		return "", false
	}
	p := hs[4:]

	// ClientVersion(2) + Random(32)
	if len(p) < 34 {
		return "", false
	}
	p = p[34:]

	// SessionID
	if len(p) < 1 {
		return "", false
	}
	sidLen := int(p[0])
	p = p[1:]
	if len(p) < sidLen {
		return "", false
	}
	p = p[sidLen:]

	// CipherSuites
	if len(p) < 2 {
		return "", false
	}
	csLen := int(binary.BigEndian.Uint16(p[:2]))
	p = p[2:]
	if len(p) < csLen {
		return "", false
	}
	p = p[csLen:]

	// CompressionMethods
	if len(p) < 1 {
		return "", false
	}
	cmLen := int(p[0])
	p = p[1:]
	if len(p) < cmLen {
		return "", false
	}
	p = p[cmLen:]

	// Extensions
	if len(p) < 2 {
		return "", false
	}
	extTotalLen := int(binary.BigEndian.Uint16(p[:2]))
	p = p[2:]
	if extTotalLen > len(p) {
		extTotalLen = len(p)
	}
	p = p[:extTotalLen]

	for len(p) >= 4 {
		extType := binary.BigEndian.Uint16(p[0:2])
		extLen := int(binary.BigEndian.Uint16(p[2:4]))
		p = p[4:]
		if extLen > len(p) {
			return "", false
		}
		extData := p[:extLen]
		p = p[extLen:]

		if extType == 0x0000 { // server_name
			return parseServerNameExt(extData)
		}
	}
	return "", false
}

func parseServerNameExt(data []byte) (string, bool) {
	if len(data) < 2 {
		return "", false
	}
	listLen := int(binary.BigEndian.Uint16(data[:2]))
	data = data[2:]
	if listLen > len(data) {
		listLen = len(data)
	}
	data = data[:listLen]

	for len(data) >= 3 {
		nameType := data[0]
		nameLen := int(binary.BigEndian.Uint16(data[1:3]))
		data = data[3:]
		if nameLen > len(data) {
			return "", false
		}
		name := data[:nameLen]
		data = data[nameLen:]
		if nameType == 0x00 { // host_name
			return string(name), true
		}
	}
	return "", false
}
