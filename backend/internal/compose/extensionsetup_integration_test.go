// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/extsecrets"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestAUnitNeedsSetupUntilItsWorkspaceSecretIsStored(t *testing.T) {
	e := setupExtRuntime(t)
	const unit = "setup-probe"
	t.Cleanup(func() { setComposedExtensions(nil) })
	setComposedExtensions([]extension.Extension{{Name: unit, Version: "1.0.0", Secrets: []extension.SecretsRequest{
		{Key: "api-key", Scope: extension.SecretScopeWorkspace},
		{Key: "own-token", Scope: extension.SecretScopeUser},
	}}})
	list := func() []crmcontracts.ComposedExtensionSetupItem {
		t.Helper()
		ctx := e.Admin()
		rec := httptest.NewRecorder()
		extensionsHandlers{pool: e.Pool}.ListExtensions(rec, httptest.NewRequest(http.MethodGet, "/v1/extensions", nil).WithContext(ctx))
		var got crmcontracts.ExtensionDirectory
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || len(got.Extensions) != 1 || got.Extensions[0].SetupMissing == nil {
			t.Fatalf("list → %d %s (decode err %v)", rec.Code, rec.Body.String(), err)
		}
		return *got.Extensions[0].SetupMissing
	}

	want := []crmcontracts.ComposedExtensionSetupItem{{Kind: crmcontracts.ComposedExtensionSetupItemKindWorkspaceSecret, Name: "api-key"}}
	if got := list(); !slices.Equal(got, want) {
		t.Fatalf("before the key is set, missing = %+v, want %+v", got, want)
	}
	if err := extsecrets.For(unit, e.Pool, e.vault).Put(e.callCtx(e.WS), "api-key", []byte("sk_probe")); err != nil {
		t.Fatalf("storing the key: %v", err)
	}
	if got := list(); len(got) != 0 {
		t.Errorf("after the key is set, missing = %+v, want none: the unit is ready", got)
	}
}
