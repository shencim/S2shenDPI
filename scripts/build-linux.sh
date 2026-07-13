#!/bin/bash
set -e

echo "[BUILD] S2shenDPI Linux build başlıyor..."

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
DIVERT_DIR="$ROOT_DIR/s2shen-divert"
BINARIES_DIR="$ROOT_DIR/src-tauri/binaries"
SPOOFDPI_VERSION="1.2.1"
SPOOFDPI_DIR="$ROOT_DIR/SpoofDPI-${SPOOFDPI_VERSION}"

mkdir -p "$BINARIES_DIR"

# ─── 1) s2shen-proxy (SpoofDPI) ────────────────────────────────────────────
if [ ! -f "$BINARIES_DIR/s2shen-proxy-x86_64-unknown-linux-gnu" ]; then
    echo "[BUILD] s2shen-proxy (SpoofDPI ${SPOOFDPI_VERSION}) derleniyor..."

    if [ ! -d "$SPOOFDPI_DIR" ]; then
        echo "[BUILD] SpoofDPI kaynak kodu indiriliyor..."
        curl -sL "https://github.com/xvzc/SpoofDPI/archive/refs/tags/v${SPOOFDPI_VERSION}.tar.gz" \
            -o /tmp/spoofdpi.tar.gz
        tar -xzf /tmp/spoofdpi.tar.gz -C "$ROOT_DIR"
        rm /tmp/spoofdpi.tar.gz
        # GitHub arşivi SpoofDPI-1.2.1 olarak çıkartır
        if [ -d "$ROOT_DIR/SpoofDPI-${SPOOFDPI_VERSION}" ]; then
            echo "[BUILD] SpoofDPI kaynak kodu hazır"
        else
            echo "[ERROR] SpoofDPI kaynak dizini bulunamadı: $SPOOFDPI_DIR"
            exit 1
        fi
    fi

    cd "$SPOOFDPI_DIR"
    GOOS=linux GOARCH=amd64 go build \
        -trimpath \
        -ldflags="-s -w" \
        -o "$BINARIES_DIR/s2shen-proxy-x86_64-unknown-linux-gnu" \
        ./cmd/spoofdpi

    echo "[BUILD] s2shen-proxy ✅"
else
    echo "[BUILD] s2shen-proxy zaten mevcut, atlanıyor..."
fi

# ─── 2) s2shen-divert (NFQUEUE engine) ─────────────────────────────────────
echo "[BUILD] s2shen-divert (Linux/NFQUEUE) derleniyor..."
cd "$DIVERT_DIR"

if [ ! -f go.sum ]; then
    echo "[BUILD] go mod tidy çalıştırılıyor (bağımlılıklar indiriliyor)..."
    go mod tidy
fi

GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o "$BINARIES_DIR/s2shen-divert-x86_64-unknown-linux-gnu" \
    .

echo "[BUILD] s2shen-divert ✅"

# ─── 3) Tauri uygulaması ─────────────────────────────────────────────────────
echo "[BUILD] Tauri uygulaması derleniyor..."
cd "$ROOT_DIR"

if [ ! -d node_modules ]; then
    npm install
fi

npm run tauri build

echo ""
echo "[BUILD] ✅ Tüm build tamamlandı!"
echo "[BUILD] Çıktı: src-tauri/target/release/bundle/"
echo ""
echo "[SONRAKI ADIM] Capabilities kurulumu için:"
echo "  sudo bash scripts/s2shendpi-setup.sh"
