// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

const (
	admissionDomainArg = "domain"
	admissionReasonArg = "reason"
	admissionHumanArg  = "human"
)

// The admission lock keeps the prior image and upsert on the same decision.
// Without it, a concurrent writer could change the row after the CTE snapshot.
func setDomainAdmissionTx(ctx context.Context, tx pgx.Tx, domain, admission string, reason domainAdmissionReason, source string) (map[string]any, bool, error) {
	if reason.Code == "" || reason.Text == "" {
		return nil, false, errors.New("contacts: a domain admission needs a reason code and readable reason")
	}
	if err := lockDomainAdmissionTx(ctx, tx, domain); err != nil {
		return nil, false, err
	}
	// A triage row without an admission has no prior decision to audit.
	var priorJSON []byte
	err := tx.QueryRow(ctx, `
		WITH was(admission, admission_reason, admission_source, admission_reason_code, admission_sender_kind) AS (
		  SELECT d.admission, d.admission_reason, d.admission_source, `+domainReasonCodeColumns+`
		    FROM company_domain_disposition d WHERE d.domain = @domain
		)
		INSERT INTO company_domain_disposition (domain, status, admission, admission_reason, admission_source, admission_at, admission_reason_code, admission_sender_kind, admission_reason_coded_at)
		VALUES (
		        @domain, @status, @admission, @reason, @source, now(), @reason_code, NULLIF(@sender_kind, ''), now())
		ON CONFLICT (domain) DO UPDATE
		   SET admission = EXCLUDED.admission,
		       admission_reason = EXCLUDED.admission_reason,
		       admission_reason_code = EXCLUDED.admission_reason_code,
		       admission_reason_coded_at = EXCLUDED.admission_reason_coded_at,
		       admission_sender_kind = EXCLUDED.admission_sender_kind,
		       admission_source = EXCLUDED.admission_source,
		       admission_at = now(),
		       -- A refused domain is not waiting on evidence; it has an answer.
		       -- Only a suppression clears the marker, because admitting one
		       -- leaves the company question genuinely open again.
		       pending_reason = CASE WHEN EXCLUDED.admission = 'suppressed'
		                             THEN NULL ELSE company_domain_disposition.pending_reason END,
		       -- next_attempt_at means scheduled work, and a refused domain has
		       -- none: nothing will crawl it while the refusal stands.
		       next_attempt_at = CASE WHEN EXCLUDED.admission = 'suppressed'
		                              THEN NULL ELSE company_domain_disposition.next_attempt_at END,
		       updated_at = now()
		 -- The sticky rule: an automatic caller may not overwrite a decision a
		 -- HUMAN made, while a human may overwrite anything. Guarded on the
		 -- source already stored, not on the value being written, because what
		 -- must survive is the authority behind the row rather than which way
		 -- it happened to point.
		 WHERE company_domain_disposition.admission_source IS DISTINCT FROM @human
		    OR EXCLUDED.admission_source = @human
		RETURNING (SELECT jsonb_build_object(
		             'admission', was.admission,
		             'admission_reason', coalesce(was.admission_reason, ''),
		             'admission_source', coalesce(was.admission_source, ''),
		             'admission_reason_code', was.admission_reason_code,
		             'admission_sender_kind', was.admission_sender_kind)
		             FROM was WHERE was.admission IS NOT NULL)`,
		pgx.NamedArgs{
			admissionDomainArg: domain, "status": DomainPending, "admission": admission,
			admissionReasonArg: reason.Text, auditKeySource: source, admissionHumanArg: AdmissionSourceHuman,
			"reason_code": string(reason.Code), "sender_kind": string(reason.SenderKind),
		}).Scan(&priorJSON)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// The sticky rule declined this write: a machine may not overwrite a
		// human's decision. Nothing moved, so there is no prior state to name.
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("contacts: recording the admission of %s: %w", domain, err)
	case priorJSON == nil:
		return nil, true, nil
	}
	var prior map[string]any
	if err := json.Unmarshal(priorJSON, &prior); err != nil {
		return nil, false, fmt.Errorf("contacts: reading the decision %s carried before: %w", domain, err)
	}
	return prior, true, nil
}

// lockDomainAdmissionTx serializes every writer of one domain's admission, so
// the decision a write replaces is read under the same lock that replaces it.
func lockDomainAdmissionTx(ctx context.Context, tx pgx.Tx, domain string) error {
	return storekit.LockWriteIdentity(ctx, tx, admissionWriteIdentity, domain)
}

// admissionWriteIdentity names the write identity a domain's admission
// decisions serialize on.
const admissionWriteIdentity = "domain_admission"
