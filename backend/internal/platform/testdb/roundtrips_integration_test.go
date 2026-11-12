// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgproto3"
)

func TestRoundTripFramingSurvivesEveryWriteBoundary(t *testing.T) {
	messages := []pgproto3.FrontendMessage{
		&pgproto3.StartupMessage{ProtocolVersion: pgproto3.ProtocolVersionNumber},
		&pgproto3.Parse{Query: "SELECT 'S Q'"},
		&pgproto3.Describe{ObjectType: 'S'},
		&pgproto3.Sync{},
		&pgproto3.Bind{},
		&pgproto3.Execute{},
		&pgproto3.Sync{},
		&pgproto3.Query{String: "SELECT 'Q S'"},
	}
	var wire []byte
	for _, message := range messages {
		var err error
		wire, err = message.Encode(wire)
		if err != nil {
			t.Fatal(err)
		}
	}
	for split := range len(wire) + 1 {
		count := &atomic.Int64{}
		writer := roundTripWriter{writer: io.Discard, count: count, startup: true}
		for _, part := range [][]byte{wire[:split], wire[split:]} {
			if _, err := writer.Write(part); err != nil {
				t.Fatal(err)
			}
		}
		if count.Load() != 3 {
			t.Fatalf("split %d counted %d round trips, want 3", split, count.Load())
		}
	}
	count := &atomic.Int64{}
	writer := roundTripWriter{writer: io.Discard, count: count, startup: true}
	for _, b := range wire {
		if _, err := writer.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if count.Load() != 3 {
		t.Fatalf("byte-at-a-time writes counted %d trips, want 3", count.Load())
	}
}

func TestUnsupportedProtocolAndBadFramingCannotPassTheCounter(t *testing.T) {
	for name, wire := range map[string][]byte{
		"flush-and-wait": {'H', 0, 0, 0, 4},
		"bad length":     {'S', 0, 0, 0, 3},
	} {
		t.Run(name, func(t *testing.T) {
			writer := roundTripWriter{writer: io.Discard, count: &atomic.Int64{}}
			if _, err := writer.Write(wire); err == nil {
				t.Fatal("the counter accepted protocol traffic it cannot count")
			}
		})
	}
}

type brokenProtocolWriter struct{ failure error }

func (w brokenProtocolWriter) Write(p []byte) (int, error) { return len(p), w.failure }

func TestFramingFailureKeepsTheTransportError(t *testing.T) {
	failure := errors.New("connection failed")
	writer := roundTripWriter{writer: brokenProtocolWriter{failure}, count: &atomic.Int64{}}
	if _, err := writer.Write([]byte{'S', 0, 0, 0, 3}); !errors.Is(err, failure) {
		t.Fatalf("framing failure lost the transport error: %v", err)
	}
}
