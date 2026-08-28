// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// mintedSecret is the answer's shape, restated here rather than exported from
// the handler: a test that reached for the production struct would pass even if
// the JSON member were renamed, and the member name is what a client reads.
type mintedSecret struct {
	SigningSecret string   `json:"signing_secret"`
	Endpoint      endpoint `json:"endpoint"`
}

func TestMintingSealsTheSecretUnderTheCallerAndShowsItOnce(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{
		endpointRow(endpointID, ownerUserID, "", true),
		endpointRow(endpointID, ownerUserID, "", true),
		endpointRow(endpointID, ownerUserID, "", true),
	}

	out, err := mintSecret(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	minted := jsonOf[mintedSecret](t, out)
	sealed, ok := rt.secrets.stored[ownerUserID+"/"+inboundSecretKey]
	if !ok {
		t.Fatalf("nothing was sealed for the caller; the namespace holds %v", rt.secrets.stored)
	}
	if string(sealed) != minted.SigningSecret {
		t.Fatal("the secret shown to the caller is not the one that was sealed, so nothing they configure will ever verify")
	}
	raw, err := hex.DecodeString(minted.SigningSecret)
	if err != nil {
		t.Fatalf("the minted secret is not hex, so it will not survive being pasted: %v", err)
	}
	if len(raw) != signingSecretBytes {
		t.Fatalf("the minted secret is %d bytes, want %d", len(raw), signingSecretBytes)
	}
	// The endpoint carried beside it must not repeat the credential: everything
	// about the row is rendered, and a secret in a rendered object is one every
	// reader of that screen holds.
	encoded, err := json.Marshal(minted.Endpoint)
	if err != nil {
		t.Fatalf("re-encoding the endpoint: %v", err)
	}
	if strings.Contains(string(encoded), minted.SigningSecret) {
		t.Fatalf("the endpoint object carries the secret:\n%s", encoded)
	}
}

func TestMintingTwiceReplacesTheSecretRatherThanAddingOne(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rows := [][]any{}
	for range 6 {
		rows = append(rows, endpointRow(endpointID, ownerUserID, "", true))
	}
	rt.tx.singleRows = rows

	first, err := mintSecret(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("first mint: %v", err)
	}
	second, err := mintSecret(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("second mint: %v", err)
	}
	one, two := jsonOf[mintedSecret](t, first), jsonOf[mintedSecret](t, second)
	if one.SigningSecret == two.SigningSecret {
		t.Fatal("rotating answered the same secret, so rotating changes nothing")
	}
	if len(rt.secrets.stored) != 1 {
		t.Fatalf("the namespace holds %d secrets for one endpoint; a second valid one is a credential nobody revokes", len(rt.secrets.stored))
	}
	if string(rt.secrets.stored[ownerUserID+"/"+inboundSecretKey]) != two.SigningSecret {
		t.Fatal("the sealed secret is not the most recently minted one")
	}
}

func TestMintingForAnEndpointNobodyOpenedSealsNothing(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.noRows = map[int]bool{1: true}

	_, err := mintSecret(context.Background(), rt, json.RawMessage(noArgs))
	if !errors.Is(err, extension.ErrNotFound) {
		t.Fatalf("minting without an endpoint must be not-found, got %v", err)
	}
	if len(rt.secrets.stored) != 0 {
		t.Fatal("material was sealed for a member who owns no endpoint, and this surface has no operation to withdraw it")
	}
}

func TestMintingRecordsTheRotationAgainstTheEndpoint(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{
		endpointRow(endpointID, ownerUserID, "", true),
		endpointRow(endpointID, ownerUserID, "", true),
		endpointRow(endpointID, ownerUserID, "", true),
	}

	if _, err := mintSecret(context.Background(), rt, json.RawMessage(noArgs)); err != nil {
		t.Fatalf("minting: %v", err)
	}
	if len(rt.tx.audited) != 1 {
		t.Fatalf("minting recorded %d ledger rows, want 1 — a rotation is exactly the fact somebody asks about later", len(rt.tx.audited))
	}
	if rt.tx.published[0].Verb != eventSecretMinted {
		t.Fatalf("minting published %q", rt.tx.published[0].Verb)
	}
	// The ledger images come from the row, which has no secret column, so this
	// is a property of the schema rather than of a filter. Asserted anyway,
	// because the day somebody adds the column is the day it stops holding.
	for _, change := range rt.tx.audited {
		if strings.Contains(string(change.After), "secret") {
			t.Fatalf("the audited image mentions a secret:\n%s", change.After)
		}
	}
}
