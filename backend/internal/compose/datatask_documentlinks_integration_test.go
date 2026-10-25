// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Mail captured before link rows existed gets them from its stored original.
// A second run writes nothing, and a link a human archived stays archived.
func TestStoredMailGetsItsDocumentLinksOnceAndKeepsAnArchivedOne(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	connector := principal.WithActor(reader, principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: integration.AdminPerms})
	system := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:document_links", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})

	raw := []byte(strings.Join([]string{
		"From: buyer@customer.test", "To: rep@ourco.test", "Subject: The deck",
		"Message-ID: <doc-links-1@customer.test>", "Date: " + at.Format(time.RFC1123Z),
		"Content-Type: text/html; charset=utf-8", "",
		`<p><a href="https://docs.google.com/presentation/d/abc/edit">Q4 plan</a> and <a href="https://example.net/pricing">pricing</a></p>`,
	}, "\r\n"))
	parsed, err := mailmap.Parse(raw, "rep@ourco.test")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := capture.NewSink(e.DB()).Upsert(connector, parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM attachment WHERE activity_id = $1`, ref.ID); n != 0 {
		t.Fatalf("a capture with no file keeper wrote %d rows, want 0", n)
	}

	run := func() int {
		var changed int
		if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
			out, err := documentLinksDataTask().Steps[0].Batch(system, tx, datatask.BatchIn{Limit: 50})
			changed = out.Changed
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return changed
	}
	if changed := run(); changed != 1 {
		t.Fatalf("first run recorded %d links, want only the deck", changed)
	}
	if v := e.WsScalar(t, `SELECT filename FROM attachment WHERE activity_id = $1 AND category = 'link'`, ref.ID); v != "Q4 plan" {
		t.Errorf("repaired link is named %q, want its anchor text", v)
	}
	if changed := run(); changed != 0 {
		t.Errorf("second run recorded %d links, want 0", changed)
	}

	link := e.WsScalar(t, `SELECT id::text FROM attachment WHERE activity_id = $1 AND category = 'link'`, ref.ID)
	e.WsExec(t, `UPDATE attachment SET archived_at = now() WHERE id = $1`, link)
	if changed := run(); changed != 0 {
		t.Errorf("a run after a human archived the link recorded %d links, want 0", changed)
	}
}
