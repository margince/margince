// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The held-message reuse rule is written twice, and the two copies must agree.
//
// A send the consent engine refuses is frozen into a held scheduled_send so the
// review recording that refusal has something a human can resume. Pressing send
// again reuses that row rather than freezing a second copy — otherwise one
// message sits in front of somebody twice.
//
// "The same message" is spelled out in two places. The migration's unique index
// decides which rows the DATABASE refuses as duplicates; heldTwinOf decides
// which row the WRITER goes looking for. If they disagree, the looser one wins
// in a way nobody sees: a twin read narrower than the index makes every second
// press hit ON CONFLICT and re-read, which is merely slow — but a twin read
// WIDER than the index reuses a row the database does not consider the same
// message. That is a rep's refusal bound to somebody else's held message, or to
// a message on another thread, which resumes and files under the wrong record
// or sends from the wrong mailbox.
//
// Neither copy can be deleted. The index is what makes the race safe, and the
// query is what avoids losing it every time. So they are pinned to each other
// here instead.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// heldTwinColumns is the identity both copies must name. Adding a column to the
// index without adding it here fails this test, which is the point: the list is
// the agreement, and it is edited deliberately.
var heldTwinColumns = []string{
	"agent_actor_id",
	"also_links",
	"agent_passport_id",
	"anchor_activity_id",
	"held_reason",
	"origin_kind",
	"origin_links",
	"payload",
	"payload_version",
	"principal_kind",
	"scheduled_by",
}

func TestTheHeldTwinReadMatchesItsUniquenessIndex(t *testing.T) {
	t.Parallel()
	index := readHeldTwinSource(t,
		"migrations/core/1789081200_one_seat_holds_one_refused_message_once.up.sql",
		"CREATE UNIQUE INDEX scheduled_send_one_held_message_per_seat")
	query := readHeldTwinSource(t,
		"internal/modules/activities/holdforreview.go",
		"func heldTwinOf")

	indexTerms := heldTwinTerms(index)
	queryTerms := heldTwinTerms(query)

	// THE TERM SETS MUST BE EQUAL, not merely overlapping. A column named on
	// one side only is drift in whichever direction it points: an index term
	// the query lacks makes every second press lose the ON CONFLICT race and
	// re-read, which is slow; a query term the index lacks reuses a row the
	// database does not consider the same message — a rep's refusal bound to
	// another seat's held message, which resumes from the wrong mailbox.
	//
	// The terms carry their COALESCE wrapping, so dropping a coalesce from one
	// side is drift too. Without that, a NULL anchor would compare unequal to
	// itself on the query side while the index folded it to a zero uuid, and
	// every account-started refusal would miss its own twin.
	for column, indexWrapping := range indexTerms {
		queryWrapping, named := queryTerms[column]
		if !named {
			t.Errorf("the unique index compares %s and heldTwinOf does not.\n\n"+
				"The index decides which held rows the database refuses as duplicates; the twin "+
				"read decides which row the writer reuses. A read narrower than the index makes "+
				"every second press lose the conflict race and re-read.", column)
			continue
		}
		if queryWrapping != indexWrapping {
			t.Errorf("%s is compared as %q by the unique index and as %q by heldTwinOf.\n\n"+
				"A coalesce on one side only makes a NULL compare unequal to itself there while "+
				"the other folds it to a default, so a refusal misses its own twin and holds a "+
				"second copy of one message.",
				column, heldTwinWrappingName(indexWrapping), heldTwinWrappingName(queryWrapping))
		}
	}
	for column := range queryTerms {
		if _, named := indexTerms[column]; !named {
			t.Errorf("heldTwinOf compares %s and the unique index does not.\n\n"+
				"A read WIDER than the index reuses a row the database does not consider the "+
				"same message — a rep's refusal bound to another seat's held message, which "+
				"resumes from the wrong mailbox.", column)
		}
	}

	// The census is asserted before its result is believed: two empty maps
	// agree perfectly, and a walk that parsed nothing would report exactly
	// that.
	for _, column := range heldTwinColumns {
		if _, named := indexTerms[column]; !named {
			t.Errorf("the unique index no longer compares %s. It is part of what makes two "+
				"refusals one message; dropping it means a held row is reused for a message "+
				"that differs in it.", column)
		}
	}

	// The index must stay scoped to a refusal hold. Widened to every held row
	// it would refuse two SCHEDULED sends of one message held for the same
	// reason — two real promises a rep is entitled to make, and the second
	// would fail to be held at all.
	if !strings.Contains(index, "WHERE status = 'held' AND held_reason = 'send_refused'") {
		t.Error("the uniqueness index is no longer scoped to refusal holds, so a scheduled " +
			"message stopped for another reason would collide with an identical one")
	}
}

// heldTwinTerms reduces one side to a map from column to the wrapping it is
// compared under — "" bare, "coalesce", "md5" or "md5(coalesce". The two sides
// can then be compared as maps rather than by reading each other's syntax.
//
// The wrapping is part of the comparison, not decoration. A coalesce dropped
// from one side makes a NULL anchor compare unequal to itself there while the
// other folds it to a zero uuid, and every account-started refusal would then
// miss its own twin.
//
// The coalesce DEFAULT is deliberately not compared: the two sides spell the
// same zero uuid differently and both mean "absent".
func heldTwinTerms(body string) map[string]string {
	out := map[string]string{}
	for _, column := range heldTwinColumns {
		for _, m := range heldTwinTermFor(column).FindAllStringSubmatch(body, -1) {
			wrapping := strings.ToLower(strings.Join(strings.Fields(m[1]), ""))
			wrapping = strings.TrimSuffix(wrapping, "(")
			// The longest wrapping wins, so one bare mention elsewhere in the
			// statement — a column named in a comment, or in the INSERT's own
			// column list — cannot mask the wrapped comparison that matters.
			if _, seen := out[column]; !seen || len(wrapping) > len(out[column]) {
				out[column] = wrapping
			}
		}
	}
	return out
}

// heldTwinTermFor matches this column where it is compared, capturing whatever
// wraps it. The column name is anchored on both sides so payload does not match
// inside payload_version.
func heldTwinTermFor(column string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)((?:md5\s*\(\s*)?(?:coalesce\s*\(\s*)?)\b` +
		regexp.QuoteMeta(column) + `\b(?:[^_a-z0-9]|$)`)
}

// heldTwinWrappingName renders a wrapping for a message a reader can act on.
func heldTwinWrappingName(wrapping string) string {
	if wrapping == "" {
		return "the bare column"
	}
	return wrapping + "(...)"
}

// readHeldTwinSource returns the file's text from the named declaration on, so
// a column named elsewhere in the file cannot satisfy the check above.
func readHeldTwinSource(t *testing.T, path, from string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	body := string(raw)
	start := strings.Index(body, from)
	if start < 0 {
		t.Fatalf("%s no longer contains %q — the held-message reuse rule moved and this gate "+
			"cannot see it any more", path, from)
	}
	rest := body[start:]
	// One declaration's worth. The SQL statement ends at its semicolon; the Go
	// function ends at the first line starting with a closing brace.
	if end := strings.Index(rest, ";"); end > 0 && strings.HasPrefix(from, "CREATE") {
		return rest[:end]
	}
	if end := strings.Index(rest, "\n}\n"); end > 0 {
		return rest[:end]
	}
	return rest
}
