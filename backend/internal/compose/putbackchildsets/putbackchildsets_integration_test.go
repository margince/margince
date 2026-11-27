// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package putbackchildsets

// A writer other than the edit form can change a contact's emails or phones or
// a company's domains after an edit of them. Put back of that edit must not be
// offered, and the restore must refuse, because replaying it would drop the
// rows the writer added. Each case runs the real writer and the real Put back.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func seamFor(e *integration.Env) compose.RestoreSeam {
	return compose.NewRestoreSeam(e.Pool, compose.NewProvider(e.Pool), deals.NewStore(e.DB(), compose.DealsInstallation()))
}

func latestUpdate(t *testing.T, e *integration.Env, entityType string, id ids.UUID) (entry ids.UUID, version int64) {
	t.Helper()
	admin := e.Admin()
	err := database.WithTx(admin, e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(admin, `SELECT id FROM audit_log WHERE entity_type = $1 AND entity_id = $2 AND action = 'update'
			ORDER BY occurred_at DESC, id DESC LIMIT 1`, entityType, id).Scan(&entry); err != nil {
			return err
		}
		return tx.QueryRow(admin, `SELECT version FROM `+pgx.Identifier{entityType}.Sanitize()+` WHERE id = $1`, id).Scan(&version)
	})
	if err != nil {
		t.Fatalf("read the latest %s update entry: %v", entityType, err)
	}
	return entry, version
}

func offered(t *testing.T, e *integration.Env, entityType string, id, entry ids.UUID) bool {
	t.Helper()
	answers, err := compose.NewUndoabilityPage(seamFor(e)).ForRecord(e.Admin(), entityType, id, []ids.UUID{entry})
	if err != nil {
		t.Fatalf("judging the entry: %v", err)
	}
	return answers[entry].Undoable
}

func versionOf(t *testing.T, e *integration.Env, entityType string, id ids.UUID) int64 {
	t.Helper()
	_, version := latestUpdate(t, e, entityType, id)
	return version
}

func contactEmail(tag, local string) []contacts.ContactEmailInput {
	return []contacts.ContactEmailInput{{Email: local + "-" + tag + "@example.test", EmailType: "work", IsPrimary: true}}
}

func contactPhone(number string) []contacts.ContactPhoneInput {
	return []contacts.ContactPhoneInput{{Phone: number, PhoneType: "work", IsPrimary: true}}
}

func setEmails(t *testing.T, e *integration.Env, id ids.UUID, in []contacts.ContactEmailInput) {
	t.Helper()
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](id), contacts.UpdateContactInput{Emails: in}); err != nil {
		t.Fatalf("set emails: %v", err)
	}
}

func setPhones(t *testing.T, e *integration.Env, id ids.UUID, in []contacts.ContactPhoneInput) {
	t.Helper()
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](id), contacts.UpdateContactInput{Phones: in}); err != nil {
		t.Fatalf("set phones: %v", err)
	}
}

func setDomains(t *testing.T, e *integration.Env, id ids.UUID, names ...string) {
	t.Helper()
	in := make([]contacts.CompanyDomainInput, len(names))
	for i, name := range names {
		in[i] = contacts.CompanyDomainInput{Domain: name, IsPrimary: i == 0}
	}
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](id), contacts.UpdateCompanyInput{Domains: &in}); err != nil {
		t.Fatalf("set domains %v: %v", names, err)
	}
}

