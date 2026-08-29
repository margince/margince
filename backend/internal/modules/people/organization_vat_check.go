// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What the EU register said about a company's VAT ID, and the receipt for
// having asked.
//
// The profile field holds the number a page stated. This holds what happened
// when somebody checked it: the answer, the date, and — when the installation
// consulted under its own VAT number — the consultation number VIES issues as
// proof. That receipt is the whole reason the check is stored rather than
// performed and forgotten: it is what a business shows to say it verified its
// counterpart before treating a supply as intra-community.
//
// One row per company, replaced on re-check. The history of checks is the audit
// log's, which every write here also writes.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// VatCheckStatus is what the register answered.
type VatCheckStatus string

// The three things a consultation can conclude. Valid and invalid are answers
// ABOUT the number; unavailable is the register declining to answer, and is a
// fact about the lookup rather than about the company.
const (
	VatCheckValid       VatCheckStatus = "valid"
	VatCheckInvalid     VatCheckStatus = "invalid"
	VatCheckUnavailable VatCheckStatus = "unavailable"
)

// The audit-image keys for a VAT check. The before and after images must
// describe the same shape, or a reader diffing them sees changes that did not
// happen — so both build from these constants rather than from literals.
const (
	auditKeyVatNumber          = "vat_number"
	auditKeyVatStatus          = "vat_status"
	auditKeyVatConsultationNum = "vat_consultation_number"
	auditKeyVatRegisteredName  = "vat_registered_name"
	// The address and the date belong in the image for the same reason the
	// receipt does: the row keeps only the CURRENT consultation, so what a
	// re-check overwrote is reconstructable from the audit trail or from
	// nowhere. A receipt without the date it was issued proves nothing.
	auditKeyVatRegisteredAddr = "vat_registered_address"
	auditKeyVatCheckedAt      = "vat_checked_at"
)

// VatCheck is one company's current VAT standing.
type VatCheck struct {
	OrganizationID ids.OrganizationID
	// Number is the VAT ID AS CONSULTED. It is kept beside the answer because a
	// profile field edited afterwards must not silently inherit this proof: a
	// receipt names the number it was issued for.
	Number string
	Status VatCheckStatus
	// ConsultationNumber is VIES's receipt, empty when it issued none.
	ConsultationNumber string
	RegisteredName     string
	RegisteredAddress  string
	CheckedAt          time.Time
}

// ErrVatCheckNotRecorded says this company's VAT ID has never been consulted.
// Distinct from a check that came back invalid: one is an absence of evidence,
// the other is evidence.
var ErrVatCheckNotRecorded = errors.New("people: no VAT check is recorded for this organization")

// VatCheckEnqueue hands the consultation to whatever runs jobs, inside the
// caller's transaction.
//
// Nil-safe by contract: a deployment that checks no VAT numbers passes nil, the
// number still writes, and no job is queued. Same shape as GeocodeEnqueue, and
// for the same reason — the number is what the page stated; the verification is
// what this installation can offer.
type VatCheckEnqueue func(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID) error

// enqueueVatCheck queues the consultation a VAT-number write earns, on the
// caller's transaction so the job and the number commit together.
func (s *Store) enqueueVatCheck(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID) error {
	if s.vatCheckEnqueue == nil {
		return nil
	}
	return s.vatCheckEnqueue(ctx, tx, orgID)
}

// statesAVatNumber reports whether an accepted set of read-back fields carries
// a VAT ID with a value — the condition that earns a consultation.
func statesAVatNumber(fields []ColdStartFieldInput) bool {
	for _, f := range fields {
		if f.Field == fieldRegisterVat && strings.TrimSpace(f.Value) != "" {
			return true
		}
	}
	return false
}

