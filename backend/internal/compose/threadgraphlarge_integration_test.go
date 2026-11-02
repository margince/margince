// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedHistoricalReverseChain(t *testing.T, size int) (*integration.Env, []string) {
	t.Helper()
	e := integration.Setup(t)
	address := seatAddress(t, e, e.Rep1)
	ctx := threadConnectorCtx(e, e.Rep1)
	// Pre-upgrade history uses the real ingress writer without the new partition postprocessing.
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithThreadJoin(capture.ThreadJoiner{})
	keys := make([]string, size)
	messages := make([]ids.ActivityID, size)
	messageKeys := make(map[ids.ActivityID]string, size)
	for i := range size {
		keys[i] = fmt.Sprintf("history-%04d@counterparty.example", i)
		raw := []byte(strings.Replace(string(threadMail(address, keys[i], "Mon, 01 Jun 2026 08:00:00 +0000", "")), "Subject: Re: Angebot", "Subject: ", 1))
		parsed, err := mailmap.Parse(raw, address)
		if err != nil {
			t.Fatal(err)
		}
		record := parsed.ToRecord("gmail", raw)
		if record.ThreadKey != keys[i] {
			t.Fatalf("independent historical message root=%q, want %q", record.ThreadKey, keys[i])
		}
		ref, err := sink.Upsert(ctx, record)
		if err != nil {
			t.Fatal(err)
		}
		messages[i] = ids.From[ids.ActivityKind](ref.ID)
		messageKeys[messages[i]] = keys[i]
	}
	// Every parent sorts after its child regardless of the writer's identity clock.
	slices.SortFunc(messages, func(a, b ids.ActivityID) int { return strings.Compare(a.String(), b.String()) })
	for i, message := range messages {
		keys[i] = messageKeys[message]
	}
	topic := "Price"
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := e.Activities.UpdateActivity(owner, messages[size-1], activities.UpdateActivityInput{Subject: &topic}); err != nil {
		t.Fatalf("setting the named ancestor through its mailbox owner: %v", err)
	}
	for start := 0; start < size; start += 64 {
		end := min(start+64, size)
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			for i := start; i < end; i++ {
				if i < size-1 {
					if err := activities.RecordMailReferencesTx(e.Admin(), tx, messages[i], []string{keys[i+1]}); err != nil {
						return err
					}
				}
				if i > 0 {
					if err := mergeThreadsTx(e.Admin(), tx, keys[i], keys[0]); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return e, keys
}

func TestALargeHistoricalReverseChainHasLinearTotalRepairWork(t *testing.T) {
	for _, size := range []int{2, 2000} {
		if !t.Run(fmt.Sprint(size), func(t *testing.T) {
			e, keys := seedHistoricalReverseChain(t, size)
			units := drainMeasuredThreadGraph(t, e, keys[0], size, 256)
			want := keys[size-1]
			for _, key := range keys {
				if got := threadKeyOfMessage(t, e, key); got != want {
					t.Fatalf("%s root=%s, want %s", key, got, want)
				}
			}
			t.Logf("historical reverse chain: messages=%d total persisted units=%d", size, units)
		}) {
			return
		}
	}
}
