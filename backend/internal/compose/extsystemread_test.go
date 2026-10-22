// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// CanRead takes the contract's field names, and a mask withholds by its own.
// Every field a mask can withhold must be one a unit may ask about, or the
// question that matters most could not be put.
func TestEveryFieldAMaskWithholdsIsOneCanReadTakes(t *testing.T) {
	known := dealFieldNames()
	withheld := fieldmask.Withheld(fieldmask.Deal, fieldmask.Maskable(fieldmask.Deal))
	if len(withheld) == 0 {
		t.Fatal("no deal field is maskable, so this census reads nothing")
	}
	for _, field := range withheld {
		if !slices.Contains(known, field) {
			t.Errorf("a mask withholds %q, and CanRead refuses it as an unknown field", field)
		}
	}
}

// A retained Runtime refuses its system reads like every other capability.
func TestAReleasedRuntimeRefusesEverySystemRead(t *testing.T) {
	rt := deliveryRuntimeFor(context.Background(), "dealmirror", "1.0.0", "subscription/mirror", extensionRuntimeBinding{})
	rt.release()
	ctx := context.Background()
	deal := extension.EntityRef{Type: "deal", ID: "01a15cc1-9f33-7665-9e65-403f04f85f6b"}
	refusals := map[string]error{}
	_, refusals["Deals().Get"] = rt.Deals().Get(ctx, deal.ID)
	_, refusals["Deals().Responsible"] = rt.Deals().Responsible(ctx, deal.ID)
	_, refusals["Authority().CanRead"] = rt.Authority().CanRead(ctx, "", deal, nil)
	_, refusals["Directory().Emails"] = rt.Directory().Emails(ctx, nil)
	for read, err := range refusals {
		if !errors.Is(err, extension.ErrRuntimeExpired) {
			t.Errorf("%s on a released runtime: err = %v, want ErrRuntimeExpired", read, err)
		}
	}
}

// A finance read and a channel send hand their result to the user who asked.
// So only a job tick and a bus delivery answer a system read.
func TestOnlyATickAndADeliveryAnswerASystemRead(t *testing.T) {
	composeReader(t, extension.SystemReadDirectory)
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	for name, mint := range map[string]func(context.Context, string, string, string, extensionRuntimeBinding) *callRuntime{
		"a job tick":         jobRuntimeFor,
		"a bus delivery":     deliveryRuntimeFor,
		"a finance read":     financeRuntimeFor,
		"a channel send":     sendRuntimeFor,
		"an attended call":   runtimeFor,
		"an inbound request": inboundRuntimeFor,
	} {
		rt := mint(ctx, "dealmirror", "1.0.0", "probe", extensionRuntimeBinding{pool: &pgxpool.Pool{}})
		_, err := rt.Directory().Emails(context.Background(), nil)
		rt.release()
		answers := name == "a job tick" || name == "a bus delivery"
		if answers && err != nil {
			t.Errorf("%s: err = %v, want an answer", name, err)
		}
		if !answers && !errors.Is(err, extension.ErrAttendedRead) {
			t.Errorf("%s: err = %v, want ErrAttendedRead", name, err)
		}
	}
}

