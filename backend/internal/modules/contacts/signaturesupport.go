// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which mails still state a value a signature filled. A further address and a
// postal address are kept while some mail supports them: each supporting mail
// is a key of the evidence row's signature_support, with the snippet it quoted
// and whether it also vouched for the employer's street.

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/salutation"
)

// quotesValue holds when the snippet prints the value, case and spacing aside.
// A value the snippet does not print is the model's, not the contact's.
func quotesValue(snippet, value string) bool {
	fold := func(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
	return value != "" && strings.Contains(fold(snippet), fold(value))
}

// quotesAddress holds when the snippet prints the address as a whole token,
// so a longer address that ends in it does not count.
func quotesAddress(snippet, address string) bool {
	return slices.Contains(salutation.PrintedAddresses(snippet), strings.ToLower(address))
}

// dropSignatureSupportTx removes one narrowed mail's support. A row another
// mail still supports moves its evidence onto that mail. A row left with no
// support keeps the narrowed mail as its source, so the take-back finds it. An
// address row that lost its last street support takes the street back.
func dropSignatureSupportTx(ctx context.Context, tx pgx.Tx, sourceRef string) error {
	lost, err := dropProfileFieldSupportTx(ctx, tx, sourceRef)
	if err != nil {
		return err
	}
	for _, row := range lost {
		if !row.StreetLost {
			continue
		}
		if err := withdrawEmployerStreetTx(ctx, tx, row.Value, row.ContactID); err != nil {
			return err
		}
	}
	return nil
}
