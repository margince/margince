// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// GDPR Art. 15 subject-access assembly (admin-mediated in V1): one
// operation gathers everything held about a person — the normalized
// row, channels, relationships, deals they hold a stake in, timeline
// activities, consent state and proof log, and the raw capture
// payloads that mention them — into a single export package. The
// export is itself audited (action=export): who pulled whose data,
// when.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SARPackage is the assembled export. Sections hold raw row maps —
// the package is a data handover, not an API shape.
type SARPackage struct {
	Subject map[string]any   `json:"subject"`
	Emails  []map[string]any `json:"emails"`
	Phones  []map[string]any `json:"phones"`
	// The messaging-channel accounts bound to the subject: which provider
	// identity writes as them, the handle it carries, whether they have blocked
	// this installation's bot, and whether the binding is still live.
	ChannelIdentities []map[string]any `json:"channel_identities"`
	// Which conversations the subject was recorded as being IN, and in what
	// role (ACT-DDL-3). Distinct from Activities, which is what was said: this
	// is the record that they were a party to it at all, and it is held about
	// them whether or not they were ever a contact.
	InteractionParticipation []map[string]any `json:"interaction_participation"`
	// Where the subject appears in a colleague's imported LinkedIn network.
	// They never consented to that import and would have no way to know of it.
	LinkedInConnections []map[string]any `json:"linkedin_connections"`
	Relationships       []map[string]any `json:"relationships"`
	Deals               []map[string]any `json:"deals"`
	Leads               []map[string]any `json:"leads"`
	Activities          []map[string]any `json:"activities"`
	Attachments         []map[string]any `json:"attachments"`
	Consent             []map[string]any `json:"consent"`
	ConsentEvents       []map[string]any `json:"consent_events"`
	// The deterministic thing on the record that made business correspondence
	// lawful, which the consent state alone does not say.
	ConsentQualifyingEvents []map[string]any `json:"consent_qualifying_events"`
	// Why this contact exists: the answer to Art. 15(1)(g).
	AcquisitionEvidence []map[string]any `json:"acquisition_evidence"`
	// What the subject themselves sent through their confirm link — a
	// correction they typed, or a request to be removed. The one part of this
	// package the subject authored rather than the workspace, which is exactly
	// why an export that omitted it would be answering the wrong question.
	ConfirmSubmissions []map[string]any `json:"confirm_submissions"`
	// Why each message to this subject was permitted, what non-consent basis
	// stood behind it, and every objection or restriction they recorded.
	// Art. 15(1)(a)-(c): the purposes of the processing and its lawful ground,
	// answered per message rather than in the abstract.
	CommunicationDecisions   []map[string]any `json:"communication_decisions"`
	CommunicationBases       []map[string]any `json:"communication_bases"`
	CommunicationSuppression []map[string]any `json:"communication_suppression"`
	RawCapture               []map[string]any `json:"raw_capture"`
	FieldOrigins             []map[string]any `json:"field_origins"`
	// EnrichedFields is what the system read about the subject from a public
	// page or a mail signature, each with the verbatim text it came from.
	// Art. 15(1)(g) makes the source itself disclosable, and the snippet IS
	// the source.
	EnrichedFields []map[string]any `json:"enriched_fields"`
	// Corrections is what a human recorded over what the system inferred
	// about this subject. It is theirs twice over: the value was typed by a
	// person about them, and the suppressions are the record of which claims
	// this installation has agreed to stop making.
	Corrections []map[string]any `json:"corrections"`
	// ProviderClaims is what a licensed data provider asserted about the
	// subject and this installation retained — bought from a third party
	// rather than given by them, which is precisely the holding Art. 15(1)(g)
	// makes the SOURCE of disclosable too.
	ProviderClaims []map[string]any `json:"provider_claims"`
	// ProviderAppliedFields is what those purchases put ON the record: which
	// field each run filled, and for a plain column the value it wrote. A
	// subject asking what we hold is owed the fact that a purchase changed
	// their record, not only that we made one.
	ProviderAppliedFields []map[string]any `json:"provider_applied_fields"`
	// ProviderRuns is why and when that purchase happened: which provider was
	// asked, what was requested, what came back. Art. 15(1)(a)-(d) asks for
	// the purposes and the categories, and a values-only export would answer
	// what we hold while hiding that we went out and bought it.
	ProviderRuns []map[string]any `json:"provider_runs"`
	// What capture decided about the subject's own address, and why — an
	// automated decision the subject is owed sight of (CAP-DDL-8).
	CaptureDispositions []map[string]any `json:"capture_dispositions"`
	// The governed outbound messages addressed to the subject: what was sent
	// to them, when, and whether it left (comms_outbound).
	SentMessages []map[string]any `json:"sent_messages"`
	// The messages addressed to the subject that have NOT been sent: waiting
	// for their moment, withdrawn, or held for a human (scheduled_send,
	// ADR-0104). Held apart from SentMessages because the distinction is the
	// subject's to know — one is a message they received, the other is one
	// somebody wrote to them that the system is still holding.
	ScheduledMessages []map[string]any `json:"scheduled_messages"`
	// The messages written to the subject that nobody has DECIDED yet: an
	// automation composed them and they are waiting in somebody's approval
	// inbox (#707). Held apart from ScheduledMessages for the same reason that
	// is held apart from SentMessages — the distinction is the subject's to
	// know. One is a message with a moment already chosen; this is one a
	// colleague has not yet agreed to send at all, and may never.
	StagedMessages []map[string]any `json:"staged_messages"`
}

