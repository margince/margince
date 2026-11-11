// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package credentialrepair

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The grant is column-scoped and the trigger checks the columns, so the setting
// buys a rewrite of the images and nothing else.
//
// Two nets rather than one, because the setting is reachable by anything
// running in the transaction.
//
// A bug that turned it on elsewhere must still not be able to rewrite what an
// audit row says happened.
func TestTheAuditExceptionRefusesAChangeToWhatTheRowSaysHappened(t *testing.T) {
	e := integration.Setup(t)
	_, auditRow := seedAMailWithACredentialInItsAuditImage(t, e)

	for _, change := range []struct {
		name, set string
	}{
		{"the action", `action = 'delete'`},
		{"the entity", `entity_type = 'contact'`},
		{"the actor", `actor_id = gen_random_uuid()`},
	} {
		t.Run(change.name, func(t *testing.T) {
			ws := principal.WithWorkspaceID(context.Background(), e.WS)
			err := database.WithWorkspaceTx(ws, e.Pool, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ws,
					`SELECT set_config('margince.audit_credential_mask', 'on', true)`); err != nil {
					return err
				}
				if _, err := tx.Exec(ws, certifyTheRecordOf, auditRow); err != nil {
					return err
				}
				_, err := tx.Exec(ws,
					`UPDATE audit_log SET `+change.set+` WHERE id = $1`, auditRow)
				return err
			})
			if err == nil {
				t.Fatalf("%s was rewritable under the repair's setting", change.name)
			}
			if !strings.Contains(err.Error(), auditTrailRefusal) &&
				!strings.Contains(err.Error(), "permission denied") {
				t.Errorf("the refusal was not the trail's: %v", err)
			}
			// As the owner, where no grant binds and the trigger's column check
			// is the only net left. Without this the check is untested: the app
			// role is refused by the narrow grant whatever the trigger says.
			err = ownerStatementUnderTheRepairSetting(t, `UPDATE audit_log SET `+change.set+` WHERE id = $1`, auditRow)
			if err == nil {
				t.Errorf("%s was rewritable by the owner under the repair's setting", change.name)
			} else if !strings.Contains(err.Error(), auditTrailRefusal) {
				t.Errorf("the owner's refusal was not the trigger's: %v", err)
			}
		})
	}
}

// ownerStatementUnderTheRepairSetting runs one statement on audit_log as the
// owner, in one transaction with the repair's setting on and its certificate
// written, and answers its error.
func ownerStatementUnderTheRepairSetting(t *testing.T, statement string, auditRow ids.UUID) error {
	t.Helper()
	ctx := context.Background()
	tx, err := integration.FreshOwnerConn(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the owner statement: %v", err)
		}
	}()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('margince.audit_credential_mask', 'on', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, certifyTheRecordOf, auditRow); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, statement, auditRow)
	return err
}

// A DELETE is refused with the repair's setting on, by the trigger itself. The
// owner holds every grant, so no privilege stands in front of it.
func TestTheAuditExceptionNeverAdmitsADelete(t *testing.T) {
	e := integration.Setup(t)
	_, auditRow := seedAMailWithACredentialInItsAuditImage(t, e)

	err := ownerStatementUnderTheRepairSetting(t, `DELETE FROM audit_log WHERE id = $1`, auditRow)
	if err == nil || !strings.Contains(err.Error(), auditTrailRefusal) {
		t.Fatalf("a DELETE under the repair's setting answered %v, want the trigger's refusal", err)
	}
}

