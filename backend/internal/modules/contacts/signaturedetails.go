// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What a signature states beyond the single-answer fields: a further email
// address, the postal address as structured parts, and the employer's street.
// Each only fills what is empty, never a value a colleague set or cleared.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// addressColumnsSQL lists a record's six address parts in the order
// contactAddressArguments binds them.
const addressColumnsSQL = `address_line1, address_line2, address_region, address_postal_code, address_city, address_country`

// addressDecidedSQL holds when a colleague's or their agent's edit of record
// alias named its address. A machine fill never writes over that decision,
// a cleared address included.
func addressDecidedSQL(entity, alias string) string {
	return `EXISTS (SELECT 1 FROM audit_log decided
	  WHERE decided.entity_type = '` + entity + `' AND decided.entity_id = ` + alias + `.id
	    AND decided.action = 'update' AND decided.after ? 'address_line1'
	    AND (decided.actor_type = 'human' OR decided.on_behalf_of IS NOT NULL))`
}

// signatureAddressFillSQL fills the contact's address from one split line,
// only while every part is empty.
var signatureAddressFillSQL = `UPDATE contact c SET (` + addressColumnsSQL + `) = ($2, $3, $4, $5, $6, $7)
	 WHERE c.id = $1 AND c.archived_at IS NULL
	   AND (` + addressColumnsSQL + `) IS NOT DISTINCT FROM (NULL::text, NULL::text, NULL::text, NULL::text, NULL::text, NULL::text)
	   AND NOT ` + addressDecidedSQL(entityContact, "c")

// fillSignatureAddressTx writes the structured address a signature's address
// line states. A line the splitter cannot read stays evidence only.
func fillSignatureAddressTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, line string) error {
	address, split := SplitAddressText(line)
	if !split {
		return nil
	}
	if _, err := storekit.LockRow(ctx, tx, entityContact, contactID.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, signatureAddressFillSQL, contactAddressArguments(contactID, address)...); err != nil {
		return fmt.Errorf("contacts: filling the address a signature states: %w", err)
	}
	return nil
}

// contactAddressArguments binds the contact address statements, in the order
// addressColumnsSQL names the parts.
func contactAddressArguments(contactID ids.ContactID, a crmcontracts.Address) []any {
	return []any{contactID, a.Line1, a.Line2, a.Region, a.PostalCode, a.City, a.Country}
}

// withdrawSignatureAddress clears the address parts a signature's line filled,
// while the contact still shows those parts.
func withdrawSignatureAddress(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, line string) error {
	address, split := SplitAddressText(line)
	if !split {
		return nil
	}
	if _, err := storekit.LockRow(ctx, tx, entityContact, contactID.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return err
	}
	args := contactAddressArguments(contactID, address)
	if _, err := tx.Exec(ctx, `UPDATE contact SET (`+addressColumnsSQL+`) = (NULL, NULL, NULL, NULL, NULL, NULL)
		 WHERE id = $1 AND archived_at IS NULL
		   AND (`+addressColumnsSQL+`) IS NOT DISTINCT FROM ($2::text, $3::text, $4::text, $5::text, $6::text, $7::text)`,
		args...); err != nil {
		return fmt.Errorf("contacts: clearing the address a signature filled: %w", err)
	}
	return nil
}

