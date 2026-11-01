// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// How much of a Worklist read a frozen walk already saves: the first page
// against the page that resumes it, for every member, as Home asks.
func TestWorklistResumeCostPoC(t *testing.T) {
	owner, app := pocPools(t)
	asOf := time.Now().UTC()
	service := newAttentionService(app, NewApprovalsService(InstallationDB(app)), func() time.Time { return asOf })
	var firstTotal, resumeTotal time.Duration
	pages := 0
	for _, m := range pocMembers(t, owner, app) {
		read := func(cursor string) (time.Duration, *string) {
			ctx := principal.WithCorrelationID(m.ctx, ids.UUID(uuid.Must(uuid.NewV7())))
			start := time.Now()
			out, err := service.WorklistView(ctx, "today", "mine", "", ids.Nil, 25, cursor)
			if err != nil {
				t.Fatal(err)
			}
			return time.Since(start), out.NextCursor
		}
		read("") // warm this member's plans
		first, next := read("")
		if next == nil || *next == "" {
			t.Logf("%s first page %v, no second page", m.id.String()[24:], first.Round(time.Millisecond))
			continue
		}
		resume, _ := read(*next)
		firstTotal += first
		resumeTotal += resume
		pages++
		t.Logf("%s first page %v, resumed page %v", m.id.String()[24:], first.Round(time.Millisecond), resume.Round(time.Millisecond))
	}
	if pages > 0 {
		t.Logf("mean over %d members: first %v, resumed %v", pages, (firstTotal / time.Duration(pages)).Round(time.Millisecond), (resumeTotal / time.Duration(pages)).Round(time.Millisecond))
	}
}
