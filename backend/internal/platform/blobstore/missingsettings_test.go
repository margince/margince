// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package blobstore

// A boot that cannot start names everything it is missing, once.
//
// The refusals used to be sequential — endpoint and bucket together, then
// region — so an operator starting the binary by hand paid one restart per
// setting they had not found, and the requirements were discoverable only by
// exhausting them. Nothing about the three is sequential: they are read from
// one environment at one instant, and answering with the first is a choice to
// withhold the other two.

import (
	"strings"
	"testing"
)

func TestAnIncompleteBlobstoreConfigNamesEverySettingItIsMissing(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		what    string
		cfg     Config
		wantAll []string
		wantNot []string
	}{
		{
			what:    "nothing set at all: all three, not just the first",
			cfg:     Config{},
			wantAll: []string{EnvEndpoint, EnvBucket, EnvRegion},
		},
		{
			what:    "the case the issue reported: endpoint found, two still to discover",
			cfg:     Config{Endpoint: "localhost:9000"},
			wantAll: []string{EnvBucket, EnvRegion},
			wantNot: []string{EnvEndpoint},
		},
		{
			what:    "only the region left, and it says why it is not defaulted",
			cfg:     Config{Endpoint: "localhost:9000", Bucket: "attachments"},
			wantAll: []string{EnvRegion, "not defaulted"},
			wantNot: []string{EnvEndpoint, EnvBucket},
		},
	} {
		err := missingBlobstoreSettings(c.cfg)
		if err == nil {
			t.Errorf("%s: the config was accepted; a boot that cannot reach object storage must say so", c.what)
			continue
		}
		for _, want := range c.wantAll {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: the error does not name %q, so an operator pays a restart to discover it:\n%v",
					c.what, want, err)
			}
		}
		for _, unwanted := range c.wantNot {
			if strings.Contains(err.Error(), unwanted) {
				t.Errorf("%s: the error names %q, which IS set — an error listing settings the operator "+
					"already provided sends them to check the ones that are fine:\n%v", c.what, unwanted, err)
			}
		}
	}
}

// And a complete config is accepted, so the check above cannot pass by
// refusing everything.
func TestACompleteBlobstoreConfigIsAccepted(t *testing.T) {
	t.Parallel()
	if err := missingBlobstoreSettings(Config{
		Endpoint: "localhost:9000", Bucket: "attachments", Region: "us-east-1",
	}); err != nil {
		t.Errorf("a config naming all three was refused: %v", err)
	}
}
