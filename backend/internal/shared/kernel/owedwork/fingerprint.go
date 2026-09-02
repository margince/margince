// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedwork

// The digest a dismissal is held against.
//
// A reader dismisses a CARD, and the dismissal has to lift when the thing the
// card was about moves. Hashing the evidence — what it was, which row, and
// when — is what makes that automatic: new evidence, new digest, the card
// comes back.
//
// One spelling for every surface that shows such a card. Two would drift, and
// a drifted digest does not fail loudly: it silently stops matching, and every
// dismissal a reader ever made stops working at once.

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Mark is one piece of evidence reduced to what the digest reads: what kind of
// thing it is, which row it is, and when it was observed.
//
// The LABEL is deliberately absent. It is prose a build may reword without the
// underlying fact having moved, and a reworded headline must not silently
// un-dismiss what a reader put away.
type Mark struct {
	Kind string
	ID   string
	// At is when the evidence was observed. Nil where the evidence carries no
	// moment, which hashes as absent rather than as any particular time.
	At *time.Time
}

// Fingerprint digests the evidence a card fired on.
//
// Built as one string and hashed once. sha256's Write never returns an error,
// but writing through it would still spread unchecked returns across several
// calls to say what a single Sum says here.
func Fingerprint(marks []Mark) string {
	var b strings.Builder
	for _, m := range marks {
		b.WriteString(m.Kind)
		b.WriteByte(0)
		b.WriteString(m.ID)
		b.WriteByte(0)
		if m.At != nil {
			b.WriteString(strconv.FormatInt(m.At.UTC().UnixNano(), 10))
		}
		b.WriteByte(0)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
