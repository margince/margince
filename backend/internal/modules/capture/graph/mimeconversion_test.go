// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestGetMIMESkipsAMessageGraphCannotConvert(t *testing.T) {
	_, api := newTestClients(t)
	_, err := api.GetMIME(context.Background(), "access-2", "", "badmime")
	if !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("a message with no MIME form must skip, got %v", err)
	}
}

func TestGetMIMEKeepsAnOrdinary500AnOutage(t *testing.T) {
	_, api := newTestClients(t)
	_, err := api.GetMIME(context.Background(), "access-2", "", "boom")
	if errors.Is(err, connector.ErrSkip) || !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("a 500 without the conversion code must stay unreachable, got %v", err)
	}
}

func serverFault() error {
	return &connector.ProviderError{Op: "/messages", Status: 500, Class: ErrUnreachable}
}

// A message whose fetch keeps failing is named in the page's error, so the
// engine can count the attempts; the page still ends without it.
func TestBackfillPageNamesTheMessageItsFetchFailedOn(t *testing.T) {
	api := &fakeAPI{
		listIDs:   []string{"m1", "m2"},
		raws:      map[string][]byte{"m1": rawMsg("a@mail.example", "good@acme.com")},
		getErrIDs: map[string]error{"m2": serverFault()},
	}
	_, err := pinnedConn(api).BackfillPage(context.Background(), authBytes(t), time.Time{}, "", &recordingSink{})
	var mf *connector.MessageFaultError
	if !errors.As(err, &mf) || mf.ID != "m2" || !errors.Is(err, ErrUnreachable) {
		t.Fatalf("want an unreachable error naming m2, got %v", err)
	}
}

// Once the engine asks to walk past the stalled message, it is counted in
// Failed and the messages after it are still captured.
func TestBackfillPageWalksPastTheStalledMessage(t *testing.T) {
	api := &fakeAPI{
		listIDs: []string{"m1", "m2", "m3"},
		raws: map[string][]byte{
			"m1": rawMsg("a@mail.example", "good@acme.com"),
			"m3": rawMsg("c@mail.example", "good@acme.com"),
		},
		getErrIDs: map[string]error{"m2": serverFault()},
	}
	ctx := connector.WithBackfillSkip(context.Background(), "m2")
	res, err := pinnedConn(api).BackfillPage(ctx, authBytes(t), time.Time{}, "", &recordingSink{})
	if err != nil {
		t.Fatalf("the stalled message must not end the page: %v", err)
	}
	if res.Scanned != 3 || res.Captured != 2 || res.Failed != 1 {
		t.Fatalf("tally = %+v, want scanned=3 captured=2 failed=1", res)
	}
}

// The skip is for the named message alone: another 5xx still ends the page.
func TestBackfillPageStillEndsOnAnotherMessagesFault(t *testing.T) {
	api := &fakeAPI{listIDs: []string{"m1"}, getErrIDs: map[string]error{"m1": serverFault()}}
	ctx := connector.WithBackfillSkip(context.Background(), "other")
	if _, err := pinnedConn(api).BackfillPage(ctx, authBytes(t), time.Time{}, "", &recordingSink{}); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("want the page to end, got %v", err)
	}
}

// Two messages that both fail are each walked past once the engine names both.
func TestBackfillPageWalksPastEveryStalledMessage(t *testing.T) {
	api := &fakeAPI{
		listIDs:   []string{"m1", "m2", "m3"},
		raws:      map[string][]byte{"m1": rawMsg("a@mail.example", "good@acme.com")},
		getErrIDs: map[string]error{"m2": serverFault(), "m3": serverFault()},
	}
	ctx := connector.WithBackfillSkip(context.Background(), "m2", "m3")
	res, err := pinnedConn(api).BackfillPage(ctx, authBytes(t), time.Time{}, "", &recordingSink{})
	if err != nil {
		t.Fatalf("the stalled messages must not end the page: %v", err)
	}
	if res.Scanned != 3 || res.Captured != 1 || res.Failed != 2 {
		t.Fatalf("tally = %+v, want scanned=3 captured=1 failed=2", res)
	}
}
