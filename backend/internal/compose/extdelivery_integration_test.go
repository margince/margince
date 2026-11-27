// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit reacting to a new company as its service member. It reads the company
// back, sets custom values, finds the admins and files a task for one.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// deliverCompanyCreated runs fn as the unit's handler for one company.created
// delivery, through the consumer the worker runs, and answers what it returned.
func (e *crmEnv) deliverCompanyCreated(t *testing.T, event ids.UUID, company string, fn func(context.Context, extension.Runtime) error) error {
	t.Helper()
	sub := ComposedSubscription{Unit: ingressUnit, Version: "1.0.0", Sub: extension.Subscription{
		Name: "on_company", Events: []string{"company.created"},
		Handle: func(ctx context.Context, rt extension.Runtime, _ extension.Delivery) error { return fn(ctx, rt) },
	}}
	return sub.Handler(e.Pool, quietTestLogger())(context.Background(), kevents.Envelope{
		EventID: event, Type: "company.created",
		Entity: kevents.EntityRef{Type: "company", ID: ids.MustParse(company)},
		Trace:  kevents.Trace{CorrelationID: ids.NewV7()},
	})
}

// withServiceMember gives the unit the service member its jobs land records as.
func (e *crmEnv) withServiceMember(t *testing.T) ids.UUID {
	t.Helper()
	member := declareServiceMemberFor(t, ingressUnit)
	depositCredential(t, e.extRuntimeEnv, member)
	return member
}

func (e *crmEnv) companyField(t *testing.T, label, kind string, options ...string) string {
	t.Helper()
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	field, err := catalog.Create(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms), customfields.FieldSpec{
		Object: "company", Label: label, Type: kind, Options: options, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the %s field: %v", label, err)
	}
	return *field.ColumnName
}

func TestADeliveryReadsTheCompanyAsTheServiceMember(t *testing.T) {
	e := setupCRMIngress(t)
	member := e.withServiceMember(t)
	made := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))

	err := e.deliverCompanyCreated(t, ids.NewV7(), made.Ref.ID, func(ctx context.Context, rt extension.Runtime) error {
		if got := rt.Caller(); got.UserID != member.String() {
			t.Errorf("caller = %+v, want the service member %s", got, member)
		}
		return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
			got, err := tx.Core().Companies().Get(ctx, made.Ref.ID)
			if err != nil || got.DisplayName != "Acme" {
				t.Errorf("Get = %+v, %v, want the company read back", got, err)
			}
			_, err = tx.Core().Companies().Get(ctx, ids.NewV7().String())
			if !errors.Is(err, extension.ErrNotFound) {
				t.Errorf("Get of a record the member cannot see = %v, want ErrNotFound", err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestADeliveryOfAUnitWithNoServiceMemberRefusesTheCore(t *testing.T) {
	e := setupCRMIngress(t)
	made := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))

	err := e.deliverCompanyCreated(t, ids.NewV7(), made.Ref.ID, func(ctx context.Context, rt extension.Runtime) error {
		if rt.Caller() != (extension.Caller{}) {
			t.Errorf("caller = %+v, want nobody", rt.Caller())
		}
		return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
			if _, err := tx.Core().Companies().Get(ctx, made.Ref.ID); !errors.Is(err, extension.ErrForbidden) {
				t.Errorf("Get = %v, want ErrForbidden", err)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestADeliverySetsACustomFieldAndKeepsOneAUserEdited(t *testing.T) {
	e := setupCRMIngress(t)
	e.withServiceMember(t)
	column := e.companyField(t, "Segment", customfields.TypeText)
	made := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))
	company := ids.From[ids.CompanyKind](ids.MustParse(made.Ref.ID))
	store := newContactsStore(InstallationDB(e.Pool)).WithFieldCatalog(customfields.NewService(e.Pool, integration.SchemaPool(t)))

	set := func(value string) []string {
		var withheld []string
		err := e.deliverCompanyCreated(t, ids.NewV7(), made.Ref.ID, func(ctx context.Context, rt extension.Runtime) error {
			return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) (err error) {
				withheld, err = tx.Core().Companies().SetCustomFields(ctx, made.Ref.ID, map[string]any{column: value})
				return err
			})
		})
		if err != nil {
			t.Fatalf("setting %q: %v", value, err)
		}
		return withheld
	}
	read := func() any {
		got, err := store.GetCompany(e.Admin(), company, storekit.LiveOnly)
		if err != nil {
			t.Fatalf("read the company: %v", err)
		}
		return got.AdditionalProperties[column]
	}

	if withheld := set("mid-market"); len(withheld) != 0 || read() != "mid-market" {
		t.Fatalf("first delivery: withheld %v, value %v, want the value written", withheld, read())
	}
	if withheld := set("mid-market"); len(withheld) != 0 || read() != "mid-market" {
		t.Errorf("second delivery: withheld %v, value %v, want the same value and nothing withheld", withheld, read())
	}
	if withheld := set("smb"); len(withheld) != 0 || read() != "smb" {
		t.Errorf("a changed value: withheld %v, value %v, want the unit's earlier write replaced", withheld, read())
	}
	if _, err := store.UpdateCompany(e.Admin(), company,
		contacts.UpdateCompanyInput{CustomFields: map[string]any{column: "typed by a user"}}); err != nil {
		t.Fatalf("editing as a user: %v", err)
	}
	if withheld := set("enterprise"); !slices.Equal(withheld, []string{column}) || read() != "typed by a user" {
		t.Errorf("third delivery: withheld %v, value %v, want the user's value kept and named", withheld, read())
	}
}

func TestADeliveryCannotSetAnUnknownKeyOrAnOptionTheListLacks(t *testing.T) {
	e := setupCRMIngress(t)
	e.withServiceMember(t)
	tier := e.companyField(t, "Tier", customfields.TypePicklist, "gold", "silver")
	made := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))

	for name, values := range map[string]map[string]any{
		"an unknown key":         {"cf_missing": "x"},
		"a value not an option":  {tier: "bronze"},
		"a key that is no field": {"name": "x"},
	} {
		err := e.deliverCompanyCreated(t, ids.NewV7(), made.Ref.ID, func(ctx context.Context, rt extension.Runtime) error {
			return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
				_, err := tx.Core().Companies().SetCustomFields(ctx, made.Ref.ID, values)
				return err
			})
		})
		if !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

