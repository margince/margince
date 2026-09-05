// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The installation-settings surface (ADR-0090/A135): the module-facing shape
// over the settings identity owns. RBAC, validation, the freeze probe
// and the audit-only write all live on the entries — this file is only the
// read/patch shape a transport needs.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// InstallationSettings is the installation's identity and reporting basis.
type InstallationSettings struct {
	Name         string
	Timezone     string
	BaseCurrency string
	BaseLanguage string
	// FiscalYearStartMonth is the month the business year begins, 1..12.
	FiscalYearStartMonth int
	// BaseCurrencyLocked and its reason let a client render the field
	// read-only instead of discovering the refusal by attempting a write —
	// the same information the write path would give, offered before the
	// operator types a currency they cannot save.
	BaseCurrencyLocked       bool
	BaseCurrencyLockedReason string
	// EnabledOidcProviders is the admin's chosen provider list, or nil when they
	// have never chosen — which means every provider the deployment configured.
	// It is the STORED answer and not the effective one: the deployment decides
	// what is possible, and compose intersects the two.
	EnabledOidcProviders []string
}

// InstallationPatch is a sparse installation-settings write: a nil field is
// left unchanged. Named fields rather than positional pointers, because most of
// them are *string and a transposed pair would write a language into the
// currency row and pass the type checker.
type InstallationPatch struct {
	Name                 *string
	Timezone             *string
	BaseCurrency         *string
	BaseLanguage         *string
	FiscalYearStartMonth *int
	// EnabledOidcProviders replaces the whole list. A nil pointer leaves it
	// unchanged; a pointer to an empty slice is a real choice — offer password
	// only — so the two cannot be collapsed.
	EnabledOidcProviders *[]string
}

// pendingWrite is one field of a sparse patch, already reduced to the two
// things the write loop needs: which setting to write under, and the JSON to
// write. `raw` nil means the field was absent and nothing is written.
type pendingWrite struct {
	entry settings.Definition
	raw   []byte
}

// encodePatchField reduces one patch field to a pendingWrite, encoding it here
// where its type is still known rather than handing the loop an `any`.
//
// Generic over the field's own type, which is what keeps the write typed: the
// installation's fields are strings except the fiscal start month, and a loop
// carrying `any` would accept a value of any type at all — including, silently,
// a nil typed pointer, which is NOT equal to nil once it is inside an
// interface.
func encodePatchField[T any](entry settings.Definition, value *T) (pendingWrite, error) {
	if value == nil {
		return pendingWrite{entry: entry}, nil
	}
	raw, err := json.Marshal(*value)
	if err != nil {
		return pendingWrite{}, fmt.Errorf("identity: encoding %s: %w", entry.Key(), err)
	}
	return pendingWrite{entry: entry, raw: raw}, nil
}

// InstallationSettingsStore reads and patches the installation settings.
type InstallationSettingsStore struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3); it is
	// held for the lock probe, which asks a question no setting read can
	// answer: whether the currency has become immutable.
	db       *database.DB
	settings *settings.Store
}

// NewInstallationSettings builds the store over the settings mechanism, on a
// handle already bound to the workspace it serves.
func NewInstallationSettings(db *database.DB, s *settings.Store) *InstallationSettingsStore {
	return &InstallationSettingsStore{db: db, settings: s}
}

// GetInstallation reads the settings and the base currency's lock state.
//
// Named GetInstallation rather than Get deliberately. rbacgate_test.go resolves
// gatedness by BARE FUNCTION NAME within a package — optimistic by design, so
// it never cries wolf on dispatch it cannot resolve — which means a gated
// method here called `Get` would merge with identity's existing ungated `Get`
// (the self-scoped onboarding wizard state) and vouch for it, and for `Login`,
// whose waivers say plainly that neither has a principal to gate yet. A
// distinct name keeps those two honestly reported as ungated.
func (s *InstallationSettingsStore) GetInstallation(ctx context.Context) (InstallationSettings, error) {
	name, err := settings.Get(ctx, s.settings, Name)
	if err != nil {
		return InstallationSettings{}, err
	}
	zone, err := settings.Get(ctx, s.settings, Timezone)
	if err != nil {
		return InstallationSettings{}, err
	}
	currency, err := settings.Get(ctx, s.settings, BaseCurrency)
	if err != nil {
		return InstallationSettings{}, err
	}
	language, err := settings.Get(ctx, s.settings, BaseLanguage)
	if err != nil {
		return InstallationSettings{}, err
	}
	fiscalStart, err := settings.Get(ctx, s.settings, FiscalYearStartMonth)
	if err != nil {
		return InstallationSettings{}, err
	}
	providers, err := settings.Get(ctx, s.settings, EnabledOidcProviders)
	if err != nil {
		return InstallationSettings{}, err
	}
	locked, why, err := s.baseCurrencyLock(ctx)
	if err != nil {
		return InstallationSettings{}, err
	}
	return InstallationSettings{
		Name: name, Timezone: zone, BaseCurrency: currency, BaseLanguage: language,
		FiscalYearStartMonth: fiscalStart,
		BaseCurrencyLocked:   locked, BaseCurrencyLockedReason: why,
		EnabledOidcProviders: providers,
	}, nil
}

