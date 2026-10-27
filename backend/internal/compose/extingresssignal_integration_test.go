// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit landing a signal through Runtime.Ingest. Each case runs on the member's
// live authority over a real database, so what is counted is what the signals
// module wrote.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/signaltypes"
	"github.com/margince/margince/backend/pkg/extension"
)

const signalKind = "ext:" + ingressUnit + ":loss"

// signalPolicy is the grant a member needs beside crmPolicy to have a signal
// filed in their name.
const signalPolicy = `{"objects":{"signal":{"read":true,"create":true}},"row_scope":"all"}`

func setupSignalIngress(t *testing.T) *crmEnv {
	t.Helper()
	e := setupCRMIngress(t)
	signaltypes.Register(ingressUnit, []extension.SignalTypeDeclaration{{
		Kind: "loss", Label: "Loss reported",
		Evidence: []extension.SignalEvidenceField{{Name: "amount", Label: "Amount", Type: extension.SignalFieldNumber, Required: true}},
	}})
	grantPolicy(t, e.Rep1, "signal-probe", signalPolicy)
	return e
}

func signalRecord(key, subjectType, subjectID string) extension.Record {
	return extension.Record{System: crmSystem, Key: key, Signal: &extension.SignalFields{
		Kind:     "loss",
		Subject:  extension.SignalSubject{Type: subjectType, ID: subjectID},
		Evidence: extension.SignalEvidence{Fields: map[string]any{"amount": 40.0}},
	}}
}

func TestASignalLandsOnTheMembersAuthorityWithItsAuditRowAndEvent(t *testing.T) {
	e := setupSignalIngress(t)
	company := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))

	res := e.mustIngest(t, signalRecord("event-1", "company", company.Ref.ID))
	if res.Ref.Type != "signal" {
		t.Fatalf("ref type = %q, want signal", res.Ref.Type)
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM signal WHERE id = $1 AND kind = $2 AND entity_id = $3`,
		ids.MustParse(res.Ref.ID), signalKind, ids.MustParse(company.Ref.ID)); n != 1 {
		t.Errorf("signal rows = %d, want the one the unit landed", n)
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'signal' AND entity_id = $1 AND actor_id = $2 AND on_behalf_of = $3`,
		ids.MustParse(res.Ref.ID), ingressPrincipalPrefix+ingressUnit, e.member); n != 1 {
		t.Errorf("audit rows = %d, want one by the unit's connector identity on behalf of the member", n)
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM event_outbox e JOIN audit_log a ON a.id = (e.envelope->'trace'->>'audit_log_id')::uuid
		WHERE a.entity_id = $1 AND e.envelope->>'type' = 'signal.detected'`, ids.MustParse(res.Ref.ID)); n != 1 {
		t.Errorf("signal.detected events = %d, want one", n)
	}
}

func TestARepeatedSignalIsAlreadyRaisedAndLandsNoSecondRow(t *testing.T) {
	e := setupSignalIngress(t)
	company := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))
	first := e.mustIngest(t, signalRecord("event-1", "company", company.Ref.ID))

	again, err := e.ingest(t, e.member, signalRecord("event-1", "company", company.Ref.ID))
	if err != nil || again.Disposition != extension.DispositionAlreadyRaised || again.Ref != first.Ref {
		t.Fatalf("repeat = %+v, %v, want already raised naming the first signal", again, err)
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM signal WHERE kind = $1`, signalKind); n != 1 {
		t.Errorf("signal rows = %d after a repeat, want one", n)
	}
}

func TestADemotedMemberLandsNoSignalFromTheNextCallOn(t *testing.T) {
	e := setupSignalIngress(t)
	company := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))
	revokeRoles(t, e.extRuntimeEnv, e.member)

	_, err := e.ingest(t, e.member, signalRecord("event-1", "company", company.Ref.ID))
	if !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden for a demoted member", err)
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM signal WHERE kind = $1`, signalKind); n != 0 {
		t.Errorf("signal rows = %d after a refused call, want none", n)
	}
}

func TestASignalAboutARecordTheMemberCannotSeeOrTheUnitDoesNotLandIsRefused(t *testing.T) {
	e := setupSignalIngress(t)
	company := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))
	unknown := signalRecord("event-2", "company", ids.NewV7().String())
	notLanded := signalRecord("event-3", "contact", company.Ref.ID)
	badEvidence := signalRecord("event-4", "company", company.Ref.ID)
	badEvidence.Signal.Evidence.Fields = map[string]any{"amount": "forty"}
	otherKind := signalRecord("event-5", "company", company.Ref.ID)
	otherKind.Signal.Kind = "undeclared"

	for name, tc := range map[string]struct {
		rec  extension.Record
		want error
	}{
		"a subject that is not there for the member": {unknown, extension.ErrNotFound},
		"a subject kind the source did not declare":  {notLanded, extension.ErrIngressNotDeclared},
		"evidence the layout refuses":                {badEvidence, extension.ErrInvalid},
		"a signal type the unit never declared":      {otherKind, extension.ErrInvalid},
	} {
		if _, err := e.ingest(t, e.member, tc.rec); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM signal WHERE kind = $1`, signalKind); n != 0 {
		t.Errorf("signal rows = %d after refused calls, want none", n)
	}
}

func TestAnAttendedCallCannotLandASignal(t *testing.T) {
	e := setupSignalIngress(t)
	company := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))
	rt, ctx := e.runtime(ingressUnit)
	if _, err := rt.Ingest(ctx, extension.UserID(e.member.String()), signalRecord("event-1", "company", company.Ref.ID)); !errors.Is(err, extension.ErrAttendedIngest) {
		t.Fatalf("err = %v, want ErrAttendedIngest", err)
	}
}
