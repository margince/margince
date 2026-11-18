// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// StatedCountryError refuses a recipient country stated by anyone but the user
// writing the message. A stated or cleared country can put a recipient under
// laxer rules or none. It is the user's correction of a wrong record, never an
// agent's or an integration's.
type StatedCountryError struct{}

func (*StatedCountryError) Error() string {
	return "only the user writing a message may state a recipient's country"
}

// FieldFault names the field to drop.
func (e *StatedCountryError) FieldFault() (field, code, message string) {
	return "recipient_countries", "invalid", e.Error()
}

// refuseAMachineStatedCountry admits a stated country from a human at the
// keyboard, through the check the direct send uses.
func refuseAMachineStatedCountry(ctx context.Context, req commsauthz.Request) error {
	if len(req.Evidence.RecipientCountries) == 0 {
		return nil
	}
	if requireAHumanAtTheKeyboard(ctx) != nil {
		return &StatedCountryError{}
	}
	return nil
}
