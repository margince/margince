// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

// The current engine is the ground truth. This reads every member's Worklist
// in every view and scope at one pinned instant and either records the answers
// (POC_PARITY_OUT) or compares them with a recording (POC_PARITY_IN), with
// POC_PLANS=generic switching plan reuse on for the whole read.

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

func TestWorklistParityPoC(t *testing.T) {
	ctx := context.Background()
	openPool := func(env string) *database.DB {
		t.Helper()
		pool, err := database.NewPool(ctx, os.Getenv(env))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return InstallationDB(pool)
	}
	owner, app := openPool("POC_OWNER_DSN").Pool(), openPool("POC_APP_DSN").Pool()
	asOf, err := time.Parse(time.RFC3339, os.Getenv("POC_AS_OF"))
	if err != nil {
		t.Fatal(err)
	}
	generic := os.Getenv("POC_PLANS") == "generic"
	service := newAttentionService(app, NewApprovalsService(InstallationDB(app)), func() time.Time { return asOf })

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

	answers := map[string]json.RawMessage{}
	var took []time.Duration
	for _, user := range users {
		p, err := authz.MemberPrincipal(base, authority, ws, user)
		if err != nil {
			continue
		}
		member := principal.WithActor(base, p)
		for _, scope := range []string{"mine", "team"} {
			for _, view := range []string{"today", "decisions", "system"} {
				key := user.String() + "/" + scope + "/" + view
				var out crmcontracts.Worklist
				read := func() (time.Duration, error) {
					reading := principal.WithCorrelationID(member, ids.UUID(uuid.Must(uuid.NewV7())))
					start := time.Now()
					err := database.WithWorkspaceSnapshot(reading, app, func(ctx context.Context) error {
						page := func(ctx context.Context) error {
							var err error
							out, err = service.WorklistView(ctx, view, scope, "", ids.Nil, 25, "")
							return err
						}
						if generic {
							return database.ReuseGenericPlans(ctx, page)
						}
						return page(ctx)
					})
					return time.Since(start), err
				}
				if _, err := read(); err != nil {
					answers[key] = json.RawMessage(`"refused"`)
					continue
				}
				d, err := read()
				if err != nil {
					t.Fatalf("%s: %v", key, err)
				}
				took = append(took, d)
				// The walk and its cursor name a snapshot row minted per read.
				out.Walk, out.NextCursor = nil, nil
				answer, err := json.Marshal(out)
				if err != nil {
					t.Fatal(err)
				}
				answers[key] = answer
			}
		}
	}
	slices.Sort(took)
	var sum time.Duration
	for _, d := range took {
		sum += d
	}
	t.Logf("%d reads (plans %s): mean %v, p50 %v, p95 %v", len(took), map[bool]string{true: "generic", false: "default"}[generic],
		(sum / time.Duration(len(took))).Round(time.Millisecond), took[len(took)/2].Round(time.Millisecond), took[len(took)*95/100].Round(time.Millisecond))

	if path := os.Getenv("POC_PARITY_OUT"); path != "" {
		recorded, err := json.Marshal(answers)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, recorded, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %d answers", len(answers))
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
		same := 0
		for key, want := range truth {
			if string(want) == string(answers[key]) {
				same++
				continue
			}
			t.Errorf("%s differs from the ground truth", key)
		}
		t.Logf("%d of %d answers identical to the ground truth", same, len(truth))
	}
}
