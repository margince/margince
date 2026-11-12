// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CountRoundTrips counts the pool's outbound Query and Sync messages, including
// preparation exchanges. It retains framing bytes only, never SQL or values.
func CountRoundTrips(t testing.TB, cfg *pgxpool.Config) *atomic.Int64 {
	t.Helper()
	ip := net.ParseIP(cfg.ConnConfig.Host)
	if cfg.ConnConfig.Host != "localhost" && !strings.HasPrefix(cfg.ConnConfig.Host, "/") && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("testdb: protocol counting requires a local test database")
	}
	if cfg.ConnConfig.TLSConfig != nil {
		t.Fatal("testdb: protocol counting requires an explicitly unencrypted test pool")
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if fallback.TLSConfig != nil {
			t.Fatal("testdb: protocol counting cannot inspect a TLS fallback")
		}
	}
	count := &atomic.Int64{}
	dial := cfg.ConnConfig.DialFunc
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if address, ok := conn.RemoteAddr().(*net.TCPAddr); ok && !address.IP.IsLoopback() {
			return nil, errors.Join(errors.New("testdb: protocol counter dialled a non-local address"), conn.Close())
		}
		return &roundTripConn{Conn: conn, wire: roundTripWriter{writer: conn, count: count, startup: true}}, nil
	}
	return count
}

type roundTripConn struct {
	net.Conn
	wire roundTripWriter
}

func (c *roundTripConn) Write(p []byte) (int, error) {
	return c.wire.Write(p)
}

type roundTripWriter struct {
	writer    io.Writer
	count     *atomic.Int64
	header    [5]byte
	headerLen int
	remaining int64
	message   byte
	startup   bool
}

func (w *roundTripWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if scanErr := w.scan(p[:n]); scanErr != nil {
		return n, errors.Join(err, scanErr)
	}
	return n, err
}

func (w *roundTripWriter) scan(p []byte) error {
	for len(p) > 0 {
		if w.remaining > 0 {
			n := min(int64(len(p)), w.remaining)
			p = p[n:]
			w.remaining -= n
			if w.remaining == 0 {
				if err := w.complete(); err != nil {
					return err
				}
			}
			continue
		}
		size := len(w.header)
		if w.startup {
			size--
		}
		n := copy(w.header[w.headerLen:size], p)
		w.headerLen += n
		p = p[n:]
		if w.headerLen < size {
			continue
		}
		length := binary.BigEndian.Uint32(w.header[size-4 : size])
		if length < 4 {
			return fmt.Errorf("testdb: invalid frontend message length %d", length)
		}
		w.message = w.header[0]
		if w.startup {
			w.message = 0
			w.startup = false
		}
		w.headerLen = 0
		w.remaining = int64(length) - 4
		if w.remaining == 0 {
			if err := w.complete(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *roundTripWriter) complete() error {
	if w.message == 'H' {
		return errors.New("testdb: Flush-and-wait is not supported by the round-trip counter")
	}
	if w.message == 'Q' || w.message == 'S' {
		w.count.Add(1)
	}
	return nil
}