// On an attended call CanRead answers its human caller about themselves, and
// refuses every other question. The gate is passed when the record type, not
// the caller, is what refuses: a company is no record CanRead answers for.
func TestAnAttendedCanReadAnswersOnlyItsHumanCallerAboutThemselves(t *testing.T) {
	composeReader(t, extension.SystemReadAuthority)
	self, other := ids.NewV7(), ids.NewV7()
	human := principal.Principal{Type: principal.PrincipalHuman, UserID: self}
	agent := principal.Principal{Type: principal.PrincipalAgent, ID: "agent:probe", OnBehalfOf: self}
	connector := principal.Principal{Type: principal.PrincipalConnector, ID: "connector:probe", OnBehalfOf: self}
	company := extension.EntityRef{Type: "company", ID: ids.NewV7().String()}
	for name, c := range map[string]struct {
		actor principal.Principal
		about ids.UUID
		want  error
	}{
		"a human about themselves":     {human, self, extension.ErrInvalid},
		"a human about a colleague":    {human, other, extension.ErrAttendedRead},
		"an agent about its own human": {agent, self, extension.ErrAttendedRead},
		"a connector about its human":  {connector, self, extension.ErrAttendedRead},
		"the same id in upper case":    {human, self, extension.ErrInvalid},
	} {
		ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), c.actor)
		rt := runtimeFor(ctx, "dealmirror", "1.0.0", "tool/probe", extensionRuntimeBinding{pool: &pgxpool.Pool{}})
		about := c.about.String()
		if name == "the same id in upper case" {
			about = strings.ToUpper(about)
		}
		_, err := rt.Authority().CanRead(ctx, extension.UserID(about), company, nil)
		rt.release()
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), human)
	rt := runtimeFor(ctx, "dealmirror", "1.0.0", "tool/probe", extensionRuntimeBinding{pool: &pgxpool.Pool{}})
	defer rt.release()
	// A deal, so the refusal is the user id's and not the record type's.
	deal := extension.EntityRef{Type: "deal", ID: ids.NewV7().String()}
	if _, err := rt.Authority().CanRead(ctx, "not-a-user", deal, nil); !errors.Is(err, extension.ErrInvalid) ||
		!strings.Contains(err.Error(), "user") {
		t.Errorf("a malformed user id: err = %v, want ErrInvalid naming the user", err)
	}
}

// composeReader composes the dealmirror unit declaring reads for this test.
func composeReader(t *testing.T, reads ...extension.SystemRead) {
	t.Helper()
	setComposedExtensions([]extension.Extension{{Name: "dealmirror", Version: "1.0.0", SystemReads: reads}})
	t.Cleanup(func() { setComposedExtensions(nil) })
}

// A unit reads only what its declaration lists, so the manifest an operator
// reads is the whole of what it can read.
func TestASystemReadTheUnitDidNotDeclareIsRefused(t *testing.T) {
	composeReader(t, extension.SystemReadDirectory)
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	rt := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", extensionRuntimeBinding{pool: &pgxpool.Pool{}})
	defer rt.release()
	deal := extension.EntityRef{Type: "deal", ID: "01a15cc1-9f33-7665-9e65-403f04f85f6b"}
	refusals := map[string]error{}
	_, refusals["Deals().Get"] = rt.Deals().Get(ctx, deal.ID)
	_, refusals["Deals().Responsible"] = rt.Deals().Responsible(ctx, deal.ID)
	_, refusals["Authority().CanRead"] = rt.Authority().CanRead(ctx, "", deal, nil)
	for read, err := range refusals {
		if !errors.Is(err, extension.ErrUndeclaredRead) {
			t.Errorf("%s, undeclared: err = %v, want ErrUndeclaredRead", read, err)
		}
	}
	if _, err := rt.Directory().Emails(ctx, nil); err != nil {
		t.Errorf("Directory().Emails, declared: err = %v, want an answer", err)
	}
}

func TestTheBootRefusesASystemReadNoGroupNames(t *testing.T) {
	for name, reads := range map[string][]extension.SystemRead{
		"an unknown read": {"contacts"},
		"a read twice":    {extension.SystemReadDeals, extension.SystemReadDeals},
	} {
		if err := preflightSystemReads(extension.Extension{Name: "dealmirror", SystemReads: reads}); err == nil {
			t.Errorf("%s: the boot accepted %v", name, reads)
		}
	}
	every := []extension.SystemRead{extension.SystemReadDeals, extension.SystemReadAuthority, extension.SystemReadDirectory}
	if err := preflightSystemReads(extension.Extension{Name: "dealmirror", SystemReads: every}); err != nil {
		t.Errorf("the boot refused every published read: %v", err)
	}
}
