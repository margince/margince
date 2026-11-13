// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Key is what one written assessment answers for: this reader and
// subject, everything the model read, this prompt and this model binding. The
// evaluation's own instant is left out, so the same figures read a minute
// later are the same assessment.
func Key(reader, lang, routingVersion string, subject []byte, in Input) string {
	hash := sha256.New()
	for _, part := range [][]byte{[]byte(reader), []byte(lang), []byte(routingVersion), []byte(PromptVersion), subject, KeyPart(in)} {
		hash.Write(part)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// KeyPart spells one thing an assessment answers for: a selection, a Progress
// period or the model's input.
func KeyPart[T crmcontracts.ReportingSelection | crmcontracts.GetSalesProgressParams | Input](read T) []byte {
	encoded, err := json.Marshal(read)
	if err != nil {
		return []byte(fmt.Sprint(read))
	}
	return encoded
}
