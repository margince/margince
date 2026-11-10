// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// writeHoldLimit is the longest an erasure can wait on a delivery. The row is
// held only while the request is written; a write not done by then is dropped.
const writeHoldLimit = 5 * time.Second

// checkedTransport sends one delivery over one new HTTP/1.1 connection. After
// TCP and TLS it takes the send check and writes the whole request in one call.
// The hold ends before any response is read; a refusal writes nothing.
//
// An erasure that commits first stops the send. A later one waits until the
// request is in the kernel's send buffer, for at most writeLimit.
// net/http's Transport flushes after its last trace hook, so it cannot do this.
type checkedTransport struct {
	dial       DialFunc
	tlsConfig  *tls.Config
	writeLimit time.Duration
}

func (c checkedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	wire, err := requestBytes(req)
	if err != nil {
		return nil, err
	}
	conn, err := c.connect(req.Context(), req.URL)
	if err != nil {
		return nil, err
	}
	check, ok := req.Context().Value(sendCheckKey{}).(*sendCheck)
	if !ok {
		return nil, errors.Join(errNoSendCheck, conn.Close())
	}
	stop := context.AfterFunc(req.Context(), func() {
		//craft:ignore swallowed-errors closing on cancellation only unblocks the read or write, which reports the failure
		_ = conn.Close()
	})
	if err := check.writeHeld(func() error { return writeWithin(conn, wire, c.writeLimit) }); err != nil {
		stop()
		return nil, errors.Join(err, conn.Close())
	}
	resp, err := readFinalResponse(bufio.NewReader(conn), req)
	if err != nil {
		stop()
		return nil, errors.Join(err, conn.Close())
	}
	resp.Body = &connBody{ReadCloser: resp.Body, conn: conn, stop: stop}
	return resp, nil
}

// maxInformational is how many 1xx answers may precede the final one, the cap
// net/http's own Transport applies.
const maxInformational = 5

// readFinalResponse skips the informational answers (100, 103) a receiver may
// send before its final one, which is the answer the delivery is judged by.
func readFinalResponse(br *bufio.Reader, req *http.Request) (*http.Response, error) {
	for range maxInformational + 1 {
		resp, err := http.ReadResponse(br, req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 200 || resp.StatusCode == http.StatusSwitchingProtocols {
			return resp, nil
		}
		if err := resp.Body.Close(); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("webhooks: the receiver sent more than %d informational answers", maxInformational)
}

// requestBytes is the whole request as it goes on the wire, with the
// connection closed after it. Writing closes the request body.
func requestBytes(req *http.Request) ([]byte, error) {
	out := req.Clone(req.Context())
	out.Close = true
	var wire bytes.Buffer
	if err := out.Write(&wire); err != nil {
		return nil, fmt.Errorf("webhooks: serialising the delivery request: %w", err)
	}
	return wire.Bytes(), nil
}

// connect dials the target and, for https, does the TLS handshake. ALPN offers
// only HTTP/1.1.
func (c checkedTransport) connect(ctx context.Context, target *url.URL) (net.Conn, error) {
	port := target.Port()
	if port == "" {
		port = "443"
		if target.Scheme == "http" {
			port = "80"
		}
	}
	conn, err := c.dial(ctx, "tcp", net.JoinHostPort(target.Hostname(), port))
	if err != nil || target.Scheme != "https" {
		return conn, err
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.tlsConfig != nil {
		cfg = c.tlsConfig.Clone()
	}
	if cfg.ServerName == "" {
		cfg.ServerName = target.Hostname()
	}
	cfg.NextProtos = []string{"http/1.1"}
	tlsConn := tls.Client(conn, cfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return tlsConn, nil
}

// writeWithin hands the whole request to the kernel, or fails once limit
// passes, for example when the receiver stops reading.
func writeWithin(conn net.Conn, wire []byte, limit time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(limit)); err != nil {
		return err
	}
	if _, err := conn.Write(wire); err != nil {
		return fmt.Errorf("writing the request within %s: %w", limit, err)
	}
	return conn.SetWriteDeadline(time.Time{})
}

// connBody closes the connection with the response body.
type connBody struct {
	io.ReadCloser
	conn net.Conn
	stop func() bool
}

func (b *connBody) Close() error {
	b.stop()
	return errors.Join(b.ReadCloser.Close(), b.conn.Close())
}
