// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// attentionNames against real Postgres and the real row scope: a label is
// exactly as visible as the record behind it. A person this reader may see
// answers their name; a capture-private person owned by somebody else
// answers absent — never an error, and never the name — because the label
// pass must degrade a card, not disclose a record or empty a lane.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func namesOver(e *integration.Env) attentionNames {
	db := InstallationDB(e.Pool)
	return attentionNames{
		people:     people.NewStore(db),
		deals:      deals.NewStore(db, DealsInstallation()),
		activities: activities.NewStore(db),
		projects:   projects.NewStore(db),
	}
}

func TestALabelIsExactlyAsVisibleAsItsRecord(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)

	visible := e.SeedPerson(t, "Dana Weiss", nil)
	private := e.SeedPerson(t, "Zeta Privatkontakt", &e.Rep3)
	e.MakeCapturePrivate(t, "person", private, e.Rep3)

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)

	// BOTH in one ask, which is the batched seam's whole shape and also the
	// sharper test: the visible one must come back named and the private one
	// must not, from the same query. A batch that answered per-set rather
	// than per-row would either name both or name neither.
	labels, err := names.Labels(reader, "person", []ids.UUID{visible, private})
	if err != nil {
		t.Fatalf("naming a page of people: %v", err)
	}
	if labels[visible] != "Dana Weiss" {
		t.Fatalf("label = %q, want the person's name — a resolver that cannot name a readable record makes every card anonymous", labels[visible])
	}
	if label, named := labels[private]; named {
		t.Fatalf("label = %q for another rep's capture-private contact — the label pass just disclosed a record the row scope hides", label)
	}
}

// A record with no name of its own is ABSENT rather than blank: a card
// showing an empty string where a record should be says less than one
// showing nothing, and the single-record form answered ok=false for it.
func TestARecordWithNoNameIsAbsentRatherThanBlank(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	nameless := e.SeedPerson(t, "", nil)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)

	labels, err := names.Labels(reader, "person", []ids.UUID{nameless})
	if err != nil {
		t.Fatalf("naming a nameless person: %v", err)
	}
	if label, named := labels[nameless]; named {
		t.Fatalf("label = %q for a person captured without a name, want absent", label)
	}
}

// An id nobody has ever minted is absent, not an error: the producer's claim
// that a reference exists is not this resolver's to fail the page over.
func TestAnIdThatNamesNothingIsAbsentNotAnError(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)

	labels, err := names.Labels(reader, "person", []ids.UUID{ids.NewV7()})
	if err != nil {
		t.Fatalf("naming a record that does not exist: %v", err)
	}
	if len(labels) != 0 {
		t.Fatalf("labels = %v for an id naming nothing, want none", labels)
	}
}

func TestAnUnknownSubjectTypeAnswersAbsentNotAGuess(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	labels, err := names.Labels(reader, "warehouse", []ids.UUID{ids.NewV7()})
	if err != nil || len(labels) != 0 {
		t.Fatalf("labels=%v err=%v for a type outside the vocabulary, want none and no error", labels, err)
	}
}

// The context matters: the same record, asked for without a principal,
// must refuse into absence rather than leak through the resolver.
func TestTheResolverHoldsNoAuthorityOfItsOwn(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	person := e.SeedPerson(t, "Dana Weiss", nil)
	labels, err := names.Labels(context.Background(), "person", []ids.UUID{person})
	if err == nil && len(labels) > 0 {
		t.Fatal("an unauthenticated ask was answered — the resolver carries authority its callers never granted")
	}
}

// A subject line is CONTENT, and the batch must withhold it exactly as the
// single-record read does.
//
// The card names the message a rep is being told about; on a limited thread
// the reader may know the row exists and may not read what it says. A batched
// read that filtered on the audience instead of selecting THROUGH it would
// have answered the row's subject to everyone who could discover it.
func TestALimitedMessagesSubjectIsWithheldFromTheBatchToo(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	owner := integration.OwnerConn(t)

	open := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Renewal terms', now(), 'manual', 'human:x')`)
	limited := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Board compensation', now(), 'manual', 'human:someone-else')`)
	if _, err := owner.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, limited); err != nil {
		t.Fatalf("limiting the message: %v", err)
	}

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	labels, err := names.Labels(reader, "activity", []ids.UUID{open, limited})
	if err != nil {
		t.Fatalf("naming a page of messages: %v", err)
	}
	if labels[open] != "Renewal terms" {
		t.Errorf("label = %q for an open message, want its subject", labels[open])
	}
	if label, named := labels[limited]; named {
		t.Fatalf("label = %q for a message this reader is not on — the batch just disclosed what a limited thread says", label)
	}
}

