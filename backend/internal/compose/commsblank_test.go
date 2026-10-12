// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A tool call that leaves out `subject` or `body` reaches the adapter as "".
// Both send tools refuse it before any consent decision or staging, as the
// HTTP door refuses a body without the keys.
func TestAMailSendToolWithABlankSubjectOrBodyIsRefusedBeforeStaging(t *testing.T) {
	adapter := commsAdapter{}
	ok := agents.SendEmailArgs{To: []string{"buyer@example.test"}, Subject: "Hi", Body: "Hello"}
	cases := map[string]struct {
		mutate func(*agents.SendEmailArgs)
		field  string
	}{
		"no subject":   {func(a *agents.SendEmailArgs) { a.Subject = "" }, "subject"},
		"blank body":   {func(a *agents.SendEmailArgs) { a.Body = "  \n" }, "body"},
		"both missing": {func(a *agents.SendEmailArgs) { a.Subject, a.Body = "", "" }, "subject"},
	}
	for name, c := range cases {
		args := ok
		c.mutate(&args)
		sends := map[string]func() error{
			"reply": func() error {
				_, err := adapter.SendEmail(context.Background(), ids.NewV7(), args)
				return err
			},
			"company": func() error {
				_, err := adapter.SendCompanyEmail(context.Background(), nil, args)
				return err
			},
		}
		for door, send := range sends {
			t.Run(name+" "+door, func(t *testing.T) {
				var required *activities.RequiredFieldError
				if err := send(); !errors.As(err, &required) || required.Field != c.field {
					t.Fatalf("err = %v, want a required-field refusal naming %s", err, c.field)
				}
			})
		}
	}
}
