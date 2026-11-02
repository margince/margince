// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

type threadBoundaryState struct {
	Key              string
	OwnTopic         string
	AncestorTopic    string
	AncestorDistance int
	FrontierKey      string
	FrontierTopic    string
	RootKey          string
	RootProven       bool
}

// threadBoundaryTopic removes only tags for which the caller holds durable project evidence.
func threadBoundaryTopic(subject string, projectKeys []string) string {
	recognized := make(map[string]bool, len(projectKeys))
	for _, key := range projectKeys {
		recognized[strings.ToLower(strings.TrimSpace(key))] = true
	}
	var plain strings.Builder
	for {
		start := strings.IndexByte(subject, '[')
		if start < 0 {
			plain.WriteString(subject)
			break
		}
		end := strings.IndexByte(subject[start+1:], ']')
		if end < 0 {
			plain.WriteString(subject)
			break
		}
		end += start + 1
		plain.WriteString(subject[:start])
		key := strings.ToLower(strings.TrimSpace(subject[start+1 : end]))
		if recognized[key] {
			plain.WriteByte(' ')
		} else {
			plain.WriteString(subject[start : end+1])
		}
		subject = subject[end+1:]
	}
	canonical := mailsubject.ThreadNormalized(plain.String())
	if canonical == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

func newThreadBoundaryState(key, topic string) threadBoundaryState {
	state := threadBoundaryState{Key: key, OwnTopic: topic, AncestorDistance: -1, RootKey: key}
	if topic != "" {
		state.AncestorTopic, state.AncestorDistance = topic, 0
		state.FrontierKey, state.FrontierTopic = key, topic
	}
	return state
}

// advanceThreadAncestor is one bounded edge step toward the nearest named ancestor.
func advanceThreadAncestor(child, parent threadBoundaryState) threadBoundaryState {
	if child.OwnTopic != "" || parent.AncestorDistance < 0 {
		return child
	}
	distance := parent.AncestorDistance + 1
	if child.AncestorDistance < 0 || distance < child.AncestorDistance {
		child.AncestorTopic, child.AncestorDistance = parent.AncestorTopic, distance
	}
	return child
}

// advanceThreadFrontier gives an unnamed root one stable topic without joining its other branches.
func advanceThreadFrontier(parent, child threadBoundaryState) threadBoundaryState {
	if parent.OwnTopic == "" && child.FrontierKey != "" && (parent.FrontierKey == "" || child.FrontierKey < parent.FrontierKey) {
		parent.FrontierKey, parent.FrontierTopic = child.FrontierKey, child.FrontierTopic
	}
	return parent
}

func (s threadBoundaryState) topic() string {
	if s.AncestorDistance >= 0 {
		return s.AncestorTopic
	}
	return s.FrontierTopic
}

func (s threadBoundaryState) rooted(parent *threadBoundaryState) threadBoundaryState {
	s.RootKey = s.Key
	s.RootProven = parent == nil || s.topic() != parent.topic()
	return s
}

// advanceThreadRoots shares the proven root, or a stable identity for a component containing a cycle.
func advanceThreadRoots(child, parent threadBoundaryState) (threadBoundaryState, threadBoundaryState) {
	if child.topic() != parent.topic() {
		return child, parent
	}
	best := child
	if parent.RootProven && !best.RootProven || parent.RootProven == best.RootProven && parent.RootKey < best.RootKey {
		best = parent
	}
	child.RootKey, child.RootProven = best.RootKey, best.RootProven
	parent.RootKey, parent.RootProven = best.RootKey, best.RootProven
	return child, parent
}
