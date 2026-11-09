// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// mixedPage lists thirty messages; the ones colleague names come from the
// workspace's own domain, the rest from a customer.
func mixedPage(colleague func(i int) bool) (*headerAPI, []string) {
	ids := make([]string, 30)
	for i := range ids {
		ids[i] = fmt.Sprintf("m%d@mail.gmail.com", i)
	}
	api := pageOf(ids...)
	for i, id := range ids {
		from := "alice@acme.com"
		if colleague(i) {
			from = "colleague@myco.com"
		}
		api.raws[id] = rawMsg(id, from)
	}
	return api, ids
}

func TestAMailboxOfKeptMailIsDownloadedWithoutReadingEveryHeader(t *testing.T) {
	// Colleague mail only after the sample: the sample drops nothing.
	api, ids := mixedPage(func(i int) bool { return i == 20 })
	sink := &judgingSink{dropDomain: "myco.com"}

	res, err := New(fakeOAuth{access: "access-1"}, api).BackfillPage(context.Background(), authBytes(t), clocktest.Now(t), "", sink)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(sink.judged) != headerSample {
		t.Errorf("judged %d messages by their headers, want the %d of the sample", len(sink.judged), headerSample)
	}
	for _, id := range ids {
		if api.fullReads[id] != 1 {
			t.Errorf("%s downloaded %d times, want once", id, api.fullReads[id])
		}
	}
	if res.Scanned != len(ids) || res.Captured != len(ids) {
		t.Errorf("page = %+v, want all %d scanned and captured", res, len(ids))
	}
}

func TestAMailboxOfColleagueMailKeepsReadingHeadersFirst(t *testing.T) {
	api, ids := mixedPage(func(i int) bool { return i%2 == 0 })
	sink := &judgingSink{dropDomain: "myco.com"}

	res, err := New(fakeOAuth{access: "access-1"}, api).BackfillPage(context.Background(), authBytes(t), clocktest.Now(t), "", sink)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(sink.judged) != len(ids) {
		t.Errorf("judged %d messages by their headers, want all %d once the sample showed it pays", len(sink.judged), len(ids))
	}
	if api.fullReads[ids[28]] != 0 {
		t.Error("colleague mail after the sample was downloaded; its headers settle it")
	}
	if res.Skipped != len(ids)/2 || res.Captured != len(ids)/2 {
		t.Errorf("page = %+v, want half skipped and half captured", res)
	}
}

func TestTheHeaderPassPaysOnlyWhenItsDropsSaveMoreThanItCosts(t *testing.T) {
	// A header read is a third of a download, so it pays above one drop in three.
	for dropped, want := range map[int]bool{0: false, 3: false, 4: true, 10: true} {
		if got := headerPassPays(headerSample, dropped); got != want {
			t.Errorf("%d of %d dropped: pays = %v, want %v", dropped, headerSample, got, want)
		}
	}
}