// VatNumberForCheck answers the number this company states, and whether asking
// about it is worth a consultation.
//
// ok is false when the company states no number, or when the number it states
// has already been consulted — the worker does not need to tell those apart,
// because both mean "do not ask". A CORRECTED number is worth asking about
// again: the stored check names the number it was issued for, so a number that
// no longer matches it has never been checked.
func (s *Store) VatNumberForCheck(ctx context.Context, org ids.OrganizationID) (string, bool, error) {
	if err := auth.Require(ctx, entityOrganization, principal.ActionRead); err != nil {
		return "", false, err
	}
	var number string
	var worth bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, entityOrganization, org.UUID); err != nil {
			return err
		}
		// Worth asking when the number has never been consulted, when it has
		// CHANGED since it was, or when the last consultation got no answer.
		//
		// The comparison is on the trimmed value, because that is what was
		// consulted: an extracted field holding " DE123456789 " is the same
		// number as the stored one, and treating it as different would spend
		// another consultation on every enqueue forever.
		//
		// An `unavailable` row is a register that declined, not a verdict, so
		// it must not silence the number for good — a later read asks again.
		// A verdict that has gone stale is a different question, and one this
		// lane deliberately does not answer: nothing re-reads a website on a
		// schedule either.
		const read = `
			SELECT f.value,
			       c.organization_id IS NULL
			       OR btrim(c.vat_number) IS DISTINCT FROM btrim(f.value)
			       OR c.status = 'unavailable'
			  FROM organization_profile_field f
			  LEFT JOIN organization_vat_check c ON c.organization_id = f.organization_id
			 WHERE f.organization_id = $1 AND f.field = 'register_vat'`
		err := tx.QueryRow(ctx, read, org.UUID).Scan(&number, &worth)
		if errors.Is(err, pgx.ErrNoRows) {
			// The company states no VAT number. Not an error: most do not.
			return nil
		}
		return err
	})
	if err != nil {
		return "", false, fmt.Errorf("people: reading the VAT number to check: %w", err)
	}
	return strings.TrimSpace(number), worth && strings.TrimSpace(number) != "", nil
}

// RecordVatCheck stores what the register answered about a company's VAT ID.
//
// It is an UPDATE gate rather than a create: the row is a fact about the
// organization, so writing it is changing the organization's record and takes
// the same authority. A caller who may not write the company may not attach a
// verification to it either.
func (s *Store) RecordVatCheck(ctx context.Context, check VatCheck) error {
	if err := auth.Require(ctx, entityOrganization, principal.ActionUpdate); err != nil {
		return err
	}
	number := strings.TrimSpace(check.Number)
	if number == "" {
		return fmt.Errorf("people: a VAT check names no number")
	}
	if check.Status != VatCheckValid && check.Status != VatCheckInvalid && check.Status != VatCheckUnavailable {
		return fmt.Errorf("people: %q is not a VAT check status", check.Status)
	}
	// A consultation number beside "the service did not answer" would be a
	// receipt for a check that did not happen. The schema refuses it too; this
	// says so in a sentence rather than as a constraint violation.
	if check.Status == VatCheckUnavailable && strings.TrimSpace(check.ConsultationNumber) != "" {
		return fmt.Errorf("people: an unavailable VAT lookup carries no consultation number")
	}

	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureOrgWritable(ctx, tx, check.OrganizationID); err != nil {
			return err
		}
		before, err := readVatCheckTx(ctx, tx, check.OrganizationID)
		if err != nil && !errors.Is(err, ErrVatCheckNotRecorded) {
			return err
		}
		// The authenticated principal, never anything the caller supplied: a
		// check run by the ingestion worker is the system's, one a rep asked
		// for is theirs, and the row must say which.
		writer, byErr := storekit.CapturedBy(ctx)
		if byErr != nil {
			return byErr
		}
		if err := upsertVatCheckTx(ctx, tx, check, writer); err != nil {
			return err
		}
		after := map[string]any{
			auditKeyVatNumber:          number,
			auditKeyVatStatus:          string(check.Status),
			auditKeyVatConsultationNum: check.ConsultationNumber,
			auditKeyVatRegisteredName:  check.RegisteredName,
			auditKeyVatRegisteredAddr:  check.RegisteredAddress,
			auditKeyVatCheckedAt:       check.CheckedAt,
		}
		// A first check REPLACES nothing: there was no answer, no receipt and
		// no register name. A later one moved all three, and says what they
		// were — which is the question a re-check raises, because a number that
		// was valid and now is not is the finding.
		// Two doors, and they are not interchangeable: Audit refuses a nil
		// before-image outright, because an update claiming to have moved
		// something from nowhere is the shape that hides a lost prior state.
		var auditID ids.UUID
		var auditErr error
		if prior := vatCheckPriorImage(before, errors.Is(err, ErrVatCheckNotRecorded)); prior == nil {
			auditID, auditErr = storekit.AuditEvent(ctx, tx, "update", entityOrganization,
				check.OrganizationID.UUID, after)
		} else {
			auditID, auditErr = storekit.Audit(ctx, tx, "update", entityOrganization,
				check.OrganizationID.UUID, prior, after)
		}
		if auditErr != nil {
			return auditErr
		}
		// The organization now says something it did not before — whether its
		// stated VAT ID is real — so the fact is announced like any other
		// change to the record, under the verb the closed catalog already has
		// for it. A subscriber that mirrors companies needs to learn a number
		// went bad exactly as much as it needs to learn a name changed.
		//
		// The VERDICT travels and the receipt does not. A subscriber acts on
		// whether the number is real; the consultation number is evidence this
		// installation holds for its own filings, and putting it on the bus
		// would copy it to every mirror that ever subscribed.
		return storekit.EmitEvent(ctx, tx, auditID, check.OrganizationID.UUID,
			crmcontracts.PublicEventOrganizationUpdated{
				ChangedFields: map[string]any{auditKeyVatStatus: string(check.Status)},
			})
	})
}

