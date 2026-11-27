// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A merge cannot name a customer link on an invoice or payment mirrored before
// rows named one. The merge and its preview refuse up front, with a message
// that names no finance record, and nothing moves. Each table refuses alone.
func TestAMergeRefusesToMoveLedgerRowsThatNameNoCustomerLink(t *testing.T) {
	t.Parallel()
	for name, unlinked := range map[string][]string{
		"an invoice and a payment": {"finance_invoice", "finance_payment"},
		"a payment alone":          {"finance_payment"},
	} {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			store := financeStoreFor(e, testBooksSource{})
			admin := connectAndSync(t, e, store)
			src, tgt := seedMergePair(t, e, "Unlinked ledger")
			if _, err := store.LinkCustomer(admin, "CUST-A", src); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SyncConnection(admin, testBooksSource{paid: true}); err != nil {
				t.Fatalf("sync: %v", err)
			}
			testdb.WithCustomerLinkChecksLifted(t, func(tx pgx.Tx) error {
				for _, table := range unlinked {
					if _, err := tx.Exec(t.Context(), `UPDATE `+table+` SET customer_link_id = NULL WHERE company_id = $1`, src); err != nil {
						return err
					}
				}
				return nil
			})

			_, previewErr := e.Contacts.PreviewMergeSurvivorship(admin, "company", src.UUID, tgt.UUID, nil, nil)
			_, mergeErr := e.Contacts.MergeCompany(admin, src, tgt, nil)
			for step, err := range map[string]error{"preview": previewErr, "merge": mergeErr} {
				var refused *contacts.UnlinkedLedgerRowsError
				if !errors.As(err, &refused) || !errors.Is(err, apperrors.ErrConflict) {
					t.Errorf("%s = %v, want the unlinked ledger conflict", step, err)
				}
			}
			for _, table := range []string{"finance_invoice", "finance_payment"} {
				if n := e.WsCount(t, `SELECT count(*) FROM `+table+` WHERE company_id = $1`, src); n != 1 {
					t.Errorf("the merged-away company holds %d %s rows after the refusal, want its 1", n, table)
				}
			}
		})
	}
}