// AssembleSAR builds the package. It is a privileged read: the caller must be
// a human holding the person.delete grant (the same trust level erasure needs)
// over an unbounded row scope — see the checks below.
func AssembleSAR(ctx context.Context, db *database.DB, personID ids.PersonID) (SARPackage, error) {
	if err := auth.Require(ctx, "person", principal.ActionDelete); err != nil {
		return SARPackage{}, err
	}
	// Human-only, for the reason the grant above cannot express: an agent
	// acting under a passport carries the granting human's live grants, so an
	// admin's read-scoped passport would otherwise assemble a subject's entire
	// Art. 15 package — every activity, capture row, correction and outbound
	// message.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return SARPackage{}, fmt.Errorf("human-only subject-access assembly: %w", apperrors.ErrPermissionDenied)
	}
	// The assembly deliberately crosses the caller's row scope — Art. 15 owes
	// the subject everything held, not the slice one rep may see — so a bounded
	// caller cannot run it. Scope is the second condition, not a stand-in for
	// authority: the person.delete grant above is what limits this to the roles
	// trusted with erasure, and it is what keeps read_only out.
	//
	// Together those two admit MORE than admin: the seeded defaults give
	// person.delete and row scope `all` to ops and management as well. That is
	// this function's contract — the erasure trust level — and callers that owe
	// a narrower one gate before they get here. The subject-request route does:
	// it reads the request through the queue's own admin-only gate first, which
	// TestTheQueueGateIsWhatKeepsTheExportAdminOnly pins from both sides. A
	// future caller assembling from a person id alone would be reachable by two
	// more roles, and would need its own gate.
	if !auth.Unbounded(actor) {
		return SARPackage{}, apperrors.ErrPermissionDenied
	}
	var pkg SARPackage
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleForSubjectRights(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		// The subject's addresses and lead twins, read BEFORE the sections run:
		// the staged-approvals section matches on them, and unlike erasure this
		// path destroys nothing, so they are still there to read.
		emails, leads, err := subjectReach(ctx, tx, personID)
		if err != nil {
			return err
		}
		sections := sarSections(&pkg, personID, emails, leads)

		subject, err := rowMaps(ctx, tx, `
			SELECT p.id, p.full_name, p.first_name, p.last_name, p.title,
			       (SELECT jsonb_object_agg(ps.platform, ps.handle) FROM person_social ps WHERE ps.person_id = p.id) AS social,
			       p.address_line1, p.address_line2, p.address_city, p.address_region, p.address_postal_code, p.address_country,
			       p.source, p.created_at
			FROM person p WHERE p.id = $1`, personID)
		if err != nil {
			return err
		}
		if len(subject) == 0 {
			return apperrors.ErrNotFound
		}
		pkg.Subject = subject[0]
		if err := appendSubjectCustomValues(ctx, tx, personID, pkg.Subject); err != nil {
			return err
		}

		for _, section := range sections {
			args := section.args
			if args == nil {
				args = []any{personID}
			}
			rows, err := rowMaps(ctx, tx, section.query, args...)
			if err != nil {
				return err
			}
			*section.dest = rows
		}

		_, err = storekit.Audit(ctx, tx, "export", "person", personID.UUID, nil, map[string]any{
			"kind": "sar", "activities": len(pkg.Activities), "raw_rows": len(pkg.RawCapture),
		})
		return err
	})
	return pkg, err
}

