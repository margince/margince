// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
)

func TestTheRegistrationLimitOptionRecordsTheDeploymentFile(t *testing.T) {
	var s Server
	limit := identity.RegistrationMethods{Dynamic: false, MetadataDocument: true}
	WithClientRegistrationLimit(limit)(&s, nil)
	if s.clientRegistrationLimit == nil || *s.clientRegistrationLimit != limit {
		t.Fatalf("limit = %+v, want %+v", s.clientRegistrationLimit, limit)
	}
}
