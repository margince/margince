// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package searchplan

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A grouped page shows, for every type, the hits a ranked search of that type
// alone puts first. It names the type as holding more when the ranked search
// has more. In the corpus, some types fill the page by name. Others are found
// only by a contained address, a domain, or the company a contact works at.
func TestAGroupedPageKeepsEachTypesRankedHits(t *testing.T) {
	e := integration.Setup(t)
	seedGroupedCorpus(t, e)
	store := search.NewStore(e.DB())
	queries := []string{"co", "ang", "contract", "zeb", "quinn@oz", "ozebra.example", "cora alpha", "cobalt or zeb", "o'rei"}
	for _, query := range queries {
		for _, perType := range []int{3, 8} {
			grouped, err := store.Search(e.Admin(), paletteInput(query, &perType, nil))
			if err != nil {
				t.Fatalf("grouped %q: %v", query, err)
			}
			for _, entity := range []string{"contact", "company", "lead"} {
				ranked, err := store.Search(e.Admin(), paletteInput(query, nil, []string{entity}))
				if err != nil {
					t.Fatalf("ranked %q %s: %v", query, entity, err)
				}
				want := hitsOf(ranked.Hits, entity)
				wantMore := len(want) > perType
				want = want[:min(perType, len(want))]
				if got := hitsOf(grouped.Hits, entity); !slices.Equal(got, want) {
					t.Errorf("%q per_type %d: grouped %s hits %v, want the ranked %v", query, perType, entity, got, want)
				}
				if more := slices.Contains(grouped.TypesWithMore, entity); more != wantMore {
					t.Errorf("%q per_type %d: %s holds more = %v, want %v", query, perType, entity, more, wantMore)
				}
			}
		}
	}
	if hits := hitsOf(mustSearch(e.Admin(), t, store, "ang"), "contact"); len(hits) == 0 {
		t.Error(`"ang" found no contact: the corpus no longer reaches the contained-address rows`)
	}
}

// When a type's own matches fill the page, the contained-address scans never
// run; when they do not, they do. The second half keeps the first from passing
// on a corpus the scans never reached.
func TestAFilledGroupedPageSkipsTheContainedAddressScans(t *testing.T) {
	e := integration.Setup(t)
	seedGroupedCorpus(t, e)
	statements := &statementLog{}
	cfg := e.Pool.Config().Copy()
	cfg.ConnConfig.Tracer = statements
	pool, err := testdb.OwnPoolFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := search.NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.WS)))
	for query, wantScan := range map[string]bool{"co": false, "ang": true} {
		perType := 3
		statements.reset()
		if _, err := store.Search(e.Admin(), paletteInput(query, &perType, nil)); err != nil {
			t.Fatal(err)
		}
		ranking, args := statements.ranking()
		if ran := containedScanRan(t, pool, ranking, args); ran != wantScan {
			t.Errorf("%q: a contained-address scan ran = %v, want %v", query, ran, wantScan)
		}
	}
}

func paletteInput(query string, perType *int, types []string) search.Input {
	in := search.Input{Query: query, Types: types, PerType: perType, WithEmployees: true, AddressCompanies: true}
	if perType == nil {
		in.Limit = 200
	}
	return in
}

func mustSearch(ctx context.Context, t *testing.T, store *search.Store, query string) []search.Hit {
	t.Helper()
	perType := 3
	page, err := store.Search(ctx, paletteInput(query, &perType, nil))
	if err != nil {
		t.Fatal(err)
	}
	return page.Hits
}

func hitsOf(hits []search.Hit, entity string) []string {
	var out []string
	for _, hit := range hits {
		if hit.Type == entity {
			out = append(out, fmt.Sprintf("%s:%.6f", hit.ID, hit.Score))
		}
	}
	return out
}

