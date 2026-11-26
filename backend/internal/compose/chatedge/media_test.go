// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatedge

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

func body(b []byte, err error) MediaRef {
	return MediaRef{Ordinal: 1, Name: "a.pdf", Download: func(context.Context, ids.UUID, int64) ([]byte, error) { return b, err }}
}

func TestAttachTurnsDownloadsIntoPartsAndDropsByReason(t *testing.T) {
	pdf := []byte("%PDF-1.4 x")
	refused := errors.New("404")
	cases := []struct {
		name      string
		ref       MediaRef
		last      bool
		wantParts int
		wantDrop  string
		wantErr   bool
	}{
		{"a file", body(pdf, nil), false, 1, "", false},
		{"a refusal drops the file", body(nil, Refused(DropRefused, refused)), false, 0, DropRefused, false},
		{"too large drops the file", body(nil, Refused(DropTooLarge, refused)), false, 0, DropTooLarge, false},
		{"an empty download drops the file", body(nil, nil), false, 0, DropEmpty, false},
		{"a passing failure retries the job", body(nil, refused), false, 0, "", true},
		{"a passing failure on the last attempt drops the file", body(nil, refused), true, 0, DropUnavailable, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var msg connector.ChatMessage
			err := attach(context.Background(), ids.NewV7(), []MediaRef{tc.ref}, tc.last, &msg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if len(msg.Parts) != tc.wantParts {
				t.Errorf("%d parts, want %d", len(msg.Parts), tc.wantParts)
			}
			if tc.wantDrop != "" && (len(msg.PartDrops) != 1 || msg.PartDrops[0] != (connector.PartDrop{Reason: tc.wantDrop, Count: 1})) {
				t.Errorf("drops = %+v, want one %s", msg.PartDrops, tc.wantDrop)
			}
			if tc.wantDrop == "" && len(msg.PartDrops) != 0 {
				t.Errorf("drops = %+v, want none", msg.PartDrops)
			}
		})
	}
}

func TestAttachSniffsTheTypeAndKeepsTheClaimOnlyWhereItDiffers(t *testing.T) {
	ref := body([]byte("%PDF-1.4 x"), nil)
	ref.DeclaredType = "image/png"
	ref.Name = "../../evil.png"
	var msg connector.ChatMessage
	if err := attach(context.Background(), ids.NewV7(), []MediaRef{ref}, false, &msg); err != nil {
		t.Fatal(err)
	}
	got := msg.Parts[0]
	if got.ContentType != "application/pdf" || got.DeclaredType != "image/png" || got.Filename == "../../evil.png" || got.Ordinal != 1 {
		t.Errorf("part = %+v, want a sniffed pdf, the png claim kept, a safe name and ordinal 1", got)
	}
}

func TestAttachKeepsTheFirstFilesAndCountsTheRest(t *testing.T) {
	refs := make([]MediaRef, extension.MaxInboundFiles+3)
	for i := range refs {
		refs[i] = body([]byte("%PDF-1.4 x"), nil)
		refs[i].Ordinal = i + 1
	}
	var msg connector.ChatMessage
	if err := attach(context.Background(), ids.NewV7(), refs, false, &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.Parts) != extension.MaxInboundFiles || len(msg.PartDrops) != 1 || msg.PartDrops[0] != (connector.PartDrop{Reason: DropTooMany, Count: 3}) {
		t.Errorf("%d parts and drops %+v, want %d parts and 3 counted too many", len(msg.Parts), msg.PartDrops, extension.MaxInboundFiles)
	}
}
