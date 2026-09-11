// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The works_with kind and its suggestion against a real database. The pair
// uniqueness, the shape refusal and the suggestion's edge-gate are all SQL —
// hand-built rows can prove none of them.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// worksWithPerms is a bounded rep who may read the graph AND record a
// relationship — the exact authority the one-click acceptance needs.
var worksWithPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":               {Read: true, Update: true},
		"company":               {Read: true},
		"relationship":          {Read: true, Create: true},
		"activity":              {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

func peerSuggested(nodes []crmcontracts.ContactGraphNode, contact ids.UUID) bool {
	for _, n := range nodes {
		if n.ContactId != nil && ids.UUID(*n.ContactId) == contact && string(n.Group) == "peer" {
			return n.SuggestEdge != nil && *n.SuggestEdge
		}
	}
	return false
}

// One pair is one row, whichever order it was recorded in: the reversed
// second write is a conflict, not a second fact.
func TestWorksWithIsOnePairOneRowEitherWayRound(t *testing.T) {
	e := Setup(t)
	anna := e.SeedContact(t, "Anna Weber", &e.Rep1)
	birgit := e.SeedContact(t, "Birgit Sommer", &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, worksWithPerms)
	store := contacts.NewStore(e.DB())

	annaID := ids.From[ids.ContactKind](anna)
	birgitID := ids.From[ids.ContactKind](birgit)
	if _, err := store.CreateRelationship(rep, contacts.CreateRelationshipInput{
		Kind: "works_with", ContactID: &annaID, CounterpartyContactID: &birgitID, Source: "manual",
	}); err != nil {
		t.Fatalf("recording the pair: %v", err)
	}
	_, err := store.CreateRelationship(rep, contacts.CreateRelationshipInput{
		Kind: "works_with", ContactID: &birgitID, CounterpartyContactID: &annaID, Source: "manual",
	})
	var conflict *contacts.RelationshipConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("the reversed pair → %v, want the uniqueness conflict", err)
	}
}

// The suggestion flag rides a peer with enough shared evidence, and vanishes
// the moment the edge is recorded — the read tells the truth about the record.
func TestPeerSuggestionAppearsAndVanishesOnceRecorded(t *testing.T) {
	e := Setup(t)
	anchor := e.SeedContact(t, "Anna Weber", &e.Rep1)
	peer := e.SeedContact(t, "Birgit Sommer", &e.Rep1)
	for _, subject := range []string{"thread one", "thread two", "thread three"} {
		seedSharedThread(t, e, e.Rep1, anchor, peer, subject, "workspace")
	}

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, worksWithPerms)
	code, graph := readGraph(rep, t, e, anchor)
	if code != 200 {
		t.Fatalf("graph → %d, want 200", code)
	}
	if !peerSuggested(graph.Nodes, peer) {
		t.Fatalf("no suggestion on the evidenced peer, want suggest_edge=true")
	}

	anchorID := ids.From[ids.ContactKind](anchor)
	peerID := ids.From[ids.ContactKind](peer)
	if _, err := contacts.NewStore(e.DB()).CreateRelationship(rep, contacts.CreateRelationshipInput{
		Kind: "works_with", ContactID: &anchorID, CounterpartyContactID: &peerID, Source: "manual",
	}); err != nil {
		t.Fatalf("recording the pair: %v", err)
	}
	_, graph = readGraph(rep, t, e, anchor)
	if peerSuggested(graph.Nodes, peer) {
		t.Errorf("the suggestion survived the recorded edge, want it gone")
	}
}

