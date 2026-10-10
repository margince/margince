// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// A call that breaks a bound and a vocabulary is told the words of both.
func TestTwoArgumentRefusalsKeepBothGuidances(t *testing.T) {
	first := &BadArgsError{Cause: errors.New("a"), Guidance: "limit takes 1 to 5"}
	second := &BadArgsError{Cause: errors.New("b"), Guidance: "`mode` takes one of: x, y"}

	var joined *BadArgsError
	if !errors.As(joinArgRefusals(first, second), &joined) {
		t.Fatal("two argument refusals did not join into one")
	}

	if !strings.Contains(joined.Guidance, first.Guidance) || !strings.Contains(joined.Guidance, second.Guidance) {
		t.Errorf("joined guidance %q lost one of its parts", joined.Guidance)
	}
}

// A null among the enum's members is not the empty word.
func TestANullEnumMemberIsNotAVocabulary(t *testing.T) {
	if words, ok := textWords([]json.RawMessage{json.RawMessage(`"open"`), json.RawMessage(`null`)}); ok {
		t.Errorf("an enum holding null was read as the text vocabulary %q", words)
	}
}
