// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package searchtext

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// found is the predicate lexical search asks of an activity row.
const found = `SELECT count(*) FROM activity WHERE id = $1 AND search_tsv @@ websearch_to_tsquery('simple', $2)`

// outOfStep counts the rows whose stored search value is not the one their
// subject, body, stored end and language produce.
const outOfStep = `SELECT count(*) FROM activity
	WHERE search_tsv IS DISTINCT FROM activity_search_tsv(subject, body, body_own_end, language)`

const (
	ownWords    = "The Friday delivery works for us."
	quotedReply = "\n\nOn Mon, 3 Mar 2025 at 10:15, Sales <sales@seller.example> wrote:\n> Can you take the walnut cabinets?\n"
)

func logMail(t *testing.T, e *integration.Env, subject, body string, links ...activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Source: "manual", Subject: &subject, Body: &body, Links: links,
	})
	if err != nil {
		t.Fatalf("logging a mail: %v", err)
	}
	return ids.UUID(row.Id)
}

func assertFound(t *testing.T, e *integration.Env, id ids.UUID, word string, want bool) {
	t.Helper()
	if got := e.WsCount(t, found, id, word) == 1; got != want {
		t.Errorf("search for %q finds the mail: %t, want %t", word, got, want)
	}
}

func TestMailSearchReadsTheSendersWordsAndNotTheMailTheyQuote(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	mail := logMail(t, e, "Re: Cabinets", ownWords+quotedReply)

	assertFound(t, e, mail, "delivery", true)
	assertFound(t, e, mail, "cabinets", true) // the subject
	assertFound(t, e, mail, "walnut", false)
	if got := e.WsCount(t, `SELECT body_own_end FROM activity WHERE id = $1`, mail); got != len(ownWords)+2 {
		t.Errorf("stored end %d, want %d: the text above the attribution line", got, len(ownWords)+2)
	}

	// An edit moves the end with the text it was read from.
	edited := "We would rather have the oak ones.\n\n> Can you take the walnut cabinets?\n"
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](mail), activities.UpdateActivityInput{Body: &edited}); err != nil {
		t.Fatalf("editing the mail: %v", err)
	}
	assertFound(t, e, mail, "oak", true)
	assertFound(t, e, mail, "delivery", false)
	assertFound(t, e, mail, "walnut", false)
}

// An Outlook forward and reply carry the same header block. Only the subject
// tells search whether the mail under it is passed on or quoted.
func TestAForwardIsFoundByTheMailItPassesOn(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	body := "Zur Info.\n\nVon: Max Muster <max@kunde.de>\nGesendet: Dienstag, 12. August 2026 09:14\nAn: Vertrieb\nBetreff: Schrank\n\nDer Nussbaum passt.\n"
	forward := logMail(t, e, "WG: Schrank", body)
	reply := logMail(t, e, "AW: Schrank", body)
	assertFound(t, e, forward, "nussbaum", true)
	assertFound(t, e, reply, "nussbaum", false)

	// A new subject moves the end over the body the row keeps.
	subject := "WG: Schrank"
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](reply), activities.UpdateActivityInput{Subject: &subject}); err != nil {
		t.Fatalf("renaming the reply a forward: %v", err)
	}
	assertFound(t, e, reply, "nussbaum", true)
}

// The database cuts by characters, as Go counted them. The words above this
// quote hold letters of two and three bytes, and the stored words lose accents.
func TestTheEndCutsAMailWithLettersOfSeveralBytesAtTheQuote(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	mail := logMail(t, e, "Trả lời", "Bên em đồng ý với điều khoản. Grüße aus Köln.\n\n> Thứ Sáu giao tủ óc chó được không?\n")
	assertFound(t, e, mail, "koln", true)
	assertFound(t, e, mail, "khoan", true)
	assertFound(t, e, mail, "giao", false)
	assertFound(t, e, mail, "tu", false)
}

// A language with no dictionary of its own reads the body once, and one with a
// dictionary reads it a second time as written. Both find the word.
func TestTheBodyIsReadOnceWhenTheLanguageHasNoDictionary(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	mail := logMail(t, e, "Cabinets", "Walnut cabinets arriving.")
	const weights = `SELECT (search_tsv::text LIKE '%C%')::int FROM activity WHERE id = $1`
	e.WsExec(t, `UPDATE activity SET language = NULL WHERE id = $1`, mail)
	if e.WsCount(t, weights, mail) != 0 {
		t.Error("a mail with no language holds a second reading of its body")
	}
	assertFound(t, e, mail, "arriving", true)
	e.WsExec(t, `UPDATE activity SET language = 'en' WHERE id = $1`, mail)
	if e.WsCount(t, weights, mail) != 1 {
		t.Error("an English mail holds no reading of its body as written")
	}
	assertFound(t, e, mail, "arriving", true)
}