// signInPolicyReadActor names the entry read this projection performs after it
// has already admitted the caller. A SYSTEM actor for the same reason the login
// screen's read uses one: the question is what this INSTALLATION offers, not
// what this reader may see, and the reader's own authority was settled one line
// above.
const signInPolicyReadActor = "system:sign_in_policy_read"

// SignInPolicy answers which sign-in providers the installation offers, gated on
// `authentication_policy` rather than on the settings aggregate around it.
//
// THE GATE HERE IS THE WHOLE SECURITY OF THIS READ. The entry itself is defined
// on installation_settings — moving it would make every read of the aggregate
// demand this grant and take the name, timezone and currency with it, which
// every role is meant to read — so this checks the caller first and then reads
// the entry as the installation. A system principal bypasses object RBAC
// entirely, so removing or weakening the Require below does not merely widen
// this endpoint, it removes its only gate.
func (s *InstallationSettingsStore) SignInPolicy(ctx context.Context) ([]string, error) {
	if err := auth.Require(ctx, authenticationPolicyObject, principal.ActionRead); err != nil {
		return nil, err
	}
	// Only after the caller is admitted. The workspace and correlation id ride
	// from the request so the read stays attributable to the trace that asked.
	readCtx := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   signInPolicyReadActor,
	})
	chosen, err := settings.Get(readCtx, s.settings, EnabledOidcProviders)
	if err != nil {
		return nil, fmt.Errorf("identity: reading the sign-in policy: %w", err)
	}
	return chosen, nil
}

// baseCurrencyLock asks the entry's own probe, so the answer the read reports
// and the answer the write enforces come from one place. A read with the probe
// unwired reports "changeable", which is what the write would then do — the
// two agree even when the wiring is wrong, and the fitness gate catches the
// wiring.
func (s *InstallationSettingsStore) baseCurrencyLock(ctx context.Context) (bool, string, error) {
	var locked bool
	var why string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var probeErr error
		locked, why, probeErr = BaseCurrency.Frozen(ctx, tx)
		return probeErr
	})
	if err != nil {
		return false, "", fmt.Errorf("identity: reading base-currency lock state: %w", err)
	}
	return locked, why, nil
}

// encodeInstallationPatch reduces every field of a sparse patch to its wire
// bytes. Every field appears exactly once, and a field left out here is a value
// that silently stops saving — the form still shows it, the PATCH still carries
// it, the write still returns 200, and the row never moves.
//
// Held by TestEveryInstallationPatchFieldIsEncoded, which derives the expected
// list from InstallationPatch by reflection rather than restating it: a sixth
// setting added to that struct fails until it is encoded here. The claim was a
// comment first, and dropping a line from the slice below left the whole tree
// green.
//
// The error returns are not dead despite json.Marshal never failing on the
// string and int fields that exist today: encodePatchField is generic, and the
// first field whose type has a MarshalJSON that can fail arrives without
// touching this function.
func encodeInstallationPatch(in InstallationPatch) ([]pendingWrite, error) {
	name, err := encodePatchField(Name, in.Name)
	if err != nil {
		return nil, err
	}
	zone, err := encodePatchField(Timezone, in.Timezone)
	if err != nil {
		return nil, err
	}
	currency, err := encodePatchField(BaseCurrency, in.BaseCurrency)
	if err != nil {
		return nil, err
	}
	language, err := encodePatchField(BaseLanguage, in.BaseLanguage)
	if err != nil {
		return nil, err
	}
	fiscal, err := encodePatchField(FiscalYearStartMonth, in.FiscalYearStartMonth)
	if err != nil {
		return nil, err
	}
	providers, err := encodePatchField(EnabledOidcProviders, in.EnabledOidcProviders)
	if err != nil {
		return nil, err
	}
	return []pendingWrite{name, zone, currency, language, fiscal, providers}, nil
}

// UpdateInstallation applies a sparse patch. Named for the same reason as
// GetInstallation above. A nil field is left unchanged; an unchanged value
// writes nothing and audits nothing. Returns the settings after the write.
//
// The update gate is taken here, before any branch, so an empty patch is
// refused for a caller who may not write rather than answered from the read
// gate alone.
//
// Every field commits in ONE transaction, and the settings rows are the only
// copy: a change here moves the value everything computes in, because there is
// no second place for it to disagree.
func (s *InstallationSettingsStore) UpdateInstallation(ctx context.Context, in InstallationPatch) (InstallationSettings, error) {
	if err := auth.Require(ctx, installationSettingsObject, principal.ActionUpdate); err != nil {
		return InstallationSettings{}, err
	}
	// Each field is encoded where its own type is still known, so the loop
	// below carries bytes rather than an `any` that would accept anything.
	// Every field of the patch appears exactly once: one left out is a value
	// that silently stops saving.
	patch, err := encodeInstallationPatch(in)
	if err != nil {
		return InstallationSettings{}, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, w := range patch {
			if w.raw == nil {
				continue
			}
			if err := s.settings.SetRawTx(ctx, tx, w.entry.Key(), w.raw); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return InstallationSettings{}, err
	}
	return s.GetInstallation(ctx)
}
