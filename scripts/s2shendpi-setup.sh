#!/bin/bash
# S2shenDPI Linux kurulum ve izin ayarları
set -e

APP_BIN="$(which s2shendpi 2>/dev/null || echo "/opt/s2shendpi/s2shendpi")"
DIVERT_BIN="/opt/s2shendpi/binaries/s2shen-divert-x86_64-unknown-linux-gnu"
PROXY_BIN="/opt/s2shendpi/binaries/s2shen-proxy-x86_64-unknown-linux-gnu"

echo "[SETUP] S2shenDPI Linux kurulum..."

# s2shen-divert'e gerekli kernel capabilities ver
# (root olmadan NFQUEUE kullanımı için)
if [ -f "$DIVERT_BIN" ]; then
    setcap 'cap_net_admin+eip cap_net_raw+eip' "$DIVERT_BIN"
    echo "[SETUP] s2shen-divert capabilities: cap_net_admin, cap_net_raw ✅"
else
    echo "[WARN] s2shen-divert binary bulunamadı: $DIVERT_BIN"
fi

# s2shen-proxy capabilities (düşük port açmak için gerekli değil, >1024)
if [ -f "$PROXY_BIN" ]; then
    setcap 'cap_net_bind_service+eip' "$PROXY_BIN" 2>/dev/null || true
    echo "[SETUP] s2shen-proxy capabilities ✅"
fi

# nfnetlink_queue modülünü otomatik yükle
if ! lsmod | grep -q nfnetlink_queue; then
    modprobe nfnetlink_queue 2>/dev/null || echo "[WARN] nfnetlink_queue modülü yüklenemedi"
fi

# Modülü boot'ta yükle
echo "nfnetlink_queue" > /etc/modules-load.d/s2shendpi.conf
echo "xt_NFQUEUE" >> /etc/modules-load.d/s2shendpi.conf

echo "[SETUP] ✅ S2shenDPI kurulum tamamlandı"
echo "[SETUP] Uygulamayı root olmadan başlatabilirsiniz."
