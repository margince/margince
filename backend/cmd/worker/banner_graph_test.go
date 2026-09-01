// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
)

// The banner names the lanes that CAME UP, not the ones that were asked for.
//
// It is the one place an operator looks to check, so a line claiming the Graph
// renewal is running when no job was registered sends them to look for a fault
// somewhere else entirely — while every Outlook mailbox quietly stays on the
// poll.
func TestTheBannerNamesTheGraphRenewalOnlyWhenItIsRegistered(t *testing.T) {
	t.Parallel()
	cfg := workerConfig{
		graphClientID:      "an-app",
		graphNotifyURL:     "https://example.test/webhooks/graph",
		graphWatchInterval: time.Hour,
	}
	for name, tc := range map[string]struct {
		cfg   workerConfig
		runs  bool
		holds string
	}{
		"registered": {cfg, true, "graph subscription renew every"},
		// The url is set and the connector is not — a role with incomplete
		// client credentials. Nothing was registered, and the banner used to
		// say the renewal was running.
		"no connector for this role": {cfg, false, "no graph app configured for this role"},
		"no url at all": {
			workerConfig{graphClientID: "an-app", graphWatchInterval: time.Hour},
			false, "no notification url",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			banner := jobRunnerBanner(tc.cfg, compose.GmailWatchConfig{}, tc.runs, compose.ModelPath{}, nil, nil)
			if !strings.Contains(banner, tc.holds) {
				t.Errorf("the banner reads %q, want it to say %q", banner, tc.holds)
			}
			if !tc.runs && strings.Contains(banner, "graph subscription renew every") {
				t.Errorf("the banner claims the renewal lane is running when nothing registered it: %q", banner)
			}
		})
	}
}

// THE DEFAULT DEPLOYMENT, which is every installation that never opted into
// Outlook: no Entra app and no notification URL. The banner says nothing about
// Graph at all — not "no notification url", which would read as a
// misconfiguration to an operator who never asked for the lane, and which is
// the one branch every case above steps over because they all set a client id.
func TestTheBannerIsSilentAboutGraphOnADeploymentThatNeverAskedForIt(t *testing.T) {
	t.Parallel()
	banner := jobRunnerBanner(workerConfig{}, compose.GmailWatchConfig{}, false, compose.ModelPath{}, nil, nil)
	if strings.Contains(banner, "graph") {
		t.Errorf("the banner mentions graph on a deployment that configured none of it: %q", banner)
	}
}
