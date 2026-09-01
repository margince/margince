// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What GET /passports actually puts on the wire. The store's own tests prove
// which ROWS the list returns; these prove what each row becomes — the seam
// where "absent" has to stay absent rather than becoming a zero value a client
// would read as an answer.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func passportRowForTest() PassportRow {
	return PassportRow{
		ID:        ids.New[ids.PassportKind](),
		Scopes:    []string{"read"},
		CreatedAt: time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC),
	}
}

// `connection` is what Settings splits its two cards on, so the difference
// between "omitted" and "null" is load-bearing rather than cosmetic: a client
// that tests presence must not see a key at all on a minted passport, and the
// contract says so in as many words.
func TestAMintedPassportCarriesNoConnectionKeyAtAll(t *testing.T) {
	encoded, err := json.Marshal(passportSummary(passportRowForTest()))
	if err != nil {
		t.Fatalf("encoding the summary: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("decoding the summary: %v", err)
	}
	if _, present := wire["connection"]; present {
		t.Fatalf("a minted passport carries a connection key (%s); the contract says it is omitted, not null", encoded)
	}
}

// A NULL label is a string the contract requires, so it becomes "" rather than
// failing the read or arriving as a null a client has to guard.
func TestAnUnlabelledPassportReadsAsAnEmptyName(t *testing.T) {
	if got := passportSummary(passportRowForTest()).Label; got != "" {
		t.Fatalf("label = %q, want the empty string", got)
	}
	label := "Scout"
	row := passportRowForTest()
	row.Label = &label
	if got := passportSummary(row).Label; got != label {
		t.Fatalf("label = %q, want %q", got, label)
	}
}

// The whole connection, mapped. Asserted field by field because a mapping that
// dropped one — or crossed connected_at with the passport's own created_at —
// would still produce a well-formed response.
func TestAConnectionMapsEveryFieldItWasGiven(t *testing.T) {
	connectedAt := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	row := passportRowForTest()
	row.Connection = &PassportConnectionRow{
		ClientID:    "dcr-client-id",
		ClientName:  "Claude Code",
		ConnectedAt: connectedAt,
		Renewable:   true,
	}

	got := passportSummary(row).Connection
	if got == nil {
		t.Fatal("a grant-bound row produced no connection")
	}
	if got.ClientId != "dcr-client-id" || got.ClientName != "Claude Code" {
		t.Fatalf("client = %q/%q, want dcr-client-id/Claude Code", got.ClientId, got.ClientName)
	}
	// The GRANT's age, never the credential's: the row above was created a day
	// earlier than its connection, so a mapping that read the wrong one shows it.
	if !got.ConnectedAt.Equal(connectedAt) {
		t.Fatalf("connected_at = %v, want the grant's own %v", got.ConnectedAt, connectedAt)
	}
	if !got.Renewable {
		t.Fatal("renewable = false although the grant allows refresh; a reader would call this connection dead at its next expiry")
	}
}
