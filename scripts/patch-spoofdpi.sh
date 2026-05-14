#!/bin/bash
# SpoofDPI'yi Windows cross-compile için patch'le
set -e

NETUTIL_DIR="internal/netutil"

# 1) conn_common.go — platform bağımsız paylaşılan kod
cat > "$NETUTIL_DIR/conn_common.go" << 'EOF'
package netutil

import (
	"io"
	"sync"
)

var bufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

func CloseConns(closers ...io.Closer) {
	for _, c := range closers {
		if c != nil {
			_ = c.Close()
		}
	}
}
EOF

# 2) conn.go — sadece !windows için işaretle, paylaşılan fonksiyonları kaldır
sed -i '1s/^/\/\/go:build !windows\n\n/' "$NETUTIL_DIR/conn.go"

python3 - << 'PYEOF'
import re
with open('internal/netutil/conn.go', 'r') as f:
    content = f.read()
content = re.sub(r'var bufferPool = sync\.Pool\{.*?\n\}\n', '', content, flags=re.DOTALL)
content = re.sub(r'func CloseConns\(.*?\n\}\n', '', content, flags=re.DOTALL)
with open('internal/netutil/conn.go', 'w') as f:
    f.write(content)
print("conn.go temizlendi")
PYEOF

# 3) conn_windows.go — Windows stub
cat > "$NETUTIL_DIR/conn_windows.go" << 'EOF'
//go:build windows

package netutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/rs/zerolog"
	"github.com/xvzc/SpoofDPI/internal/logging"
)

func TunnelConns(
	ctx context.Context,
	logger zerolog.Logger,
	errCh chan<- error,
	dst net.Conn,
	src net.Conn,
) {
	var n int64
	logger = logging.WithLocalScope(ctx, logger, "tunnel")

	stop := context.AfterFunc(ctx, func() { CloseConns(src, dst) })
	defer func() {
		stop()
		CloseConns(src, dst)
		logger.Trace().Int64("len", n).
			Str("route", fmt.Sprintf("%s -> %s", src.RemoteAddr(), dst.RemoteAddr())).
			Msgf("done")
	}()

	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	n, err := io.CopyBuffer(dst, src, *bufPtr)
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
		errCh <- err
		return
	}
	errCh <- nil
}

func SetTTL(conn net.Conn, isIPv4 bool, ttl uint8) error {
	return nil
}
EOF

echo "Patch tamamlandı"
