// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// A connector's record may name any kind, but an activity links only to a
// contact, company or deal; anything else is refused before a row is written.
func TestACapturedActivityRefusesALinkToAKindActivitiesDoNotLinkTo(t *testing.T) {
	t.Parallel()
	links := []datasource.EntityRef{{Type: datasource.EntityLead, ID: ids.NewV7()}}

	err := (&Sink{}).linkActivity(context.Background(), nil, ids.New[ids.ActivityKind](), links)

	if err == nil || !strings.Contains(err.Error(), "cannot link a lead") {
		t.Fatalf("linking an activity to a lead = %v, want a refusal naming the kind", err)
	}
}