// seedGroupedCorpus writes through the contacts store. Ten contacts, companies
// and leads start with "co". Other records are reached only by an address, a
// domain or an employer.
func seedGroupedCorpus(t *testing.T, e *integration.Env) {
	t.Helper()
	for i := range 10 {
		seedContact(t, e, fmt.Sprintf("Cora %c", 'A'+i), fmt.Sprintf("cora%d@north.example", i))
		seedCompany(t, e, fmt.Sprintf("Cobalt %c", 'A'+i), fmt.Sprintf("cobalt%d.example", i))
		seedLead(t, e, fmt.Sprintf("Corey %c", 'A'+i), fmt.Sprintf("corey%d@south.example", i))
		seedContact(t, e, fmt.Sprintf("O Reid %c", 'A'+i), fmt.Sprintf("reid%d@east.example", i))
	}
	// "o'rei" matches the O Reids by name. Ana's address and her word "orei"
	// rank her first, so a page the O Reids fill must still read her.
	seedContact(t, e, "Ana Orei", "ana@orei.example")
	seedContact(t, e, "Cora Alpha", "cora.alpha@north.example")
	for name, email := range map[string]string{
		"Yara Quill": "yara@mekong-controls.example", "Lin Wang": "lin.wang@north.example",
		"Mei Zhang": "mei.zhang@west.example", "Tom Ruiz": "tom@contracts-hub.example",
		"Zeb Ono": "zeb@ono.example", "Zebulon Ray": "ray@ray.example",
		"Quinn Hale": "quinn@ozebra.example", "Rhea Moss": "rhea@kazeb.example",
		"Nova Alpha": "nova@north.example", "Tim OReilly": "tim@media.example",
		"Ida Rhys": "ida@oreilly-books.example",
	} {
		seedContact(t, e, name, email)
	}
	seedCompany(t, e, "Riverside Ltd", "riverside-cogs.example")
	seedCompany(t, e, "Hill Ltd", "hillang.example")
	seedLead(t, e, "Ana Silva", "ana@kangaroo.example")
	seedLead(t, e, "Ben Ode", "ben@contractor.example")
	partners := seedCompany(t, e, "Contract Partners", "partners.example")
	employee := seedContact(t, e, "Pia Lund", "pia@lund.example")
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &employee, CompanyID: &partners, Source: provenance.RecordSourceManual,
	}); err != nil {
		t.Fatal(err)
	}
}

func seedContact(t *testing.T, e *integration.Env, name, email string) ids.ContactID {
	t.Helper()
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: provenance.RecordSourceManual,
		Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}

func seedCompany(t *testing.T, e *integration.Env, name, domain string) ids.CompanyID {
	t.Helper()
	created, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: name, Domains: []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(created.Id))
}

func seedLead(t *testing.T, e *integration.Env, name, email string) {
	t.Helper()
	if _, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Email: &email, Source: "manual"}); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
}

// statementLog keeps the statements a pool sends, so a test can read the plan
// of the one the search ranked with.
type statementLog struct {
	mu   sync.Mutex
	sent []pgx.TraceQueryStartData
}

func (l *statementLog) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, data)
	return ctx
}

func (*statementLog) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (l *statementLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = nil
}

// ranking is the grouped statement and its arguments, the exec mode dropped.
func (l *statementLog) ranking() (string, []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, data := range l.sent {
		if strings.Contains(data.SQL, ") grouped ORDER BY") {
			return data.SQL, slices.DeleteFunc(slices.Clone(data.Args), func(a any) bool {
				_, mode := a.(pgx.QueryExecMode)
				return mode
			})
		}
	}
	return "", nil
}

// containedScanRan explains the statement and reports whether a node testing a
// LIKE that starts with a wildcard ran at least once.
func containedScanRan(t *testing.T, pool *pgxpool.Pool, statement string, args []any) bool {
	t.Helper()
	if statement == "" {
		t.Fatal("the search sent no grouped statement")
	}
	var plan []struct{ Plan planNode }
	var raw []byte
	if err := pool.QueryRow(context.Background(), "EXPLAIN (ANALYZE, FORMAT JSON) "+statement,
		append([]any{pgx.QueryExecModeDescribeExec}, args...)...).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &plan); err != nil || len(plan) == 0 {
		t.Fatalf("reading the plan: %v", err)
	}
	return plan[0].Plan.containedScanRan()
}

//nolint:tagliatelle // fixed by the server's plan format
type planNode struct {
	Loops       int        `json:"Actual Loops"`
	Filter      string     `json:"Filter"`
	IndexCond   string     `json:"Index Cond"`
	RecheckCond string     `json:"Recheck Cond"`
	Plans       []planNode `json:"Plans"`
}

func (n planNode) containedScanRan() bool {
	for _, cond := range []string{n.Filter, n.IndexCond, n.RecheckCond} {
		if n.Loops > 0 && strings.Contains(cond, "~~ '%") {
			return true
		}
	}
	return slices.ContainsFunc(n.Plans, planNode.containedScanRan)
}