// vatCheckPriorImage is what the audit row records this check as having
// replaced, or nil when it replaced nothing.
//
// A first consultation moved nothing: there was no verdict, no receipt and no
// register name. A later one moved all three, and what they WERE is the
// question a re-check raises — a number that was valid and now is not is the
// finding this lane exists for.
//
//craft:ignore naked-any an audit image is jsonb, and storekit.Audit takes exactly this shape
func vatCheckPriorImage(before VatCheck, first bool) map[string]any {
	if first {
		return nil
	}
	return map[string]any{
		auditKeyVatNumber:          before.Number,
		auditKeyVatStatus:          string(before.Status),
		auditKeyVatConsultationNum: before.ConsultationNumber,
		auditKeyVatRegisteredName:  before.RegisteredName,
		auditKeyVatRegisteredAddr:  before.RegisteredAddress,
		auditKeyVatCheckedAt:       before.CheckedAt,
	}
}

// VatCheckFor reads a company's current VAT standing.
func (s *Store) VatCheckFor(ctx context.Context, org ids.OrganizationID) (VatCheck, error) {
	if err := auth.Require(ctx, entityOrganization, principal.ActionRead); err != nil {
		return VatCheck{}, err
	}
	var found VatCheck
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// A read carries the row-scope gate: a company this caller cannot see
		// must not disclose its verification either, and 404 keeps existence
		// hidden.
		if err := auth.EnsureVisible(ctx, tx, entityOrganization, org.UUID); err != nil {
			return err
		}
		var readErr error
		found, readErr = readVatCheckTx(ctx, tx, org)
		return readErr
	})
	return found, err
}

// upsertVatCheckTx writes the current standing, replacing any prior one.
func upsertVatCheckTx(ctx context.Context, tx pgx.Tx, check VatCheck, capturedBy string) error {
	const write = `
		INSERT INTO organization_vat_check
		    (organization_id, vat_number, status, consultation_number,
		     registered_name, registered_address, checked_at, captured_by)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7, $8)
		ON CONFLICT (organization_id) DO UPDATE SET
		    vat_number = EXCLUDED.vat_number,
		    status = EXCLUDED.status,
		    consultation_number = EXCLUDED.consultation_number,
		    registered_name = EXCLUDED.registered_name,
		    registered_address = EXCLUDED.registered_address,
		    checked_at = EXCLUDED.checked_at,
		    captured_by = EXCLUDED.captured_by,
		    version = organization_vat_check.version + 1,
		    updated_at = now()`
	_, err := tx.Exec(ctx, write,
		check.OrganizationID.UUID, strings.TrimSpace(check.Number), string(check.Status),
		strings.TrimSpace(check.ConsultationNumber), strings.TrimSpace(check.RegisteredName),
		strings.TrimSpace(check.RegisteredAddress), check.CheckedAt, capturedBy)
	if err != nil {
		return fmt.Errorf("people: recording the VAT check: %w", err)
	}
	return nil
}

// readVatCheckTx reads the standing on the caller's transaction.
func readVatCheckTx(ctx context.Context, tx pgx.Tx, org ids.OrganizationID) (VatCheck, error) {
	const read = `
		SELECT vat_number, status, COALESCE(consultation_number, ''),
		       COALESCE(registered_name, ''), COALESCE(registered_address, ''), checked_at
		  FROM organization_vat_check
		 WHERE organization_id = $1`
	found := VatCheck{OrganizationID: org}
	var status string
	err := tx.QueryRow(ctx, read, org.UUID).Scan(&found.Number, &status,
		&found.ConsultationNumber, &found.RegisteredName, &found.RegisteredAddress, &found.CheckedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return VatCheck{}, ErrVatCheckNotRecorded
	}
	if err != nil {
		return VatCheck{}, fmt.Errorf("people: reading the VAT check: %w", err)
	}
	found.Status = VatCheckStatus(status)
	return found, nil
}