// The exception admits a masked image and nothing else. It needs the setting,
// the certificate and a record type the rules reach. The image must keep its
// shape, with every changed string holding the marker. Each refusal misses one.
func TestTheAuditExceptionRefusesAnImageThatIsNotMasked(t *testing.T) {
	e := integration.Setup(t)
	_, auditRow := seedAMailWithACredentialInItsAuditImage(t, e)
	shaped := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before)
		VALUES ($1, 'human', 'rep', 'update', 'activity', $2, $3)`,
		shaped, ids.NewV7(), []byte(`{"body":"Passwort: `+seededCredential+`","seats":2}`))
	// A row whose before image is NULL, so a write to it is an image conjured
	// where the trail quoted nothing. AuditEvent writes rows of this shape.
	noBefore := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, after)
		VALUES ($1, 'human', 'rep', 'export', 'activity', $2, $3)`,
		noBefore, ids.NewV7(), []byte(`{"body":"nothing secret"}`))
	outside := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before)
		VALUES ($1, 'human', 'rep', 'update', 'deal', $2, $3)`,
		outside, ids.NewV7(), []byte(`{"body":"Passwort: `+seededCredential+`"}`))

	// A certificate committed by an earlier transaction admits nothing now.
	earlier := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before)
		VALUES ($1, 'human', 'rep', 'update', 'activity', $2, $3)`,
		earlier, ids.NewV7(), []byte(`{"body":"Passwort: `+seededCredential+`"}`))
	e.WsExec(t, certifyTheRecordOf, earlier)

	seedImage := func(mail ids.UUID, before string) ids.UUID {
		row := ids.NewV7()
		e.WsExec(t, `
			INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before)
			VALUES ($1, 'human', 'rep', 'update', 'activity', $2, $3)`, row, mail, []byte(before))
		return row
	}
	twoCredentials := seedImage(ids.NewV7(), `{"body":"Passwort: aaa1 und Kennwort: bbb2 danke"}`)
	repeated := seedImage(ids.NewV7(), `{"body":"ok then ok"}`)
	atStart := seedImage(ids.NewV7(), `{"body":"Hunter2-Example! war das Passwort"}`)
	held := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity
			(id, kind, body, occurred_at, source, captured_by,
			 archived_at, restricted_at, restricted_reason, restricted_until,
			 retention_class, retention_class_at)
		VALUES ($1, 'email', $2, now(), 'gmail:held-image', 'connector:gmail',
			now(), now(), 'a tax audit of the quarter', now() + interval '2 years',
			'commercial_correspondence', now())`, held, "Passwort: "+seededCredential)
	heldImage := seedAnAuditImageOfAnEditTo(t, e, held)
	// Another record type numbering a row with the held mail's id is not held.
	sameIDSend := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before)
		VALUES ($1, 'human', 'rep', 'update', 'scheduled_send', $2, $3)`,
		sameIDSend, held, []byte(`{"body":"Passwort: `+seededCredential+`"}`))

	const masked = `{"body":"Passwort: [masked]"}`
	cases := []struct {
		name, image, setting, certificate string
		onRow                             ids.UUID
		refused                           bool
	}{
		{"an arbitrary replacement", `{"body":"a sentence nobody wrote"}`, "local", certifyTheRecordOf, auditRow, true},
		{"a changed key structure", `{"note":"Passwort: [masked]"}`, "local", certifyTheRecordOf, auditRow, true},
		{"an added key", `{"body":"Passwort: [masked]","to":"x"}`, "local", certifyTheRecordOf, auditRow, true},
		{"a string turned into an object", `{"body":{"text":"[masked]"}}`, "local", certifyTheRecordOf, auditRow, true},
		{"a non-string leaf change", `{"body":"Passwort: [masked]","seats":3}`, "local", certifyTheRecordOf, shaped, true},
		{"an image where the row held none", `{"body":"[masked]"}`, "local", certifyTheRecordOf, noBefore, true},
		{"a record type outside the rules", masked, "local", certifyTheRecordOf, outside, true},
		{"the setting without the certificate", masked, "local", "", auditRow, true},
		{"a session-level setting without the certificate", masked, "session", "", auditRow, true},
		{"the certificate without the setting", masked, "", certifyTheRecordOf, auditRow, true},
		{"a certificate from an earlier transaction", masked, "local", "", earlier, true},
		{"text outside the marker changed", `{"body":"Kennwort: [masked]"}`, "local", certifyTheRecordOf, auditRow, true},
		{"the marker appended to unrelated text", `{"body":"unrelated [masked]"}`, "local", certifyTheRecordOf, auditRow, true},
		{"text inserted before the marker", `{"body":"Passwort: zz[masked]"}`, "local", certifyTheRecordOf, auditRow, true},
		{"a leaf shortened with no marker", `{"body":"ok"}`, "local", certifyTheRecordOf, repeated, true},
		{"text appended after the marker", `{"body":"Passwort: [masked]!!"}`, "local", certifyTheRecordOf, auditRow, true},
		{
			"old text repeated after the marker", `{"body":"Passwort: Hunter2-Example![masked]Example!"}`,
			"local", certifyTheRecordOf, auditRow, true,
		},
		{
			"text between two markers changed", `{"body":"Passwort: [masked] oder Kennwort: [masked] danke"}`,
			"local", certifyTheRecordOf, twoCredentials, true,
		},
		{
			"pieces out of order", `{"body":"[masked]Kennwort: [masked]Passwort: [masked] danke"}`,
			"local", certifyTheRecordOf, twoCredentials, true,
		},
		{"a mail under a statutory hold", masked, "local", certifyTheRecordOf, heldImage, true},
		{"a masked image", masked, "local", certifyTheRecordOf, auditRow, false},
		{"another record type sharing a held mail's id", masked, "local", certifyTheRecordOf, sameIDSend, false},
		{
			"two credentials masked in one leaf", `{"body":"Passwort: [masked] und Kennwort: [masked] danke"}`,
			"local", certifyTheRecordOf, twoCredentials, false,
		},
		{
			"a credential at the start of the leaf", `{"body":"[masked] war das Passwort"}`,
			"local", certifyTheRecordOf, atStart, false,
		},
		{
			"a credential at the end of the leaf", `{"body":"Passwort: [masked]"}`,
			"local", certifyTheRecordOf, auditRow, false,
		},
	}
	for _, variant := range certificateVariants {
		if variant.statement == certifyTheRecordOf {
			t.Fatalf("the certificate %s is the valid one: its replacement found nothing", variant.name)
		}
		cases = append(cases, struct {
			name, image, setting, certificate string
			onRow                             ids.UUID
			refused                           bool
		}{"a certificate " + variant.name, masked, "local", variant.statement, auditRow, true})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ws := principal.WithWorkspaceID(context.Background(), e.WS)
			err := database.WithWorkspaceTx(ws, e.Pool, func(tx pgx.Tx) error {
				switch c.setting {
				case "local":
					if _, err := tx.Exec(ws, `SELECT set_config('margince.audit_credential_mask', 'on', true)`); err != nil {
						return err
					}
				case "session":
					if _, err := tx.Exec(ws, `SET margince.audit_credential_mask = 'on'`); err != nil {
						return err
					}
				}
				if c.certificate != "" {
					if _, err := tx.Exec(ws, c.certificate, c.onRow); err != nil {
						return err
					}
				}
				_, err := tx.Exec(ws, `UPDATE audit_log SET before = $2::jsonb WHERE id = $1`, c.onRow, c.image)
				if err == nil && !c.refused {
					return errAcceptedOnPurpose
				}
				return err
			})
			if c.refused && (err == nil || !strings.Contains(err.Error(), auditTrailRefusal)) {
				t.Errorf("%s answered %v, want the trigger's refusal", c.name, err)
			}
			if !c.refused && !errors.Is(err, errAcceptedOnPurpose) {
				t.Errorf("%s was refused: %v", c.name, err)
			}
		})
	}
}

// The trigger's record-type list mirrors storekit's masked audit types. A type
// only the rules know keeps its credential in the trail. A type only the
// trigger knows admits rewrites of a trail the rules never mask.
func TestTheTriggerAdmitsExactlyTheRecordTypesTheRulesMask(t *testing.T) {
	e := integration.Setup(t)
	definition := e.WsScalar(t, `SELECT pg_get_functiondef('audit_log_immutable'::regproc)`)
	list := regexp.MustCompile(`OLD\.entity_type IN \(([^)]*)\)`).FindStringSubmatch(definition)
	if list == nil {
		t.Fatalf("audit_log_immutable() holds no record-type list:\n%s", definition)
	}
	inTrigger := map[string]bool{}
	for _, quoted := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(list[1], -1) {
		inTrigger[quoted[1]] = true
	}
	inRules := map[string]bool{}
	for _, entityType := range storekit.MaskedAuditEntityTypes() {
		inRules[entityType] = true
		if !inTrigger[entityType] {
			t.Errorf("%q is masked by storekit but missing from the trigger's list", entityType)
		}
	}
	for entityType := range inTrigger {
		if !inRules[entityType] {
			t.Errorf("%q is in the trigger's list but storekit does not mask it", entityType)
		}
	}
}
