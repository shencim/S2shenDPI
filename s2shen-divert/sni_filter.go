package main

import "strings"

// discordSNIAllowlist, "Discord Split" modunda dokunulacak tek trafik grubunu
// tanımlar. Discord'un DPI ile bloklanan kısmı neredeyse her zaman gateway/API/CDN
// TLS handshake'idir (WebSocket gateway + REST API + statik CDN); sesli görüşme
// (voice/RTP) trafiği SNI taşımaz ve tipik olarak DPI ile hedef alınmaz, bu yüzden
// listede yer almasına gerek yoktur — handshake geçtikten sonra ses zaten çalışır.
var discordSNIAllowlist = []string{
	"discord.com",
	"discord.gg",
	"discordapp.com",
	"discordapp.net",
	"discord.media",
	"discordcdn.com",
	"gateway.discord.gg",
}

// matchesSNIAllowlist, hostname'in allowlist'teki bir domaine tam eşit olup
// olmadığını ya da onun bir alt-domaini olup olmadığını kontrol eder
// (ör. "gateway-us-east1-b.discord.gg" -> "discord.gg" eşleşir,
// ama "notdiscord.gg" -> "discord.gg" EŞLEŞMEZ).
func matchesSNIAllowlist(hostname string, allowlist []string) bool {
	h := strings.ToLower(strings.TrimSuffix(hostname, "."))
	for _, domain := range allowlist {
		if h == domain || strings.HasSuffix(h, "."+domain) {
			return true
		}
	}
	return false
}
