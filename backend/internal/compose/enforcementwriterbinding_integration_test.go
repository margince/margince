// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/assurance"
)

// The writer checks the digest, not only the door. Asked on another connection
// it leaves a window: a deal changing treatment between the check and the save
// is switched on against numbers nobody saw.
//
// So this calls the store directly, with no handler in front of it.
func TestTheWriterRefusesAStaleDigestWithNoHandlerInFront(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	seen := v.enforcement(t).Preview
	if seen == nil {
		t.Fatal("no preview to go stale")
	}
	// The customer answers after the check, so the count the digest named is
	// no longer the count the deals give.
	v.customerWrites(t, v.silent, v.now.Add(-time.Hour))

	store := assurance.NewStore(InstallationDB(v.Pool)).WithPreviewTx(EnforcementPreviewTx)
	err := store.SetQualifiedForecastEnforced(v.Admin(), StoredConfig(v.Pool), true, &seen.Digest)
	if err == nil {
		t.Fatal("the writer took a stale digest: the check has to run where the save does")
	}
	if !strings.Contains(err.Error(), "preview_stale") &&
		!strings.Contains(err.Error(), "changed since you looked") {
		t.Errorf("refusal = %v, want the stale-preview conflict", err)
	}

	// The current digest goes through, so the refusal is the digest and not
	// the writer refusing everything.
	fresh := v.enforcement(t).Preview
	if fresh == nil {
		t.Fatal("no fresh preview")
	}
	if err := store.SetQualifiedForecastEnforced(
		v.Admin(), StoredConfig(v.Pool), true, &fresh.Digest,
	); err != nil {
		t.Errorf("the writer refused the current digest: %v", err)
	}
}
