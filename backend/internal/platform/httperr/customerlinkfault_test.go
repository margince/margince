// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// A merge cannot move an old invoice or payment that names no customer link.
// That is a conflict the caller can resolve, not a value in the request.
func TestARowNamingNoCustomerLinkIsAConflictThatSaysWhatToDo(t *testing.T) {
	for _, constraint := range []string{"finance_invoice_names_its_customer_link", "finance_payment_names_its_customer_link"} {
		t.Run(constraint, func(t *testing.T) {
			cause := &pgconn.PgError{Code: "23514", ConstraintName: constraint}
			fault, ok := Classify(fmt.Errorf("move the declared rows: %w", cause))
			if !ok {
				t.Fatal("the refusal reached the unhandled path")
			}
			if fault.Status != http.StatusConflict || fault.Code != "conflict" {
				t.Errorf("fault = %d %q, want 409 conflict", fault.Status, fault.Code)
			}
			if fault.Detail == "" || fault.InfraCause == nil {
				t.Errorf("fault = %+v, want a detail saying what to do and the cause kept for the operator", fault)
			}
		})
	}
}
