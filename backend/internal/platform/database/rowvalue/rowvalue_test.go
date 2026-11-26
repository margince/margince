// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package rowvalue

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADocumentSpellsIntervalsBytesAndNestedUUIDsAsText(t *testing.T) {
	member := ids.MustParse("0195f3a0-0000-7000-8000-000000000002")
	raw, err := json.Marshal(Document(map[string]any{
		"wait":    pgtype.Interval{Days: 2, Valid: true},
		"bits":    pgtype.Bits{Bytes: []byte{0x80}, Len: 1, Valid: true},
		"hash":    []byte("A"),
		"members": []any{[16]byte(member)},
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"bits":"1","hash":"\\x41","members":["` + member.String() + `"],"wait":"2 day 00:00:00"}`
	if string(raw) != want {
		t.Fatalf("document = %s, want %s", raw, want)
	}
}

func TestCompactJSONKeepsStoredDigitsAndCharacters(t *testing.T) {
	got, err := CompactJSON(map[string]any{"n": json.Number("9007199254740993"), "name": "A&B"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"n":9007199254740993,"name":"A&B"}`; got != want {
		t.Fatalf("CompactJSON = %s, want %s", got, want)
	}
}

func TestTheExactCodecKeepsAnIntegerAbovePrecision(t *testing.T) {
	reader := NewReader()
	jsonb, ok := reader.types.TypeForOID(pgtype.JSONBOID)
	if !ok {
		t.Fatal("the reader has no jsonb codec")
	}
	binary := append([]byte{1}, `{"n":9007199254740993}`...)
	value, err := jsonb.Codec.DecodeValue(reader.types, pgtype.JSONBOID, pgtype.BinaryFormatCode, binary)
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := value.(map[string]any)
	if !ok || doc["n"] != json.Number("9007199254740993") {
		t.Fatalf("decoded %#v, want the stored digits as a json.Number", value)
	}
}
