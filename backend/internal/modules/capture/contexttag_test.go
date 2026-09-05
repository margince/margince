// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The refusal has to reach the wire as a 422. A caller naming a word the
// vocabulary does not carry made a bad request; answering 500 would say the
// product broke, and answering 200 would say it filed something.
func TestAnUnknownWordReadsAsAnInvalidArgument(t *testing.T) {
	err := &UnknownContextTagError{TagID: ids.NewV7()}
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Error("an unknown word does not read as an invalid argument, so the wire answers the wrong status")
	}
	// And it names the word, because "not a tag this workspace carries" over an
	// unnamed id is a message nobody can act on.
	if !strings.Contains(err.Error(), err.TagID.String()) {
		t.Errorf("the refusal does not name the word it refused: %q", err.Error())
	}
}

// The three LEFT JOIN columns are null together — the join found no row — which
// is a connection that chose no word, not one whose word could not be read.
func TestAConnectionWithNoWordReportsNoneRatherThanAnEmptyOne(t *testing.T) {
	if got := contextTagOf(nil, nil, nil); got != nil {
		t.Errorf("a connection that chose no word reports %+v", got)
	}
	id, name, archived := ids.NewV7(), "Nord inbox", true
	got := contextTagOf(&id, &name, &archived)
	if got == nil {
		t.Fatal("a chosen word came back as none")
	}
	if got.ID != id || got.Name != name || !got.Archived {
		t.Errorf("the word reads as %+v, want the row that was joined", got)
	}
}

// "Chose no word" and "chose the nil word" are different facts, and only the
// first one happens. An audit image that rendered the absence as a zero uuid
// would put a word in the record that nobody ever chose.
func TestTheAuditImageTellsNoWordFromAWord(t *testing.T) {
	cleared := contextTagImage("gmail", nil)
	if cleared[auditKeyContextTag] != nil {
		t.Errorf("clearing the word recorded %v rather than an absence", cleared[auditKeyContextTag])
	}
	if cleared[auditKeyProvider] != "gmail" {
		t.Errorf("the image names provider %v", cleared[auditKeyProvider])
	}
	id := ids.NewV7()
	chosen := contextTagImage("gmail", &id)
	if chosen[auditKeyContextTag] != id.String() {
		t.Errorf("the image records %v rather than the word chosen", chosen[auditKeyContextTag])
	}
}
