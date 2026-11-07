// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// senderOwesAnAbsentColleaguesWork says whether a reply naming the sender as
// promiser gives them a colleague's work. That holds when the promised
// sentence names a colleague who is not on the message and the writer does
// not speak of themselves in it. "Demo on the 5th with Robin" is then Robin's
// work, and the sender gets no task for it. "I'll run the demo with Robin"
// stays the sender's promise. The names come from the seat roster, never from
// a list in the code.
func senderOwesAnAbsentColleaguesWork(
	ctx context.Context, tx pgx.Tx, ref, sentence string, parties []commitmentParty,
) (bool, error) {
	if !commitmentPartyHasRole(parties, ref, commitmentSenderRole) || writerIsTheSubject(sentence) {
		return false, nil
	}
	present := map[ids.UUID]bool{}
	for _, party := range parties {
		if party.Seat != nil {
			present[*party.Seat] = true
		}
	}
	rows, err := tx.Query(ctx, `SELECT id, display_name, coalesce(greeting_name, '') FROM app_user WHERE NOT is_agent AND `+identity.LiveMemberSQL(""))
	if err != nil {
		return false, fmt.Errorf("reading the colleagues a promise may name: %w", err)
	}
	defer rows.Close()
	words := sentenceWords(sentence)
	for rows.Next() {
		var seat ids.UUID
		var display, greeting string
		if err := rows.Scan(&seat, &display, &greeting); err != nil {
			return false, fmt.Errorf("reading the colleagues a promise may name: %w", err)
		}
		if !present[seat] && namesColleague(words, display, greeting) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// writerSubjectWords are the subject words a writer uses of themselves in the
// languages the product reads mail in. A sentence holding one has the writer
// acting, whoever else it names. An object such as "me" does not count:
// "Robin will send me the pricing" is Robin's promise.
// Apart from claims.writerSubjects: this scans every word of a mail sentence, so it holds single words.
var writerSubjectWords = map[string]bool{
	"i": true, "i'll": true, "i'm": true, "i've": true, "i'd": true, "let's": true,
	"we": true, "we'll": true, "we're": true, "we've": true, "we'd": true,
	"ich": true, "wir": true,
}

func writerIsTheSubject(sentence string) bool {
	for _, word := range rawWords(sentence) {
		if writerSubjectWords[strings.ToLower(strings.ReplaceAll(word, "’", "'"))] {
			return true
		}
	}
	return false
}

// rawWords splits a sentence into words, keeping an apostrophe inside one.
func rawWords(sentence string) []string {
	return strings.FieldsFunc(sentence, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' && r != '’' && r != '-'
	})
}

// sentenceWords is rawWords with a possessive "Robin's" read as the name.
func sentenceWords(sentence string) []string {
	words := rawWords(sentence)
	for i, word := range words {
		for _, suffix := range []string{"'s", "’s"} {
			if trimmed, ok := strings.CutSuffix(word, suffix); ok && trimmed != "" {
				words[i] = trimmed
			}
		}
	}
	return words
}

// namesColleague says whether the words hold a colleague's full name, first
// name or chosen greeting name, written with its capital as a name is.
func namesColleague(words []string, display, greeting string) bool {
	full := strings.Fields(display)
	candidates := [][]string{full, strings.Fields(greeting)}
	if len(full) > 1 {
		candidates = append(candidates, full[:1])
	}
	for _, name := range candidates {
		if len(name) == 0 || len([]rune(name[0])) < 2 || !unicode.IsUpper([]rune(name[0])[0]) {
			continue
		}
		for i := 0; i+len(name) <= len(words); i++ {
			if equalWords(words[i:i+len(name)], name) {
				return true
			}
		}
	}
	return false
}

func equalWords(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
