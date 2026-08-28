// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// listing is the answer's shape, restated for the reason mintedSecret is.
type listing struct {
	Entries []inboundEntry `json:"entries"`
}

func TestListingReadsOnlyTheCallersOwnQueue(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, "", true)}
	sent := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	rt.tx.queryRows = [][]any{
		{"aa3d1b90-4f62-4c18-9d07-6e1a5b8c2f34", "n-1", "pending", 0, "", 42, sent, sent.Add(time.Second)},
	}

	out, err := listInbound(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	sql, args := rt.tx.statementMentioning(t, "endpoint_id = $1::uuid")
	if args[0] != endpointID {
		t.Fatalf("the listing is predicated on %v, not the caller's own endpoint", args[0])
	}
	if args[1] != inboundPageSize {
		t.Fatalf("the listing takes %v rows rather than one page of %d", args[1], inboundPageSize)
	}
	if !strings.Contains(sql, "ORDER BY received_at DESC") {
		t.Fatalf("the listing is not newest-first:\n%s", sql)
	}
	entries := jsonOf[listing](t, out).Entries
	if len(entries) != 1 || entries[0].BodyBytes != 42 {
		t.Fatalf("the listing answered %+v", entries)
	}
	if entries[0].SentAt != sent.Format(time.RFC3339) {
		t.Fatalf("sent_at rendered as %q", entries[0].SentAt)
	}
}

// The payload is bytes an anonymous stranger chose. The listing reports its
// SIZE, and a projection that started selecting the body would put those bytes
// into everything that renders this answer.
func TestListingNeverSelectsTheReceivedBody(t *testing.T) {
	t.Parallel()
	if strings.Contains(inboundColumns, "body,") || strings.HasSuffix(strings.TrimSpace(inboundColumns), "body") {
		t.Fatalf("the listing projection selects the body itself:\n%s", inboundColumns)
	}
	if !strings.Contains(inboundColumns, "octet_length(body)") {
		t.Fatalf("the listing projection no longer reports the payload's size:\n%s", inboundColumns)
	}
}

func TestListingAnEndpointNobodyOpenedIsEmptyRatherThanAFailure(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.noRows = map[int]bool{1: true}

	out, err := listInbound(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("listing without an endpoint must not be a failure: %v", err)
	}
	if entries := jsonOf[listing](t, out).Entries; len(entries) != 0 {
		t.Fatalf("the listing answered %d entries for a member with no endpoint", len(entries))
	}
	if !strings.Contains(string(out), `"entries":[]`) {
		t.Fatalf("an empty listing must encode as an empty array, not null:\n%s", out)
	}
}
