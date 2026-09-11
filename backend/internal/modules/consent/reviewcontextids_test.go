// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The context endpoint's own obligation: refuse a subject the caller did not
// send, rather than letting the zero UUID reach the refusal lookup and come
// back as 404 — which would tell the caller this review does not name a person
// they never named, and send them looking for a review that is right there.
//
// Probed HERE rather than in the handler, which is where the guard lives:
// gates/requiredbodyids_test.go registers this body as probed, and what it
// registers has to be the door every transport comes through.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRequiredBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Parallel()

	// The store is nil on purpose: the guard runs before anything reads, so a
	// call that reached the database would itself be the failure.
	_, err := (&Store{}).RecordContext(t.Context(), ids.NewV7(), RecordContextInput{
		Kind:       KindRequestedBySubject,
		Note:       "Rang about the March order.",
		OccurredAt: time.Now().Add(-time.Hour),
	})
	if err == nil {
		t.Fatal("an omitted subject_id was accepted; the zero UUID would reach the refusal lookup " +
			"and answer 404 about a person the caller never named")
	}
	var validation *httperr.DetailedError
	if !errors.As(err, &validation) {
		// A permission error here would mean the human check ran first and this
		// test proves nothing about the id guard — which is a real ordering
		// question, not a test artefact.
		t.Fatalf("the refusal is %v, want a validation error naming the field", err)
	}
	if !namesField(validation, fieldSubjectID) {
		t.Errorf("the refusal does not name %q, so a screen cannot say which box is empty",
			fieldSubjectID)
	}
}

// namesField asks whether a validation error highlights one field.
func namesField(err *httperr.DetailedError, field string) bool {
	for _, f := range err.Fields {
		if f.Field == field {
			return true
		}
	}
	return false
}
