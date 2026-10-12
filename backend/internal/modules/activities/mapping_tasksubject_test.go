// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A task is nothing but its subject, whichever door makes it: the generic
// activity create holds the same rule as the task create.
func TestATaskMadeThroughTheActivityDoorNeedsASubject(t *testing.T) {
	blank, padded := "   ", "  Call the buyer  "
	for name, subject := range map[string]*string{"missing": nil, "blank": &blank} {
		t.Run(name, func(t *testing.T) {
			_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
				Kind:    crmcontracts.CreateActivityRequestKindCreateActivityRequestKindTask,
				Subject: subject, Source: "manual",
			})
			var required *RequiredFieldError
			if !errors.As(err, &required) || required.Field != fieldSubject {
				t.Fatalf("answered %v, want a required-field refusal naming subject", err)
			}
		})
	}

	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:    crmcontracts.CreateActivityRequestKindCreateActivityRequestKindTask,
		Subject: &padded, Source: "manual",
	})
	if err != nil || in.Subject == nil || *in.Subject != "Call the buyer" {
		t.Errorf("a padded subject became %v (err %v), want it trimmed", in.Subject, err)
	}

	note := "A note needs no subject"
	if _, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:    crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote,
		Subject: nil, Body: &note, Source: "manual",
	}); err != nil {
		t.Errorf("a note without a subject was refused: %v", err)
	}
}
