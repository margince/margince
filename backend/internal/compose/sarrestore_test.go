// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

func sarMessage(text string, file []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(file)
	var folded strings.Builder
	for at := 0; at < len(encoded); at += 76 {
		folded.WriteString(encoded[at:min(at+76, len(encoded))] + "\r\n")
	}
	return []byte("MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain\r\n\r\n" + text + "\r\n" +
		"--b1\r\nContent-Type: application/pdf\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		folded.String() + "--b1--\r\n")
}

// An Art. 15 export hands back the provider's original whichever spelling the
// column stored it in: text as a JSON string, bytes jsonb cannot hold as text
// in the base64 envelope.
func TestAnExportRestoresASlimmedOriginalInEitherSpelling(t *testing.T) {
	ctx := context.Background()
	file := bytes.Repeat([]byte("%PDF-1.4 figures "), 1024)
	sum := sha256.Sum256(file)
	blob := blobstore.NewMemory()
	if err := blob.Put(ctx, "ws/attachment/1", bytes.NewReader(file), int64(len(file)), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	stored := []partslim.StoredPart{{Ordinal: 1, Sha256: hex.EncodeToString(sum[:]), Bytes: int64(len(file)), StorageKey: "ws/attachment/1", Body: file}}
	for name, text := range map[string]string{"text": "plain words", "bytes": "not utf-8 \xff\xfe"} {
		raw := sarMessage(text, file)
		slim, count, err := partslim.StripStoredParts(raw, stored)
		if err != nil || count != 1 {
			t.Fatalf("%s: stripping the fixture: count=%d err=%v", name, count, err)
		}
		payload := func(original []byte) any {
			encoded, err := capture.EncodeStoredOriginal(original)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if err := json.Unmarshal(encoded, &value); err != nil {
				t.Fatal(err)
			}
			return value
		}
		pkg := &privacy.SARPackage{RawCapture: []map[string]any{{"payload": payload(slim)}}}
		restoreSAROriginals(ctx, blob, pkg)
		if got, want := pkg.RawCapture[0]["payload"], payload(raw); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the export holds %.80v, want the provider's original", name, got)
		}
	}
}
