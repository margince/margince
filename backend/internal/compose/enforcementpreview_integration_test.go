// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Switching the qualified forecast on is bound to the preview the admin saw.
// These drive the real enforcement handlers over deals the real worker judged.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func (v *viabilityEnv) enforcementHandlers() assurance.Handlers {
	return assurance.NewHandlers(assurance.NewStore(InstallationDB(v.Pool)), AssuranceExceptionsIn,
		func() time.Time { return v.now }).WithSettings(StoredConfig(v.Pool)).WithPreview(EnforcementPreview)
}

func (v *viabilityEnv) enforcement(t *testing.T) crmcontracts.ForecastAssuranceEnforcement {
	t.Helper()
	rec := httptest.NewRecorder()
	v.enforcementHandlers().GetForecastAssuranceEnforcement(rec,
		httptest.NewRequest(http.MethodGet, "/forecast/assurance/enforcement", nil).WithContext(v.Admin()))
	var out crmcontracts.ForecastAssuranceEnforcement
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("reading enforcement: %d %s", rec.Code, rec.Body)
	}
	return out
}

func (v *viabilityEnv) switchOn(t *testing.T, digest string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"enabled": true, "preview_digest": digest})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	v.enforcementHandlers().SetForecastAssuranceEnforcement(rec,
		httptest.NewRequest(http.MethodPut, "/forecast/assurance/enforcement", bytes.NewReader(body)).WithContext(v.Admin()))
	return rec
}

func TestSwitchingOnIsBoundToThePreviewTheAdminSaw(t *testing.T) {
	v := setupViability(t)
	v.check(t)

	seen := v.enforcement(t).Preview
	if seen == nil || seen.LeftOutCount != 1 || seen.UnknownCount != 0 {
		t.Fatalf("preview %+v, want the silent deal left out and nothing unknown", seen)
	}

	// The customer answers after the check: the receipt no longer matches.
	v.customerWrites(t, v.silent, v.now.Add(-time.Hour))
	if rec := v.switchOn(t, seen.Digest); rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("preview_stale")) {
		t.Fatalf("switching on a stale preview answered %d %s, want 409 preview_stale", rec.Code, rec.Body)
	}

	fresh := v.enforcement(t).Preview
	if fresh == nil || fresh.Digest == seen.Digest || fresh.UnknownCount != 1 {
		t.Fatalf("fresh preview %+v, want a new digest with the changed deal unknown", fresh)
	}
	if rec := v.switchOn(t, fresh.Digest); rec.Code != http.StatusOK {
		t.Fatalf("switching on the current preview answered %d %s", rec.Code, rec.Body)
	}
	if got := v.enforcement(t); got.State != crmcontracts.ForecastAssuranceEnforcementStateEnforcing || got.Preview != nil {
		t.Fatalf("after switching on: %+v, want enforcing with no preview", got)
	}
}

// The binding lives in the setting's writer, so no caller can switch on past
// it: the store recomputes the preview in the transaction that saves the
// setting, and a refused switch saves nothing.
func TestTheEnforcementWriterRecomputesThePreviewItSavesAgainst(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	store := assurance.NewStore(InstallationDB(v.Pool))
	cfg := StoredConfig(v.Pool)

	stale := "a digest no preview produced"
	err := store.SetQualifiedForecastEnforced(v.Admin(), cfg, true,
		assurance.PreviewBinding{Seen: &stale, Preview: EnforcementPreview})
	var refused *httperr.DetailedError
	if !errors.As(err, &refused) || refused.Code != "preview_stale" {
		t.Fatalf("switching on against a stale digest answered %v, want preview_stale", err)
	}
	if got := v.enforcement(t).State; got != crmcontracts.ForecastAssuranceEnforcementStateObserving {
		t.Fatalf("a refused switch left the forecast %s, want observing", got)
	}

	current := v.enforcement(t).Preview.Digest
	recomputed := 0
	counting := func(ctx context.Context, tx pgx.Tx) (assurance.EnforcementPreview, error) {
		recomputed++
		return EnforcementPreview(ctx, tx)
	}
	if err := store.SetQualifiedForecastEnforced(v.Admin(), cfg, true,
		assurance.PreviewBinding{Seen: &current, Preview: counting}); err != nil {
		t.Fatalf("switching on against the current digest: %v", err)
	}
	if recomputed != 1 {
		t.Errorf("the writer recomputed the preview %d times, want once", recomputed)
	}
	if got := v.enforcement(t).State; got != crmcontracts.ForecastAssuranceEnforcementStateEnforcing {
		t.Errorf("after switching on: %s, want enforcing", got)
	}
}
