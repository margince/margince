// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

// corruptingBlob is base64 that happens to spell a credential shape.
//
// `AKIA` plus sixteen uppercase characters is what the aws_access_key rule
// matches, and the word boundary it relies on is satisfied inside a blob by the
// `+`, `/` and `=` of the alphabet — or, at the very start, by the quote that
// opens the JSON string. Nothing about encoded bytes prevents this; it is the
// case nobody writes by accident because the natural one never triggers.
const corruptingBlob = "AKIAABCDEFGHIJKLMNOP+deadbeefQUJDRA=="

func stripBody(t *testing.T, body string) string {
	t.Helper()
	out, _, err := NewSecretStripper().Strip(context.Background(), []byte(body))
	if err != nil {
		t.Fatalf("Strip: %v", err)
	}
	return string(out)
}

// The regression this file exists for: an attachment comes out byte-identical,
// on every wire this module speaks.
//
// Per wire, because each spells the encoded field differently — a fix that
// covered Anthropic and left Gemini corrupting is the half-job this table
// makes impossible to land.
func TestAnEncodedAttachmentSurvivesTheStripper(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		wire string
		body string
	}{
		{"anthropic", `{"messages":[{"content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + corruptingBlob + `"}}]}]}`},
		{"gemini", `{"contents":[{"parts":[{"inline_data":{"mime_type":"image/png","data":"` + corruptingBlob + `"}}]}]}`},
		{"openai", `{"messages":[{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + corruptingBlob + `"}}]}]}`},
		{"ollama", `{"messages":[{"role":"user","images":["` + corruptingBlob + `"]}]}`},
	} {
		t.Run(tc.wire, func(t *testing.T) {
			t.Parallel()
			if got := stripBody(t, tc.body); got != tc.body {
				t.Errorf("the %s body was altered.\n got: %s\nwant: %s", tc.wire, got, tc.body)
			}
		})
	}
}

// And the exclusion must not widen into a hole: a credential in ordinary text
// is still removed, including in the same body as an attachment.
//
// This is the half that makes the one above safe. A stripper that stopped
// scanning would pass every byte-identical assertion in this file.
func TestACredentialInTextIsStillStrippedBesideAnAttachment(t *testing.T) {
	t.Parallel()
	const secret = "AKIAQRSTUVWXYZABCDEF"
	body := `{"messages":[{"content":[` +
		`{"type":"text","text":"the key is ` + secret + `, use it"},` +
		`{"type":"image","source":{"type":"base64","data":"` + corruptingBlob + `"}}]}]}`

	got := stripBody(t, body)
	if strings.Contains(got, secret) {
		t.Errorf("a credential in prose survived beside an attachment: %s", got)
	}
	if !strings.Contains(got, "[SECRET-REMOVED:aws_access_key]") {
		t.Errorf("the credential was not replaced with the redaction marker: %s", got)
	}
	if !strings.Contains(got, corruptingBlob) {
		t.Errorf("the attachment was altered while stripping the text beside it: %s", got)
	}
}

// A plain https URL under `url` is ordinary text, and a credential may hide in
// its query string. Only a data: URL carries bytes, which is why the value is
// tested and not the key.
func TestAPlainURLIsStillScannedUnderTheSameKey(t *testing.T) {
	t.Parallel()
	const secret = "AKIAQRSTUVWXYZABCDEF"
	body := `{"image_url":{"url":"https://example.test/fetch?key=` + secret + `"}}`
	if got := stripBody(t, body); strings.Contains(got, secret) {
		t.Errorf("a credential in an ordinary URL survived: %s", got)
	}
}

// A body carrying no attachment is scanned end to end, which is the behaviour
// every other case in this package already asserts — restated here because the
// segment walk is what would break it.
func TestABodyWithNoAttachmentIsOneSegment(t *testing.T) {
	t.Parallel()
	const secret = "AKIAQRSTUVWXYZABCDEF"
	if got := stripBody(t, `{"text":"`+secret+`"}`); strings.Contains(got, secret) {
		t.Errorf("a credential in an attachment-free body survived: %s", got)
	}
}

// Real encoded bytes, not a hand-made string: a megabyte of arbitrary data
// round-trips unaltered, which is the property a corpus test is for.
func TestArbitraryEncodedBytesRoundTrip(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 1<<16)
	for i := range raw {
		raw[i] = byte(i * 7 % 251)
	}
	blob := base64.StdEncoding.EncodeToString(raw)
	body := `{"source":{"type":"base64","data":"` + blob + `"}}`
	if got := stripBody(t, body); got != body {
		t.Error("arbitrary encoded bytes were altered by the stripper")
	}
}
