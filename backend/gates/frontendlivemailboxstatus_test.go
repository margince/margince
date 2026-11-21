// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The composer and the send refusal must call the same connections live. The
// server lists the mailboxes a send may use from the statuses in
// capture/sendingmailbox.go, and the browser mirrors that list in
// connector-status.ts. A status on one side only makes a degraded mailbox look
// connected to the server and missing to the browser, or the reverse.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	serverLiveStatuses = "internal/modules/capture/sendingmailbox.go"
	clientLiveStatuses = "../frontend/src/lib/connector-status.ts"
)

var (
	sqlStatusList = regexp.MustCompile(`status IN \(([^)]*)\)`)
	tsLiveSet     = regexp.MustCompile(`LIVE_STATUSES[^=]*=\s*new Set\(\[([^\]]*)\]\)`)
	quotedWord    = regexp.MustCompile(`["']([a-z_]+)["']`)
)

func statusWords(t *testing.T, list string) []string {
	t.Helper()
	var out []string
	for _, m := range quotedWord.FindAllStringSubmatch(list, -1) {
		out = append(out, m[1])
	}
	slices.Sort(out)
	return out
}

func TestTheFrontendLiveMailboxStatusesMatchTheServerOnes(t *testing.T) {
	t.Parallel()
	server, err := gatekit.ReadFile(serverLiveStatuses)
	if err != nil {
		t.Fatalf("reading the server's sending-mailbox query: %v", err)
	}
	client, err := gatekit.ReadFile(clientLiveStatuses)
	if err != nil {
		t.Fatalf("reading the frontend connector status module: %v", err)
	}
	fromServer := sqlStatusList.FindAllStringSubmatch(string(server), -1)
	if len(fromServer) == 0 {
		t.Fatalf("%s lists no `status IN (...)` filter any more; this gate reads a shape that is gone", serverLiveStatuses)
	}
	fromClient := tsLiveSet.FindStringSubmatch(string(client))
	if fromClient == nil {
		t.Fatalf("%s declares no LIVE_STATUSES set; this gate reads a shape that is gone", clientLiveStatuses)
	}
	want := statusWords(t, fromClient[1])
	for _, m := range fromServer {
		if got := statusWords(t, strings.ReplaceAll(m[1], "'", `"`)); !slices.Equal(got, want) {
			t.Errorf("%s filters on statuses %v but the frontend treats %v as live", serverLiveStatuses, got, want)
		}
	}
}
