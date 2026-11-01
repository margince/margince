// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

// The current engine is the ground truth. This makes the calls the screens make
// (frontend/src/lib/worklistday.ts: limit 100, relationships omitted, the
// view's own filter; the relationships panel's own read) for every member, in
// every view and scope, the second page of Today, and a named owner, at one
// pinned instant. It records the answers (POC_PARITY_OUT) or compares them with
// a recording (POC_PARITY_IN); POC_PLANS=generic reuses plans for the whole read.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

type parityCall struct {
	key                         string
	view, scope, filter, cursor string
	owner                       ids.UUID
	limit                       int
	relationships               bool
}

func TestWorklistParityPoC(t *testing.T) {
	ctx := context.Background()
	open := func(env string) *database.DB {
		t.Helper()
		pool, err := database.NewPool(ctx, os.Getenv(env))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return InstallationDB(pool)
	}
	owner, app := open("POC_OWNER_DSN").Pool(), open("POC_APP_DSN").Pool()
	asOf, err := time.Parse(time.RFC3339, os.Getenv("POC_AS_OF"))
	if err != nil {
		t.Fatal(err)
	}
	generic := os.Getenv("POC_PLANS") == "generic"
	full := newAttentionService(app, NewApprovalsService(InstallationDB(app)), func() time.Time { return asOf })
	screen := full.OmittingRelationships()

	var ws ids.UUID
	var users []ids.UUID
	if err := owner.QueryRow(ctx, `SELECT id FROM workspace LIMIT 1`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if err := owner.QueryRow(ctx, `SELECT array_agg(id ORDER BY id) FROM app_user WHERE removed_at IS NULL`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	authority := identity.NewService(app)
	base := principal.WithWorkspaceID(ctx, ws)

	read := func(member context.Context, service *attention.Service, c parityCall) (crmcontracts.Worklist, time.Duration, error) {
		reading := principal.WithCorrelationID(member, ids.UUID(uuid.Must(uuid.NewV7())))
		var out crmcontracts.Worklist
		start := time.Now()
		err := database.WithWorkspaceSnapshot(reading, app, func(ctx context.Context) error {
			page := func(ctx context.Context) error {
				var err error
				out, err = service.WorklistView(ctx, c.view, c.scope, c.filter, c.owner, c.limit, c.cursor)
				return err
			}
			if generic {
				return database.ReuseGenericPlans(ctx, page)
			}
			return page(ctx)
		})
		return out, time.Since(start), err
	}

	answers := map[string]json.RawMessage{}
	timings := map[string][]time.Duration{}
	populated, empty, refused := 0, 0, 0
	for i, user := range users {
		p, err := authz.MemberPrincipal(base, authority, ws, user)
		if err != nil {
			continue
		}
		member := principal.WithActor(base, p)
		seat := "rep"
		if _, _, err := read(member, screen, parityCall{view: "today", scope: "team", filter: "all", limit: 1}); err == nil {
			seat = "manager"
		}
		var calls []parityCall
		for _, scope := range []string{"mine", "team", "unassigned", "all"} {
			calls = append(calls,
				parityCall{view: "today", scope: scope, filter: "all", limit: 100},
				parityCall{view: "decisions", scope: scope, filter: "decisions", limit: 100},
				parityCall{view: "system", scope: scope, filter: "system", limit: 100},
				parityCall{view: "", scope: scope, filter: "all", limit: 1, relationships: true})
		}
		calls = append(calls, parityCall{view: "today", scope: "mine", filter: "all", limit: 100,
			owner: users[(i+1)%len(users)]})
		for _, c := range calls {
			c.key = fmt.Sprintf("%s/%s/%s/%s/rel=%v/owner=%v", user, c.scope, c.view, c.filter, c.relationships, !c.owner.IsZero())
			service := screen
			if c.relationships {
				service = full
			}
			out, took, err := read(member, service, c)
			if err != nil {
				answers[c.key] = json.RawMessage(fmt.Sprintf("%q", "refused: "+strings.SplitN(err.Error(), ":", 2)[0]))
				refused++
				continue
			}
			label := seat + " " + c.scope + " " + c.view
			timings[label] = append(timings[label], took)
			pages := []crmcontracts.Worklist{out}
			// The second page resumes this read's own walk.
			if c.view == "today" && c.owner.IsZero() && out.NextCursor != nil && *out.NextCursor != "" {
				c.cursor = *out.NextCursor
				next, _, err := read(member, service, c)
				if err != nil {
					t.Fatalf("%s page 2: %v", c.key, err)
				}
				pages = append(pages, next)
			}
			for n, page := range pages {
				if len(page.Queue) == 0 && (page.OptionalRelationships == nil || len(*page.OptionalRelationships) == 0) {
					empty++
				} else {
					populated++
				}
				// The walk and cursor name a snapshot row minted per read; the
				// second page is compared by what it shows instead.
				page.Walk, page.NextCursor = nil, nil
				answer, err := json.Marshal(page)
				if err != nil {
					t.Fatal(err)
				}
				answers[fmt.Sprintf("%s/page=%d", c.key, n+1)] = answer
			}
		}
	}
	t.Logf("%d answers: %d with rows, %d empty, %d refused (plans %s)", len(answers), populated, empty, refused,
		map[bool]string{true: "generic", false: "default"}[generic])
	labels := make([]string, 0, len(timings))
	for label := range timings {
		labels = append(labels, label)
	}
	slices.Sort(labels)
	for _, label := range labels {
		took := timings[label]
		slices.Sort(took)
		var sum time.Duration
		for _, d := range took {
			sum += d
		}
		t.Logf("%-32s n=%2d mean %6v p50 %6v max %6v", label, len(took), (sum / time.Duration(len(took))).Round(time.Millisecond),
			took[len(took)/2].Round(time.Millisecond), took[len(took)-1].Round(time.Millisecond))
	}

	if path := os.Getenv("POC_PARITY_OUT"); path != "" {
		recorded, err := json.Marshal(answers)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, recorded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if path := os.Getenv("POC_PARITY_IN"); path != "" {
		recorded, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var truth map[string]json.RawMessage
		if err := json.Unmarshal(recorded, &truth); err != nil {
			t.Fatal(err)
		}
		same, sameRows := 0, 0
		for key, want := range truth {
			if string(want) != string(answers[key]) {
				t.Errorf("%s differs from the ground truth", key)
				continue
			}
			same++
			var w crmcontracts.Worklist
			if json.Unmarshal(want, &w) == nil && (len(w.Queue) > 0 || w.OptionalRelationships != nil && len(*w.OptionalRelationships) > 0) {
				sameRows++
			}
		}
		if len(answers) != len(truth) {
			t.Errorf("%d answers against %d recorded", len(answers), len(truth))
		}
		t.Logf("%d of %d answers identical to the ground truth, %d of them with rows", same, len(truth), sameRows)
	}
}