// applySignatureEmails adds each further address a signature lists as a
// non-primary email of the contact, and reports how many landed. An address a
// record already holds stays out, as do erased, seat and machine addresses.
func applySignatureEmails(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, sourceRef string, observedAt time.Time, fields []SignatureField) (int, error) {
	if len(fields) == 0 {
		return 0, nil
	}
	// The subject before the email row, the order Art. 17 erasure takes them in.
	if err := auth.HoldWritableLive(ctx, tx, entityContact, contactID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	landed := 0
	for _, f := range fields {
		address, free, err := freeSignatureEmail(ctx, tx, f.Value)
		if err != nil {
			return 0, err
		}
		if !free {
			continue
		}
		if err := addSignatureEmail(ctx, tx, contactID, address); err != nil {
			return 0, err
		}
		confidence := f.Confidence
		evidence, snippet := machineProfileField(address, f.Evidence)
		if _, err := writeContactProfileField(ctx, tx, contactID, contactProfileFieldRow{
			Field: fieldEmail, Value: evidence, EvidenceSnippet: snippet, SourceRef: sourceRef,
			Source: enrichSource, CapturedBy: enrichCapturedBy, Confidence: &confidence, ObservedAt: &observedAt,
		}, supersedeOnNewerObservation); err != nil {
			return 0, err
		}
		landed++
	}
	if landed == 0 {
		return 0, nil
	}
	return landed, storekit.StampFields(ctx, tx, entityContact, contactID.UUID, sourceRef, enrichCapturedBy,
		[]storekit.FieldStamp{{Field: fieldEmail}})
}

// freeSignatureEmail parses one stated address and answers whether no record,
// seat or erasure holds it, and no sending system owns it.
func freeSignatureEmail(ctx context.Context, tx pgx.Tx, stated string) (string, bool, error) {
	parsed, err := values.ParseEmail(strings.TrimSpace(stated))
	if err != nil || refuseMachineAddress(parsed.String()) != nil {
		return "", false, nil
	}
	address := parsed.String()
	if erased, err := storekit.EmailSuppressed(ctx, tx, address); err != nil || erased {
		return "", false, err
	}
	seats, err := storekit.WorkspaceSeatAddresses(ctx, tx)
	if err != nil || seats.Covers(address, ids.Nil) {
		return "", false, err
	}
	holders, err := storekit.IdentityHoldersOf(ctx, tx, storekit.IdentityKeys{Emails: []string{address}})
	if err != nil {
		return "", false, err
	}
	return address, len(holders.Contacts) == 0 && len(holders.Leads) == 0, nil
}

// addSignatureEmail stores one address through the contact_email writer. It
// identifies the contact and proves no correspondence, so the mail ladder
// does not read it as a settled verdict about the address.
func addSignatureEmail(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, address string) error {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return fmt.Errorf("contacts: a signature's email outside a workspace")
	}
	var next int
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(position) + 1, 0) FROM contact_email
		 WHERE contact_id = $1 AND archived_at IS NULL`, contactID).Scan(&next); err != nil {
		return fmt.Errorf("contacts: placing a signature's email: %w", err)
	}
	return insertContactEmails(ctx, tx, ids.From[ids.WorkspaceKind](wsID), contactID, enrichSource, enrichCapturedBy,
		[]ContactEmailInput{{Email: address, EmailType: emailTypeWork, Position: next, VouchedNotCorresponded: true}},
		storekit.RefuseErased, nil)
}

// withdrawSignatureEmail archives the address a signature read added, while it
// is still the pass's own row and not the contact's primary.
func withdrawSignatureEmail(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, address string) error {
	if _, err := tx.Exec(ctx, `UPDATE contact_email SET archived_at = now()
		 WHERE contact_id = $1 AND email = $2 AND source = $3 AND captured_by = $4
		   AND NOT is_primary AND archived_at IS NULL`, contactID, address, enrichSource, enrichCapturedBy); err != nil {
		return fmt.Errorf("contacts: archiving the email a signature read added: %w", err)
	}
	return nil
}

// fillEmployerStreetTx writes a signature's address onto the contact's one
// current employer when the signature names that employer. The company must
// hold no address a user stated. It is a company mutation of its own.
func fillEmployerStreetTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, sourceRef string, fields []SignatureField) error {
	line, employerName := signatureValue(fields, fieldAddress), signatureValue(fields, fieldCompanyName)
	if line == "" || employerName == "" {
		return nil
	}
	var employers int
	var employer *ids.CompanyID
	if err := tx.QueryRow(ctx, `SELECT count(*), min(o.id::text)::uuid FROM relationship j
		  JOIN company o ON o.id = j.company_id AND o.archived_at IS NULL AND o.merged_into_id IS NULL
		 WHERE j.contact_id = $1 AND `+employment.CurrentMembershipSQL("j")+`
		   AND (f_fold_import_name(o.display_name) = f_fold_import_name($2)
		        OR f_fold_import_name(o.legal_name) = f_fold_import_name($2))`,
		contactID, employerName).Scan(&employers, &employer); err != nil {
		return fmt.Errorf("contacts: reading the employer a signature names: %w", err)
	}
	if employers != 1 || employer == nil {
		return nil
	}
	// With two current employers, the address could be either one's.
	var current int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM relationship j WHERE j.contact_id = $1 AND `+
		employment.CurrentMembershipSQL("j"), contactID).Scan(&current); err != nil || current != 1 {
		return err
	}
	return fillCompanyStreetTx(ctx, tx, *employer, sourceRef, line)
}

func fillCompanyStreetTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, sourceRef, line string) error {
	// A company the pass may not write, or that went, keeps its address. The
	// contact's own fields still land.
	err := auth.HoldWritableLive(ctx, tx, companyEntity, companyID.UUID)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return err
	}
	var decided bool
	if err := tx.QueryRow(ctx, `SELECT `+addressDecidedSQL(companyEntity, "o")+` FROM company o WHERE o.id = $1`,
		companyID).Scan(&decided); err != nil || decided {
		return err
	}
	filled, err := writeCompanyAddress(ctx, tx, companyID, line, fillUnclaimed)
	if err != nil || !filled {
		return err
	}
	if err := storekit.StampFields(ctx, tx, companyEntity, companyID.UUID, sourceRef, enrichCapturedBy,
		[]storekit.FieldStamp{{Field: fieldAddress}}); err != nil {
		return err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", companyEntity, companyID.UUID,
		map[string]any{fieldAddress: nil}, map[string]any{fieldAddress: signatureFieldFilled},
		map[string]any{auditKeySource: enrichSource, auditKeySourceRef: sourceRef})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, companyID.UUID, crmcontracts.PublicEventCompanyUpdated{
		ChangedFields: map[string]any{fieldAddress: signatureFieldFilled, auditKeySource: enrichSource},
	})
}

// signatureValue is the trimmed value of the one field of that name, or empty.
func signatureValue(fields []SignatureField, name string) string {
	for _, f := range fields {
		if f.Name == name {
			return strings.TrimSpace(f.Value)
		}
	}
	return ""
}
