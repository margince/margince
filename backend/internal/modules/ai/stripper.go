// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"sort"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// NewSecretStripper builds the credential-hygiene pass that runs over
// every model-bound payload (ai-operational-spec §4.2). It removes
// secrets — API keys, tokens, private keys, password assignments —
// irreversibly. It is NOT a PII filter: names, emails and phone numbers
// pass through untouched, because privacy is the location ladder (A8
// revised), and pretending a regex protects PII would be a false
// guarantee.
//
// It is a TEXT-lane guarantee. The pass runs over the marshalled body, the last
// point before egress. An attachment rides that body base64-encoded, and every
// credentialmask rule matches a secret's literal text. A credential inside an
// attached file is not present in that form, so nothing here can find it. The
// same file arriving as text is scrubbed. Reaching a file's text would mean
// decoding and re-encoding every attachment on every call. The product states
// the scope instead of implying a cover it does not have.
//
// Note which way that runs, because the converse used to bite. The rules are
// not blind to the ENCODING: two of them (AIza…, AKIA/ASIA…) are alphanumeric
// enough to match inside a blob by coincidence, and a match there replaced
// encoded bytes with a redaction marker — characters base64 has no alphabet
// for, injected into the middle of a file.
//
// The pass no longer reaches into an attachment: stripperspans.go finds the
// encoded fields by POSITION in the document the adapter built and scans around
// them, so a rule added tomorrow inherits the exclusion rather than having to be
// written not to need it. What the rules still cannot see is the plaintext
// underneath the encoding, which is the scope this comment opens with.
func NewSecretStripper() model.SecretStripper {
	return secretStripper{}
}

type secretStripper struct{}

func (s secretStripper) Strip(_ context.Context, payload []byte) ([]byte, model.StripReport, error) {
	report := model.StripReport{}
	kinds := map[string]bool{}
	// Segment by segment, around the encoded attachment bytes rather than
	// through them — stripperspans.go says why, and the whole body is one
	// segment when it carries no attachment.
	var out []byte
	at := 0
	for _, seg := range scannableSegments(payload, encodedSpans(payload)) {
		out = append(out, payload[at:seg.from]...)
		out = append(out, s.stripSegment(payload[seg.from:seg.to], &report, kinds)...)
		at = seg.to
	}
	out = append(out, payload[at:]...)
	for k := range kinds {
		report.Kinds = append(report.Kinds, k)
	}
	sort.Strings(report.Kinds)
	return out, report, nil
}

// stripSegment runs the shared credential rules for the JSON lane over one
// scannable stretch of the body.
func (secretStripper) stripSegment(payload []byte, report *model.StripReport, kinds map[string]bool) []byte {
	stripped, found := credentialmask.Replace(payload, credentialmask.JSON, func(kind string) string {
		return "[SECRET-REMOVED:" + kind + "]"
	})
	report.Findings += len(found)
	for _, kind := range found {
		kinds[kind] = true
	}
	return stripped
}