// appendSubjectCustomValues merges the subject's stored cf_ values into
// the export's subject map, keyed by column name. The column set comes
// from the catalog with ANY status (see subjectcolumns.go): Art. 15 owes
// the subject everything HELD, and a retired field's column still stores
// its values. Extraction rides the same storekit mechanics the record
// surface reads with, so each value exports in its documented wire shape;
// a NULL column stays absent, like every other empty section detail.
func appendSubjectCustomValues(ctx context.Context, tx pgx.Tx, personID ids.PersonID, subject map[string]any) error {
	columns, err := subjectCustomColumns(ctx, tx, "person")
	if err != nil || len(columns) == 0 {
		return err
	}
	dests := storekit.ScanDests(columns)
	query := `SELECT ` + strings.TrimPrefix(storekit.SelectSuffix(columns), ", ") + ` FROM person WHERE id = $1`
	if err := tx.QueryRow(ctx, query, personID).Scan(dests...); err != nil {
		return err
	}
	for name, value := range storekit.ExtractValues(columns, dests) {
		subject[name] = value
	}
	return nil
}

// sarSection pairs a destination package section with the query that fills
// it. Every query is keyed to the single personID bound param ($1).
type sarSection struct {
	dest  *[]map[string]any
	query string
	// args overrides the default single person-id argument. Only the staged
	// approvals section needs it: it reaches rows by the subject's addresses
	// and lead twins as well as by their person id, through the SAME predicate
	// erasure uses — and that predicate takes those as bound parameters rather
	// than rebuilding them in SQL, so the export and the erasure cannot come to
	// different conclusions about which rows are the subject's.
	args []any
}

// rowMaps runs one query and returns each row as column→value.
func rowMaps(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]map[string]any, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(values))
		for i, field := range rows.FieldDescriptions() {
			row[field.Name] = values[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// subjectReach reads the two things beyond a person id that identify the
// subject in a staged proposal: the addresses a message to them carries, and
// the lead rows that were the same human before promotion.
//
// Read here rather than derived inside the query so the export can hand the
// SAME bound parameters to subjectApprovalMatch that the erasure hands it.
// Erasure cannot read them at that point — it has already destroyed them — so
// the predicate takes them as arguments and each caller supplies them from
// wherever it still can.
func subjectReach(ctx context.Context, tx pgx.Tx, personID ids.PersonID) ([]string, []ids.UUID, error) {
	emails, err := subjectStrings(ctx, tx,
		`SELECT email FROM person_email WHERE person_id = $1 AND email <> ''`, personID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the subject's addresses for the export: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT id FROM lead WHERE promoted_person_id = $1`, personID.UUID)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the subject's lead twins for the export: %w", err)
	}
	defer rows.Close()
	leads := []ids.UUID{}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		leads = append(leads, id)
	}
	return emails, leads, rows.Err()
}

// subjectStrings runs a one-column text query into a slice.
func subjectStrings(ctx context.Context, tx pgx.Tx, query string, personID ids.PersonID) ([]string, error) {
	rows, err := tx.Query(ctx, query, personID.UUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