// seedMail makes a contact that sent one mail, as capture leaves it, and answers
// the contact and the mail.
func seedMail(t *testing.T, e *integration.Env, address, body string) (contact, activity ids.UUID) {
	t.Helper()
	contact, activity = ids.NewV7(), ids.NewV7()
	err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Mail Contact', 'gmail:seed', 'connector:gmail')`, []any{contact}},
			{`INSERT INTO contact_email (contact_id, email, email_type, is_primary, source, captured_by) VALUES ($1, $2, 'work', true, 'gmail:seed', 'connector:gmail')`, []any{contact, address}},
			{`INSERT INTO activity (id, kind, subject, body, direction, occurred_at, source_system, source_id, source, captured_by)
			  VALUES ($1, 'email', 'hello', $2, 'inbound', now() + interval '1 day', 'gmail', $3, 'gmail:seed', 'connector:gmail')`, []any{activity, body, activity.String()}},
			{`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, []any{activity, contact}},
			{`INSERT INTO activity_participant (activity_id, contact_id, address, role) VALUES ($1, $2, $3, 'from')`, []any{activity, contact, address}},
		} {
			if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed the mail: %v", err)
	}
	return contact, activity
}

type scenario struct {
	name   string
	entity string
	// seed makes the record and the two edits, the second of which is judged.
	seed func(t *testing.T, e *integration.Env) ids.UUID
	// write is the writer under test.
	write func(t *testing.T, e *integration.Env, id ids.UUID)
}

func scenarios() []scenario {
	return []scenario{
		{"phone merge", "contact", func(t *testing.T, e *integration.Env) ids.UUID {
			id := e.SeedContact(t, "Phone Survivor", nil)
			setPhones(t, e, id, contactPhone("+4930111001"))
			setPhones(t, e, id, contactPhone("+4930111002"))
			return id
		}, func(t *testing.T, e *integration.Env, id ids.UUID) {
			source := e.SeedContact(t, "Phone Retired", nil)
			setPhones(t, e, source, contactPhone("+4930111003"))
			if _, err := e.Contacts.MergeContact(e.Admin(), ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](id), nil); err != nil {
				t.Fatalf("merge: %v", err)
			}
		}},
		{"email merge", "contact", func(t *testing.T, e *integration.Env) ids.UUID {
			id := e.SeedContact(t, "Email Survivor", nil)
			setEmails(t, e, id, contactEmail(id.String()[:8], "a"))
			setEmails(t, e, id, contactEmail(id.String()[:8], "b"))
			return id
		}, func(t *testing.T, e *integration.Env, id ids.UUID) {
			source := e.SeedContact(t, "Email Retired", nil)
			setEmails(t, e, source, contactEmail(id.String()[:8], "carried"))
			if _, err := e.Contacts.MergeContact(e.Admin(), ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](id), nil); err != nil {
				t.Fatalf("merge: %v", err)
			}
		}},
		{"domain merge", "company", func(t *testing.T, e *integration.Env) ids.UUID {
			id := e.SeedCompany(t, "Domain Survivor", nil)
			setDomains(t, e, id, "dm-a-"+id.String()[:8]+".test")
			setDomains(t, e, id, "dm-b-"+id.String()[:8]+".test")
			return id
		}, func(t *testing.T, e *integration.Env, id ids.UUID) {
			source := e.SeedCompany(t, "Domain Retired", nil)
			setDomains(t, e, source, "dm-carried-"+id.String()[:8]+".test")
			if _, err := e.Contacts.MergeCompany(e.Admin(), ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](id), nil); err != nil {
				t.Fatalf("merge: %v", err)
			}
		}},
		{"domain adoption", "company", func(t *testing.T, e *integration.Env) ids.UUID {
			id := e.SeedCompany(t, "Adopting Co", nil)
			setDomains(t, e, id, "ad-a-"+id.String()[:8]+".test")
			setDomains(t, e, id, "ad-b-"+id.String()[:8]+".test")
			return id
		}, func(t *testing.T, e *integration.Env, id ids.UUID) {
			companyID := ids.From[ids.CompanyKind](id)
			domain := "adopted-" + id.String()[:8] + ".test"
			err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				_, err := tx.Exec(context.Background(), `INSERT INTO company_domain_disposition (domain, status, last_evidence_at)
					VALUES ($1, 'pending', now())`, domain)
				return err
			})
			if err != nil {
				t.Fatalf("open the question: %v", err)
			}
			if _, err := e.Contacts.ResolveDomainTriage(e.Admin(), contacts.ResolveDomainTriageInput{
				Domain: domain, Status: contacts.DomainCompany,
				Source: contacts.DomainSourceHuman, CompanyID: &companyID,
			}); err != nil {
				t.Fatalf("adopt the domain: %v", err)
			}
		}},
		{"website save", "company", func(t *testing.T, e *integration.Env) ids.UUID {
			website := "https://home-a-" + ids.NewV7().String()[:8] + ".test"
			saved, err := e.Contacts.SaveCompany(adminWithSettings(e), contacts.SaveCompanyInput{DisplayName: "Own House", Website: &website})
			if err != nil {
				t.Fatalf("save the company: %v", err)
			}
			id := saved.CompanyID.UUID
			setDomains(t, e, id, "own-a-"+id.String()[:8]+".test")
			setDomains(t, e, id, "own-b-"+id.String()[:8]+".test")
			return id
		}, func(t *testing.T, e *integration.Env, id ids.UUID) {
			website := "https://home-b-" + id.String()[:8] + ".test"
			if _, err := e.Contacts.SaveCompany(adminWithSettings(e), contacts.SaveCompanyInput{DisplayName: "Own House", Website: &website}); err != nil {
				t.Fatalf("save the website: %v", err)
			}
		}},
		{"provider email fill", "contact", func(t *testing.T, e *integration.Env) ids.UUID {
			id := e.SeedContact(t, "Bought Email", nil)
			setEmails(t, e, id, contactEmail(id.String()[:8], "a"))
			setEmails(t, e, id, []contacts.ContactEmailInput{})
			return id
		}, func(t *testing.T, e *integration.Env, id ids.UUID) {
			claim := provider.Claim{Key: provider.ClaimProfessionalEmails, Value: []byte(`[{"value":"bought-` + id.String()[:8] + `@example.test"}]`)}
			run := ids.NewV7()
			err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				if _, err := tx.Exec(context.Background(), `INSERT INTO provider_run (id, subject_kind, contact_id, provider, trigger, state,
					input_fingerprint, external_correlation_id, connection_version, connection_epoch, configuration_snapshot, requested_categories)
					VALUES ($1, 'contact', $2, 'surfe', 'manual', 'completed', 'fp', $3, 1, 1, '{}', '{}')`, run, id, ids.NewV7()); err != nil {
					return err
				}
				return contacts.ApplyProviderClaims(e.Admin(), tx, run.String(), id.String(), "surfe", []provider.Claim{claim})
			})
			if err != nil {
				t.Fatalf("apply the purchase: %v", err)
			}
		}},
		{"signature number", "contact", func(t *testing.T, e *integration.Env) ids.UUID {
			id, _ := seedMail(t, e, "sig-"+ids.NewV7().String()[:8]+"@example.test", "Best,\nMail Contact\nCTO\n+49 30 7654321")
			setPhones(t, e, id, contactPhone("+4930111001"))
			setPhones(t, e, id, contactPhone("+4930111002"))
			return id
		}, func(t *testing.T, e *integration.Env, id ids.UUID) {
			var activity ids.UUID
			err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), `SELECT activity_id FROM activity_link WHERE contact_id = $1`, id).Scan(&activity)
			})
			if err != nil {
				t.Fatalf("find the mail: %v", err)
			}
			field := contacts.SignatureField{Name: "phone", Value: "+49 30 7654321", Evidence: "+49 30 7654321", Confidence: 0.9}
			if res, err := e.Contacts.ApplySignatureFields(principal.SystemActing(e.Admin(), "agent:enrich"),
				ids.From[ids.ContactKind](id), activity, []contacts.SignatureField{field}, true); err != nil {
				t.Fatalf("read the signature: %v", err)
			} else if res.Applied == 0 {
				t.Fatalf("the signature number did not land: %+v", res)
			}
		}},
	}
}

func adminWithSettings(e *integration.Env) context.Context {
	actor, _ := principal.Actor(e.Admin())
	actor.Permissions.Objects["capture_settings"] = principal.ObjectGrant{Read: true, Update: true}
	return principal.WithActor(e.Admin(), actor)
}

func TestAWriterOtherThanTheEditFormSupersedesAnEarlierSetEdit(t *testing.T) {
	for _, tc := range scenarios() {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			id := tc.seed(t, e)
			entry, before := latestUpdate(t, e, tc.entity, id)
			if !offered(t, e, tc.entity, id, entry) {
				t.Fatal("the edit is not offered before the writer ran, so the case proves nothing")
			}
			tc.write(t, e, id)
			after := versionOf(t, e, tc.entity, id)
			if after <= before {
				t.Errorf("the writer left the version at %d (was %d), so a restore judged before it still matches", after, before)
			}
			if offered(t, e, tc.entity, id, entry) {
				t.Error("the listing offers Put back over rows a writer changed since")
			}
			if _, err := seamFor(e).Restore(e.Admin(), tc.entity, id, entry, before); err == nil {
				t.Error("a restore judged before the writer ran replaced the set")
			}
			_, err := seamFor(e).Restore(e.Admin(), tc.entity, id, entry, after)
			var refusal compose.RefusedRestore
			if !errors.As(err, &refusal) || refusal.Reason != compose.ReasonSuperseded {
				t.Fatalf("putting the earlier edit back answered %v, want superseded", err)
			}
		})
	}
}

// Undoing the newest edit leaves the rows equal to the older edit's result, so
// the older edit can be undone in turn.
func TestUndoingSetEditsOneAfterAnotherWorks(t *testing.T) {
	e := integration.Setup(t)
	cases := []struct {
		name   string
		entity string
		seed   func() ids.UUID
	}{
		{"emails", "contact", func() ids.UUID {
			id := e.SeedContact(t, "Walk Emails", nil)
			setEmails(t, e, id, contactEmail(id.String()[:8], "a"))
			setEmails(t, e, id, contactEmail(id.String()[:8], "b"))
			return id
		}},
		{"phones", "contact", func() ids.UUID {
			id := e.SeedContact(t, "Walk Phones", nil)
			setPhones(t, e, id, contactPhone("+4930222001"))
			setPhones(t, e, id, contactPhone("+4930222002"))
			return id
		}},
		{"domains", "company", func() ids.UUID {
			id := e.SeedCompany(t, "Walk Domains", nil)
			setDomains(t, e, id, "walk-a-"+id.String()[:8]+".test")
			setDomains(t, e, id, "walk-b-"+id.String()[:8]+".test")
			return id
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := tc.seed()
			older := previousUpdate(t, e, tc.entity, id)
			newest, version := latestUpdate(t, e, tc.entity, id)
			if _, err := seamFor(e).Restore(e.Admin(), tc.entity, id, newest, version); err != nil {
				t.Fatalf("putting back the newest edit: %v", err)
			}
			if !offered(t, e, tc.entity, id, older) {
				t.Fatal("the older edit is not offered after the newer one was put back")
			}
			if _, err := seamFor(e).Restore(e.Admin(), tc.entity, id, older, versionOf(t, e, tc.entity, id)); err != nil {
				t.Fatalf("putting back the older edit: %v", err)
			}
		})
	}
}

func previousUpdate(t *testing.T, e *integration.Env, entityType string, id ids.UUID) ids.UUID {
	t.Helper()
	admin := e.Admin()
	var entry ids.UUID
	err := database.WithTx(admin, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(admin, `SELECT id FROM audit_log WHERE entity_type = $1 AND entity_id = $2 AND action = 'update'
			ORDER BY occurred_at DESC, id DESC OFFSET 1 LIMIT 1`, entityType, id).Scan(&entry)
	})
	if err != nil {
		t.Fatalf("find the previous update: %v", err)
	}
	return entry
}
