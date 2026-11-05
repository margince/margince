// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestTheSinkSaysWhenItMaskedARecord(t *testing.T) {
	for name, tc := range map[string]struct {
		fields ActivityFields
		want   bool
	}{
		"a pin in a chat":           {ActivityFields{Body: "PIN: 4711"}, true},
		"a mapper that masked":      {ActivityFields{Body: "PIN: [masked]", CredentialsMasked: true}, true},
		"a typed marker, no mapper": {ActivityFields{Body: "hello [masked]"}, false},
		"a subject with a password": {ActivityFields{Subject: "password: hunter2x"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			rec := maskCapturedCredentials(connector.NormalizedRecord{Fields: tc.fields})
			fields, ok := rec.Fields.(ActivityFields)
			if !ok || fields.CredentialsMasked != tc.want {
				t.Errorf("CredentialsMasked = %v, want %v", fields.CredentialsMasked, tc.want)
			}
		})
	}
}
