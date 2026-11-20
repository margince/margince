// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestNeedValidate(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		need    extension.Need
		wantErr string
	}{
		"models with a reason":  {need: extension.Need{Capability: extension.CapabilityModels, Reason: "Summarises a thread."}},
		"objects with a reason": {need: extension.Need{Capability: extension.CapabilityObjects, Reason: "Keeps generated PDFs."}},
		"unknown capability":    {need: extension.Need{Capability: "vault", Reason: "Reads tokens."}, wantErr: "not a capability"},
		"empty reason":          {need: extension.Need{Capability: extension.CapabilityModels}, wantErr: "reason"},
		"multi-line reason":     {need: extension.Need{Capability: extension.CapabilityModels, Reason: "one\ntwo"}, wantErr: "reason"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := tc.need.Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Validate() = %v, want nil", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("Validate() = %v, want an error naming %q", err, tc.wantErr)
			}
		})
	}
}

func TestDeclaresIsExactlyWhatNeedsLists(t *testing.T) {
	t.Parallel()
	ext := extension.Extension{Needs: []extension.Need{{Capability: extension.CapabilityModels, Reason: "x"}}}
	if !ext.Declares(extension.CapabilityModels) {
		t.Error("a listed capability must be declared")
	}
	if ext.Declares(extension.CapabilityObjects) {
		t.Error("an unlisted capability must not be declared")
	}
}