// The other shapes, each over the store that owns it.
//
// The organization arm carries the discriminating proof — capture-private is
// a posture this reader demonstrably cannot see through, and neutering the
// scope clause fails it. The deal arm asserts something weaker on purpose:
// that the batch and the single get AGREE. A deal owned by another team's rep
// is still visible to this reader in this fixture — deal visibility is wider
// than ownership — so an exclusion assertion there would pass whatever the
// clause did, which is a test that proves nothing while looking like it does.
// Agreement is the invariant that actually matters anyway: whatever GetDeal
// decides, the batch must decide the same.
func TestEveryShapesLabelsAgreeWithTheirOwnSingleRead(t *testing.T) {
	e := integration.Setup(t)
	names := namesOver(e)
	owner := integration.OwnerConn(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	t.Run("organization", func(t *testing.T) {
		mine := e.SeedOrg(t, "Weber GmbH", nil)
		theirs := e.SeedOrg(t, "Zeta Holding", &e.Rep3)
		e.MakeCapturePrivate(t, "organization", theirs, e.Rep3)

		labels, err := names.Labels(reader, "organization", []ids.UUID{mine, theirs})
		if err != nil {
			t.Fatalf("naming companies: %v", err)
		}
		if labels[mine] != "Weber GmbH" {
			t.Errorf("label = %q for a readable company, want its name", labels[mine])
		}
		if label, named := labels[theirs]; named {
			t.Fatalf("label = %q for another rep's capture-private company", label)
		}
	})

	t.Run("deal", func(t *testing.T) {
		pipeline := integration.SeedIDRow(t, owner,
			`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Names', false, 0)`)
		stage := integration.SeedIDRow(t, owner,
			`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
			 VALUES ($1, '`+pipeline.String()+`', 'Open', 0, 'open', 30)`)
		mine := e.SeedDeal(t, "Fleet retrofit",
			ids.From[ids.PipelineKind](pipeline), ids.From[ids.StageKind](stage), &e.Rep1)
		theirs := e.SeedDeal(t, "Someone else's renewal",
			ids.From[ids.PipelineKind](pipeline), ids.From[ids.StageKind](stage), &e.Rep3)
		// Owned directly, because CreateDeal runs as the admin here and the
		// owner it records is not necessarily the one asked for — and a deal
		// this reader can see anyway would make the assertion below pass
		// whatever the scope clause did.
		e.WsExec(t, `UPDATE deal SET owner_id = $2 WHERE id = $1`, theirs, e.Rep3)

		labels, err := names.Labels(reader, "deal", []ids.UUID{mine, theirs})
		if err != nil {
			t.Fatalf("naming deals: %v", err)
		}
		if labels[mine] != "Fleet retrofit" {
			t.Errorf("label = %q for the reader's own deal, want its name", labels[mine])
		}
		// Against the SINGLE get rather than against an assumption about what
		// a rep may see: the batch's whole promise is that it decides what the
		// one-at-a-time read decides.
		_, err = deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).
			GetDeal(reader, ids.From[ids.DealKind](theirs), storekit.LiveOnly)
		_, named := labels[theirs]
		if named != (err == nil) {
			t.Fatalf("the batch %s a deal the single get %s — the two reads disagree about one record",
				map[bool]string{true: "named", false: "withheld"}[named],
				map[bool]string{true: "answered", false: "refused"}[err == nil])
		}
	})

	t.Run("lead", func(t *testing.T) {
		named := integration.SeedIDRow(t, owner,
			`INSERT INTO lead (id, full_name, source, captured_by) VALUES ($1, 'Ana Ionescu', 'import', 'human:x')`)
		// A lead captured without a name: its face carries an empty label, so
		// the batch answers absent rather than putting a blank on a card.
		nameless := integration.SeedIDRow(t, owner,
			`INSERT INTO lead (id, source, captured_by) VALUES ($1, 'import', 'human:x')`)

		// A reader holding lead.read: AccountRepPerms does not, and a caller
		// the grant refuses answers no labels — which is the refusal arm, not
		// the read this case is about.
		leadReader := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"lead": {Read: true}},
			RowScope: principal.RowScopeTeam,
		})

		labels, err := names.Labels(leadReader, "lead", []ids.UUID{named, nameless})
		if err != nil {
			t.Fatalf("naming leads: %v", err)
		}
		if labels[named] != "Ana Ionescu" {
			t.Errorf("label = %q for a named lead, want its name", labels[named])
		}
		if label, ok := labels[nameless]; ok {
			t.Errorf("label = %q for a lead captured with no name, want absent", label)
		}
	})

	t.Run("project", func(t *testing.T) {
		org := e.SeedOrg(t, "Weber Projekte", nil)
		project := integration.SeedIDRow(t, owner,
			`INSERT INTO project (id, name, organization_id, source, captured_by)
			 VALUES ($1, 'Depot rollout', '`+org.String()+`', 'manual', 'human:x')`)

		projectReader := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"project": {Read: true}},
			RowScope: principal.RowScopeTeam,
		})

		labels, err := names.Labels(projectReader, "project", []ids.UUID{project})
		if err != nil {
			t.Fatalf("naming projects: %v", err)
		}
		if labels[project] != "Depot rollout" {
			t.Errorf("label = %q for a readable project, want its name", labels[project])
		}
	})

	t.Run("archived records are absent", func(t *testing.T) {
		gone := e.SeedPerson(t, "Archived Contact", nil)
		e.WsExec(t, `UPDATE person SET archived_at = now() WHERE id = $1`, gone)

		labels, err := names.Labels(reader, "person", []ids.UUID{gone})
		if err != nil {
			t.Fatalf("naming an archived person: %v", err)
		}
		if label, named := labels[gone]; named {
			t.Fatalf("label = %q for an archived record — the batch names what every live read refuses", label)
		}
	})
}
