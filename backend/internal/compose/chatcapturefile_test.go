// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

// An omitted extraction_id decodes to the zero UUID. It is named as missing
// before any lookup, so the caller is not told a reading it never named is gone.
func TestAChatConfirmWithNoReadingNamesTheMissingField(t *testing.T) {
	_, err := (&ChatCaptureFile{}).File(t.Context(), ids.NewV7(), crmcontracts.FileChatCaptureRequest{
		ChannelProvider: "whatsapp",
	})
	var refused *activities.ChatCaptureError
	if !errors.As(err, &refused) || refused.Field != "extraction_id" || refused.Code != "required" {
		t.Fatalf("an omitted extraction_id was not refused by name: %v", err)
	}
}

func TestOnlyTheReadingsOwnTextIsTheReaders(t *testing.T) {
	read := activities.ExtractionRead{Fields: []extraction.ExtractedField{
		{Field: chatFieldText, Value: "Lena: Thursday?"},
	}}
	for body, want := range map[string]bool{
		"Lena: Thursday?":   true,
		" Lena: Thursday? ": true,
		"Lena: Friday?":     false,
		"":                  false,
	} {
		if got := bodyIsTheReading(read, body); got != want {
			t.Errorf("bodyIsTheReading(%q) = %v, want %v", body, got, want)
		}
	}
}
