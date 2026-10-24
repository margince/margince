// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
)

const (
	maskableLine = "Passwort: Hunter2-Example!"
	maskedLine   = "Passwort: " + credentialmask.Marker
)

func maskedImage(t *testing.T, entityType string, image map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(image)
	if err != nil {
		t.Fatalf("encoding the image: %v", err)
	}
	stored, err := maskAuditImage(entityType, encoded)
	if err != nil {
		t.Fatalf("maskAuditImage: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(stored)))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		t.Fatalf("the stored image is not an object: %v", err)
	}
	return out
}

func TestAnActivityImageStoresEveryTextWithItsCredentialsMasked(t *testing.T) {
	got := maskedImage(t, "activity", map[string]any{
		"subject":   maskableLine,
		"body":      "Hi,\n" + maskableLine + "\nBye",
		"cf_access": []any{map[string]any{"note": maskableLine}},
	})
	if got["subject"] != maskedLine {
		t.Errorf("subject = %q, want %q", got["subject"], maskedLine)
	}
	if want := "Hi,\n" + maskedLine + "\nBye"; got["body"] != want {
		t.Errorf("body = %q, want %q", got["body"], want)
	}
	nested, ok := got["cf_access"].([]any)
	if !ok || len(nested) != 1 {
		t.Fatalf("cf_access = %v, want one element", got["cf_access"])
	}
	if inner, ok := nested[0].(map[string]any); !ok || inner["note"] != maskedLine {
		t.Errorf("a text nested in an array reached the trail as %v", nested[0])
	}
}

// Decoding through float64 would round an int64 past 2^53, and the trail is
// where a restore reads a count back from.
func TestMaskingAnImageKeepsEveryDigitOfALargeNumber(t *testing.T) {
	const large = "9007199254740993"
	got := maskedImage(t, "activity", map[string]any{"body": maskableLine, "duration_seconds": json.Number(large)})
	if number, ok := got["duration_seconds"].(json.Number); !ok || number.String() != large {
		t.Errorf("duration_seconds = %v, want %s", got["duration_seconds"], large)
	}
}

func TestAnImageWithNothingToMaskIsStoredAsEncoded(t *testing.T) {
	encoded := []byte(`{"body":"the original text","is_done":true}`)
	stored, err := maskAuditImage("activity", encoded)
	if err != nil {
		t.Fatalf("maskAuditImage: %v", err)
	}
	if string(stored) != string(encoded) {
		t.Errorf("stored %s, want the image unchanged", stored)
	}
}

func TestATypeOutsideTheMaskedSetKeepsItsText(t *testing.T) {
	if got := maskedImage(t, "contact", map[string]any{"title": maskableLine}); got["title"] != maskableLine {
		t.Errorf("title = %q, want it as written", got["title"])
	}
	if got := AuditedText("contact", maskableLine); got != maskableLine {
		t.Errorf("AuditedText(contact) = %q, want it as written", got)
	}
}

func TestAMaskedImageMasksToItself(t *testing.T) {
	once := maskedImage(t, "activity", map[string]any{"body": maskableLine})
	if twice := maskedImage(t, "activity", once); twice["body"] != once["body"] {
		t.Errorf("a second pass stored %q, want %q", twice["body"], once["body"])
	}
}