func TestANoteIsSearchedWhole(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	subject, body := "Call notes", "They said:\n> walnut is too dark\n"
	note, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Source: "manual", Subject: &subject, Body: &body})
	if err != nil {
		t.Fatalf("logging a note: %v", err)
	}
	assertFound(t, e, ids.UUID(note.Id), "walnut", true)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND body_own_end IS NULL`, ids.UUID(note.Id)); got != 1 {
		t.Error("a note stores an end; a member wrote every line of it")
	}
}

func TestTheSearchValueFollowsEveryWriteOfItsSources(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	mail := logMail(t, e, "Re: Cabinets", ownWords+quotedReply)
	for _, write := range []string{
		`UPDATE activity SET subject = 'Re: Shelves' WHERE id = $1`,
		`UPDATE activity SET language = 'en' WHERE id = $1`,
		`UPDATE activity SET body_own_end = 3 WHERE id = $1`,
		`UPDATE activity SET body = 'Lieferung am Freitag passt.', body_own_end = NULL, language = 'de' WHERE id = $1`,
		`UPDATE activity SET search_tsv = to_tsvector('simple', 'planted words') WHERE id = $1`,
		`UPDATE activity SET search_tsv = NULL WHERE id = $1`,
	} {
		e.WsExec(t, write, mail)
		if got := e.WsCount(t, outOfStep); got != 0 {
			t.Errorf("after %s\n the search value of %d row(s) no longer follows the text", write, got)
		}
	}
	assertFound(t, e, mail, "planted", false)
	assertFound(t, e, mail, "lieferung", true)
}

// An update that names none of the text columns must store neither the body nor
// the search value again. Both are long enough here to be stored out of line,
// so a second copy shows as rows written to the table's out-of-line store.
func TestAWriteThatLeavesTheTextAloneLeavesTheSearchValueAlone(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	var words strings.Builder
	for range 3000 {
		words.WriteString(ids.NewV7().String())
		words.WriteByte(' ')
	}
	mail := logMail(t, e, "Long mail", words.String())

	storedAgain := func(write string) int {
		var before, after int
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			const counted = `SELECT pg_stat_get_xact_tuples_inserted(reltoastrelid) FROM pg_class WHERE oid = 'activity'::regclass`
			if err := tx.QueryRow(e.Admin(), counted).Scan(&before); err != nil {
				return err
			}
			if _, err := tx.Exec(e.Admin(), write, mail); err != nil {
				return err
			}
			return tx.QueryRow(e.Admin(), counted).Scan(&after)
		}); err != nil {
			t.Fatalf("%s: %v", write, err)
		}
		return after - before
	}
	for _, unrelated := range []string{
		`UPDATE activity SET sender_was_stranger = true WHERE id = $1`,
		`UPDATE activity SET owed_verdict = 'informs_us', owed_verdict_at = now() WHERE id = $1`,
		`UPDATE activity SET subject = subject, language = language WHERE id = $1`,
	} {
		if got := storedAgain(unrelated); got != 0 {
			t.Errorf("%s\n stored %d out-of-line rows again, want none: the text did not change", unrelated, got)
		}
	}
	// The measure can tell: a new subject rebuilds the search value.
	if got := storedAgain(`UPDATE activity SET subject = 'Another subject' WHERE id = $1`); got == 0 {
		t.Error("an update that rebuilds the search value stored nothing out of line, so this test cannot see a rewrite")
	}
}

func TestAnErasedMailKeepsNoEndAndNoSearchableWord(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Selma Subject", nil)
	mail := logMail(t, e, "Re: Cabinets", ownWords+quotedReply,
		activities.ActivityLinkInput{EntityType: "contact", EntityID: contact})
	assertFound(t, e, mail, "delivery", true)

	if err := privacy.NewEraser(e.DB()).WithEdgeInvalidator(search.RecomputeEdgesForActivities).EraseContact(e.Admin(), contact, "test"); err != nil {
		t.Fatalf("erasing the contact: %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NULL AND body_own_end IS NULL`, mail); got != 1 {
		t.Fatal("the erased mail keeps its body or the end read from it")
	}
	for _, word := range []string{"delivery", "friday", "walnut", "cabinets"} {
		assertFound(t, e, mail, word, false)
	}
	if got := e.WsCount(t, outOfStep); got != 0 {
		t.Errorf("the search value of %d erased row(s) no longer follows the text", got)
	}
}
