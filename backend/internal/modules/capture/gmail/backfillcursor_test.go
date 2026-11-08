// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type savedHistoryPage struct {
	cursor string
	tally  connector.BackfillPageResult
	failAt int
}

func (*savedHistoryPage) Observed(context.Context, int, int, int) {}
func (p *savedHistoryPage) Checkpoint(_ context.Context, cursor string, tally connector.BackfillPageResult) error {
	if p.failAt > 0 && tally.Scanned >= p.failAt {
		return connector.ErrUnreachable
	}
	p.cursor, p.tally = cursor, tally
	return nil
}

func TestHistoryResumeRetainsHeaderHolesWithoutRelisting(t *testing.T) {
	api := pageOf()
	for i := range 20 {
		id := fmt.Sprintf("m%d@mail.gmail.com", i)
		api.pages[""] = append(api.pages[""], id)
		sender := "colleague@myco.com"
		if i == 0 {
			sender = "customer@acme.com"
		}
		api.raws[id] = rawMsg(id, sender)
	}
	saved := &savedHistoryPage{failAt: 20}
	ctx := connector.WithBackfillProgress(context.Background(), saved)
	c := New(fakeOAuth{access: "access"}, api)
	sink := &judgingSink{dropDomain: "myco.com"}
	if _, err := c.BackfillPage(ctx, authBytes(t), time.Time{}, "", sink); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("checkpoint error: %v", err)
	}
	cursor, prepared, err := parseHistoryCursor(saved.cursor)
	if err != nil || !prepared {
		t.Fatalf("saved cursor: %v", err)
	}
	if saved.tally.Scanned != 10 || len(cursor.IDs) != 10 || !slices.Contains(cursor.IDs, "m0@mail.gmail.com") {
		t.Fatalf("header hole lost: %+v, %+v", saved.tally, cursor)
	}
	api.pages[""] = []string{"unrelated-new-mail"}
	saved.failAt = 0
	res, err := c.BackfillPage(ctx, authBytes(t), time.Time{}, saved.cursor, sink)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 10 || res.Captured != 1 || res.Skipped != 9 || res.NextToken != "" {
		t.Fatalf("resumed result: %+v", res)
	}
	if api.fullReads["m0@mail.gmail.com"] != 2 {
		t.Fatalf("uncheckpointed message should replay once: %+v", api.fullReads)
	}
	final, _, err := parseHistoryCursor(saved.cursor)
	if err != nil || len(final.IDs) != 0 {
		t.Fatalf("final cursor: %+v %v", final, err)
	}
	res, err = c.BackfillPage(ctx, authBytes(t), time.Time{}, saved.cursor, sink)
	if err != nil || res.Scanned != 0 || res.NextToken != "" {
		t.Fatalf("empty prepared page: %+v %v", res, err)
	}
}

func TestHistoryCursorRejectsMalformedAndDuplicateMessages(t *testing.T) {
	next := ""
	duplicate, err := (historyCursor{IDs: []string{"same", "same"}, Next: &next}).encode()
	if err != nil {
		t.Fatal(err)
	}
	missingNext := historyCursorPrefix + base64.RawURLEncoding.EncodeToString([]byte(`{"ids":["mail"]}`))
	for _, token := range []string{historyCursorPrefix + "!", duplicate, missingNext} {
		if _, prepared, err := parseHistoryCursor(token); !prepared || err == nil {
			t.Fatalf("accepted %q", token)
		}
	}
	if _, prepared, err := parseHistoryCursor("legacy-provider-token"); prepared || err != nil {
		t.Fatalf("legacy token: %v %v", prepared, err)
	}
}

type recoveringCaptureSink struct {
	recordingSink
	recover bool
	failed  map[string]bool
}

func (s *recoveringCaptureSink) Upsert(ctx context.Context, rec connector.NormalizedRecord) (datasource.EntityRef, error) {
	if !s.recover && s.failed[rec.NaturalKey.SourceID] {
		return datasource.EntityRef{}, errSinkRefused
	}
	return s.recordingSink.Upsert(ctx, rec)
}

func TestHistoryFailureStreakRemainsRetryableAcrossCheckpointBoundary(t *testing.T) {
	api := &pagedAPI{pages: map[string][]string{"": {}}}
	api.raws = make(map[string][]byte)
	sink := &recoveringCaptureSink{failed: make(map[string]bool)}
	for i := range 20 {
		id := fmt.Sprintf("m%d@mail.gmail.com", i)
		api.pages[""] = append(api.pages[""], id)
		api.raws[id] = rawMsg(id, "sender@acme.com")
		if i >= 6 && i <= 10 {
			sink.failed[id] = true
		}
	}
	saved := &savedHistoryPage{}
	ctx := connector.WithBackfillProgress(t.Context(), saved)
	c := New(fakeOAuth{access: "access"}, api)
	if _, err := c.BackfillPage(ctx, authBytes(t), time.Time{}, "", sink); !errors.Is(err, errSinkRefused) {
		t.Fatalf("failure streak: %v", err)
	}
	cursor, _, err := parseHistoryCursor(saved.cursor)
	if err != nil {
		t.Fatal(err)
	}
	if saved.tally.Scanned != 6 || saved.tally.Failed != 0 || len(cursor.IDs) != 14 {
		t.Fatalf("transient failures were permanently checkpointed: %+v %+v", saved.tally, cursor)
	}
	sink.recover = true
	result, err := c.BackfillPage(ctx, authBytes(t), time.Time{}, saved.cursor, sink)
	if err != nil || result.Captured != 14 || result.Failed != 0 || len(sink.recs) != 20 {
		t.Fatalf("recovered mail missing: result=%+v records=%d err=%v", result, len(sink.recs), err)
	}
}

func TestHistoryRejectsInvalidProviderListingsBeforeSaving(t *testing.T) {
	for _, ids := range [][]string{{""}, {"duplicate", "duplicate"}, make([]string, backfillPageSize+1)} {
		api := &pagedAPI{pages: map[string][]string{"": ids}}
		saved := &savedHistoryPage{}
		ctx := connector.WithBackfillProgress(t.Context(), saved)
		if _, err := New(fakeOAuth{access: "access"}, api).BackfillPage(ctx, authBytes(t), time.Time{}, "", &recordingSink{}); err == nil || saved.cursor != "" {
			t.Fatalf("invalid provider listing was saved: err=%v cursor=%q", err, saved.cursor)
		}
	}
}

func TestHistoryCannotSaveACursorItCannotRestore(t *testing.T) {
	api := &pagedAPI{pages: map[string][]string{"": {}}, next: map[string]string{"": strings.Repeat("x", maxHistoryCursorBytes)}}
	saved := &savedHistoryPage{}
	ctx := connector.WithBackfillProgress(t.Context(), saved)
	if _, err := New(fakeOAuth{access: "access"}, api).BackfillPage(ctx, authBytes(t), time.Time{}, "", &recordingSink{}); err == nil || saved.cursor != "" {
		t.Fatalf("oversized saved cursor: err=%v length=%d", err, len(saved.cursor))
	}
}
