// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package originalstore

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"strings"
)

// SubjectHintVersion lets readers treat unfamiliar filters conservatively.
const SubjectHintVersion = 1

// SubjectHintLimit caps metadata independently of the original's size.
const (
	SubjectHintLimit    = 1024
	subjectHintBitCount = 1 << 15
)

// SubjectHints exclude unrelated evidence; positive answers still use SQL's
// existing content predicate. Saturated and unknown filters exclude nothing.
type SubjectHints struct {
	Version        int      `json:"version"`
	EmailBits      []uint16 `json:"email_bits"`
	EmailSaturated bool     `json:"email_saturated,omitempty"`
	ChannelIDs     []string `json:"channel_ids,omitempty"`
}

func subjectHints(folded string, channels []string) *SubjectHints {
	hints := &SubjectHints{Version: SubjectHintVersion, EmailBits: make([]uint16, 0), ChannelIDs: channels}
	seen := make(map[uint16]struct{})
	for i := 0; i < len(folded); i++ {
		if folded[i] != '@' {
			continue
		}
		for _, width := range []int{3, 8} {
			for start := max(0, i-width+1); start <= i && start+width <= len(folded); start++ {
				for _, bit := range windowBits(folded[start : start+width]) {
					seen[bit] = struct{}{}
					if len(seen) > SubjectHintLimit {
						hints.EmailBits = nil
						hints.EmailSaturated = true
						return hints
					}
				}
			}
		}
	}
	for bit := range seen {
		hints.EmailBits = append(hints.EmailBits, bit)
	}
	slices.Sort(hints.EmailBits)
	return hints
}

func windowBits(window string) [2]uint16 {
	sum := sha256.Sum256([]byte(window))
	return [2]uint16{binary.LittleEndian.Uint16(sum[:2]) % subjectHintBitCount, binary.LittleEndian.Uint16(sum[2:4]) % subjectHintBitCount}
}

// MayMentionEmail excludes only evidence proven unrelated by a supported filter.
func (h *SubjectHints) MayMentionEmail(email string) bool {
	if h == nil || h.Version != SubjectHintVersion || h.EmailSaturated || h.EmailBits == nil {
		return true
	}
	queries := EmailHintQueries([]string{email})
	if queries[0] == nil {
		return true
	}
	for _, bit := range queries[0] {
		if !slices.ContainsFunc(h.EmailBits, func(stored uint16) bool { return int(stored) == bit }) {
			return false
		}
	}
	return true
}

func emailHintBits(email string) []uint16 {
	email = strings.ToLower(email)
	at := strings.IndexByte(email, '@')
	if at < 0 {
		return nil
	}
	width := 3
	if len(email) >= 8 {
		width = 8
	}
	var bits []uint16
	for start := max(0, at-width+1); start <= at && start+width <= len(email); start++ {
		pair := windowBits(email[start : start+width])
		bits = append(bits, pair[:]...)
	}
	return bits
}

// EmailHintQueries leaves unsupported addresses unfiltered rather than losing matches.
func EmailHintQueries(emails []string) [][]int {
	queries := make([][]int, 0, len(emails))
	for _, email := range emails {
		supported := len(email) >= 3
		for i := 0; i < len(email); i++ {
			supported = supported && email[i] < 128
		}
		var query []int
		if supported {
			for _, bit := range emailHintBits(email) {
				query = append(query, int(bit))
			}
		}
		queries = append(queries, query)
	}
	return queries
}
