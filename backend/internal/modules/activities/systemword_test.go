// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A writer passing the system's older word must name an identity the
// follow-ups read, or the task it opens is never completed by one.
func TestASystemTaskNamesAnIdentityTheFollowUpsRead(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		in   LogActivityInput
		want bool
	}{
		"a lead response escalation":  {LogActivityInput{Source: systemSource, SourceSystem: new(provenance.LeadSLATaskSource)}, true},
		"no identity at all":          {LogActivityInput{Source: systemSource}, true},
		"a commitment's identity":     {LogActivityInput{Source: systemSource, SourceSystem: new(provenance.CommitmentTaskSource)}, false},
		"another writer's own source": {LogActivityInput{Source: provenance.CommitmentTaskSource, SourceSystem: new(provenance.CommitmentTaskSource)}, true},
	} {
		if err := checkSystemWord(c.in); (err == nil) != c.want {
			t.Errorf("%s: err = %v, want admitted %v", name, err, c.want)
		}
	}
}