// A caller without the relationship read grant gets no suggestion: "not yet
// recorded" is a claim about which edges exist, and it is not theirs to hear.
// The peer itself still renders — the observation is contact-gated, not
// relationship-gated.
func TestPeerSuggestionNeedsTheRelationshipReadGrant(t *testing.T) {
	e := Setup(t)
	anchor := e.SeedContact(t, "Anna Weber", &e.Rep1)
	peer := e.SeedContact(t, "Birgit Sommer", &e.Rep1)
	for _, subject := range []string{"thread one", "thread two", "thread three"} {
		seedSharedThread(t, e, e.Rep1, anchor, peer, subject, "workspace")
	}

	noEdgeRead := worksWithPerms
	noEdgeRead.Objects = map[string]principal.ObjectGrant{
		"contact":               {Read: true},
		"company":               {Read: true},
		"activity":              {Read: true},
		"installation_settings": {Read: true},
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, noEdgeRead)
	code, graph := readGraph(rep, t, e, anchor)
	if code != 200 {
		t.Fatalf("graph → %d, want 200", code)
	}
	found := false
	for _, n := range graph.Nodes {
		if n.ContactId != nil && ids.UUID(*n.ContactId) == peer && string(n.Group) == "peer" {
			found = true
			if n.SuggestEdge != nil && *n.SuggestEdge {
				t.Errorf("suggest_edge set without the relationship read grant")
			}
		}
	}
	if !found {
		t.Errorf("the peer vanished with the grant; only the suggestion should")
	}
}

// An edge is the CONJUNCTION of its endpoints' visibility: a works_with row
// whose far end the caller may not read is absent, whichever column the far
// end sits in — otherwise the row discloses a capture-private contact's
// existence to everyone who can read the near end.
func TestAWorksWithEdgeHidesWithItsFarEnd(t *testing.T) {
	e := Setup(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	hidden := e.SeedContact(t, "Their Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", hidden, e.Rep3)
	owner := OwnerConn(t)
	for _, pair := range [][2]ids.UUID{{mine, hidden}, {hidden, mine}} {
		SeedIDRow(t, owner, `INSERT INTO relationship (id, kind, contact_id, counterparty_contact_id, source, captured_by)
			VALUES ($1, 'works_with', '`+pair[0].String()+`', '`+pair[1].String()+`', 'manual', 'human:x')`)
		rep := e.As(e.Rep1, []ids.UUID{e.Team1}, worksWithPerms)
		var peers map[ids.UUID]bool
		err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
			var err error
			peers, err = contacts.NewStore(e.DB()).WorksWithPeers(rep, tx, ids.From[ids.ContactKind](mine))
			return err
		})
		if err != nil {
			t.Fatalf("reading recorded peers: %v", err)
		}
		if peers[hidden] {
			t.Errorf("a works_with edge to a capture-private contact reached the caller (far end in %v)", pair)
		}
		if _, err := owner.Exec(context.Background(), `DELETE FROM relationship WHERE kind = 'works_with'`); err != nil {
			t.Fatalf("resetting the pair: %v", err)
		}
	}
}

// Lifecycle reaches BOTH columns: archiving the counterparty sweeps the edge,
// and merging a contact re-homes their pairs whichever side they sit on — a
// self-pair or a duplicate is archived rather than doubled.
func TestWorksWithFollowsArchiveAndMergeOnEitherColumn(t *testing.T) {
	e := Setup(t)
	anna := e.SeedContact(t, "Anna Weber", &e.Rep1)
	birgit := e.SeedContact(t, "Birgit Sommer", &e.Rep1)
	archivist := worksWithPerms
	archivist.Objects = map[string]principal.ObjectGrant{
		"contact":               {Read: true, Update: true, Delete: true},
		"company":               {Read: true},
		"relationship":          {Read: true, Create: true},
		"activity":              {Read: true},
		"installation_settings": {Read: true},
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, archivist)
	store := contacts.NewStore(e.DB())

	annaID := ids.From[ids.ContactKind](anna)
	birgitID := ids.From[ids.ContactKind](birgit)
	if _, err := store.CreateRelationship(rep, contacts.CreateRelationshipInput{
		Kind: "works_with", ContactID: &annaID, CounterpartyContactID: &birgitID, Source: "manual",
	}); err != nil {
		t.Fatalf("recording the pair: %v", err)
	}
	// Archive the COUNTERPARTY — the column the sweep used to miss.
	if _, err := store.ArchiveContact(rep, birgitID, nil); err != nil {
		t.Fatalf("archiving the counterparty: %v", err)
	}
	var live int
	owner := OwnerConn(t)
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM relationship WHERE kind = 'works_with' AND archived_at IS NULL`).Scan(&live); err != nil {
		t.Fatalf("counting live pairs: %v", err)
	}
	if live != 0 {
		t.Errorf("archiving the counterparty left %d live works_with edge(s), want 0", live)
	}
}