// aMemberHolding seats a member who holds the role and answers its id.
func aMemberHolding(t *testing.T, role, name string, agent bool) ids.UUID {
	t.Helper()
	owner := integration.FreshOwnerConn(t)
	member := ids.NewV7()
	ctx := context.Background()
	if _, err := owner.Exec(ctx, `INSERT INTO app_user (id, email, display_name, is_agent) VALUES ($1, $2, $3, $4)`,
		member, member.String()+"@members.invalid", name, agent); err != nil {
		t.Fatalf("inserting %s: %v", name, err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $2 FROM role WHERE key = $1`, role, member); err != nil {
		t.Fatalf("assigning %s the %s role: %v", name, role, err)
	}
	return member
}

func TestADeliveryListsTheActiveAdminsAndFilesOneTaskForTheFirst(t *testing.T) {
	e := setupCRMIngress(t)
	e.withServiceMember(t)
	composeUnit(t, extension.Extension{
		Name: extension.Name(ingressUnit), Version: "1.0.0",
		Ingress: []extension.IngressSource{{System: crmSystem, Lands: []extension.RecordKind{extension.KindActivity, extension.KindCompany}}},
		Secrets: []extension.SecretsRequest{{Key: ingressTestSecretKey, Scope: extension.SecretScopeUser}},
		Needs:   []extension.Need{{Capability: extension.CapabilityMembers, Reason: "Finds an administrator to hand a task to."}},
	})
	first := aMemberHolding(t, "admin", "Ada Admin", false)
	aMemberHolding(t, "admin", "Agent Seat", true)
	made := e.mustIngest(t, companyRecord("acct-1", extension.CompanyFields{DisplayName: "Acme"}))

	fileTask := func(event ids.UUID) {
		err := e.deliverCompanyCreated(t, event, made.Ref.ID, func(ctx context.Context, rt extension.Runtime) error {
			var admins []extension.Member
			if err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) (err error) {
				admins, err = tx.Core().Members().List(ctx, extension.MemberQuery{Role: "admin"})
				return err
			}); err != nil {
				return err
			}
			humans := slices.DeleteFunc(admins, func(m extension.Member) bool { return m.IsAgent })
			if len(humans) == 0 || humans[0].ID != first.String() {
				t.Errorf("admins = %+v, want the active human admin %s", admins, first)
				return nil
			}
			_, err := rt.Ingest(ctx, extension.UserID(e.member.String()), taskRecord("welcome-"+made.Ref.ID, first))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	fileTask(ids.NewV7())
	fileTask(ids.NewV7())
	if n := e.taskRows(t, "welcome-"+made.Ref.ID, first); n != 1 {
		t.Errorf("tasks for the first admin = %d after a redelivery, want one", n)
	}
}
