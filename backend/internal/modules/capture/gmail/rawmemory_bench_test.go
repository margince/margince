// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type discardingMailSink struct{}

func (discardingMailSink) Upsert(context.Context, connector.NormalizedRecord) (datasource.EntityRef, error) {
	return datasource.EntityRef{}, nil
}

func BenchmarkRawImportsShareMemoryBudget(b *testing.B) {
	const messageBytes = 50 << 20
	raw := "From: Alice <alice@acme.example>\r\nTo: owner@example.com\r\nSubject: Large mail\r\nMessage-ID: <large@acme.example>\r\nContent-Type: text/plain\r\n\r\n" + strings.Repeat("x", messageBytes)
	body := fmt.Sprintf(`{"raw":%q}`, base64.RawURLEncoding.EncodeToString([]byte(raw)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, body); err != nil {
			b.Errorf("writing large message: %v", err)
		}
	}))
	defer server.Close()
	api := newTestAPI(b, server.Client(), server.URL)
	runtime.GC()
	var initial runtime.MemStats
	runtime.ReadMemStats(&initial)
	var peak atomic.Uint64
	peak.Store(initial.HeapAlloc)
	b.ReportAllocs()
	b.SetBytes(2 * backfillLookahead * messageBytes)
	b.ResetTimer()
	for range b.N {
		var walks sync.WaitGroup
		for range 2 {
			walks.Go(func() {
				err := inOrder(b.Context(), backfillLookahead, func(ctx context.Context, _ int) (Message, error) {
					return api.GetRaw(ctx, "test", "large")
				}, func(_ int, msg Message, err error) error {
					if err != nil {
						return err
					}
					if len(msg.RFC822) < messageBytes {
						return fmt.Errorf("large message was truncated: %d bytes", len(msg.RFC822))
					}
					_, err = captureOne(b.Context(), msg, discardingMailSink{}, nil, "owner@example.com")
					var memory runtime.MemStats
					runtime.ReadMemStats(&memory)
					for old := peak.Load(); memory.HeapAlloc > old && !peak.CompareAndSwap(old, memory.HeapAlloc); old = peak.Load() {
					}
					return err
				}, processRawMemory, rawFetchReservation)
				if err != nil {
					b.Error(err)
				}
			})
		}
		walks.Wait()
	}
	b.StopTimer()
	b.ReportMetric(float64(peak.Load()-initial.HeapAlloc)/(1<<20), "sampled-heap-growth-MiB")
}
