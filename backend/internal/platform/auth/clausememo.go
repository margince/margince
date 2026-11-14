// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// templateBase numbers a template's placeholders far above any statement's
// own, so bind can tell them from a caller's and renumber only those.
const templateBase = 1_000_000_000

// clauseMemoCap bounds the memo to about 2 MB per gate. Past it the memo
// starts over, which costs one build per principal again, never a wrong answer.
const clauseMemoCap = 64

// clauseBuilder renders a clause from the principal, the alias and the
// placeholder order alone. Anything else it read would make a memo of it wrong.
type clauseBuilder func(p principal.Principal, alias string, arg func(any) int) string

// clauseMemo keeps one rendering per principal and alias. Each activity gate is
// tens of kilobytes built from dozens of nested Sprintf calls. A mail import
// asks for it several times per message, always for the same mailbox owner.
type clauseMemo struct {
	mu      sync.RWMutex
	entries map[string]clauseTemplate
}

// One memo per builder, because the key names the reader and not the gate.
var (
	contentClauses  = &clauseMemo{entries: map[string]clauseTemplate{}}
	discoverClauses = &clauseMemo{entries: map[string]clauseTemplate{}}

	messageContentClauses = &clauseMemo{entries: map[string]clauseTemplate{}}
)

// clauseTemplate is one rendering whose placeholders start at templateBase,
// with the values they bind in the order the builder asked for them.
type clauseTemplate struct {
	sql    string
	values []any
}

// render returns what build would return for this caller's arg. The key is
// the whole principal printed with %#v, so a field the builder starts to read
// later is already part of it.
//
// An alias holding '$' is built directly, since bind could not tell its digits
// from a placeholder. So is a template whose placeholders bind cannot all map.
func (m *clauseMemo) render(p principal.Principal, alias string, arg func(any) int, build clauseBuilder) string {
	if strings.ContainsRune(alias, '$') {
		return build(p, alias, arg)
	}
	key := alias + "\x00" + fmt.Sprintf("%#v", p)
	m.mu.RLock()
	template, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok {
		template = recordTemplate(p, alias, build)
		if !template.placeholdersMap() {
			return build(p, alias, arg)
		}
		m.mu.Lock()
		if len(m.entries) >= clauseMemoCap {
			clear(m.entries)
		}
		m.entries[key] = template
		m.mu.Unlock()
	}
	return template.bind(arg)
}

func recordTemplate(p principal.Principal, alias string, build clauseBuilder) clauseTemplate {
	var values []any
	sql := build(p, alias, func(v any) int {
		// A slice is copied so a caller that later reuses its backing array
		// cannot change what the memo binds for the next caller.
		if teams, ok := v.([]ids.UUID); ok {
			v = slices.Clone(teams)
		}
		values = append(values, v)
		return templateBase + len(values)
	})
	return clauseTemplate{sql: sql, values: values}
}

// placeholdersMap reports whether every number past templateBase in the text
// is one the builder handed out, so bind never indexes past its values.
func (t clauseTemplate) placeholdersMap() bool {
	mapped := true
	t.walk(func(string) {}, func(slot int) {
		mapped = mapped && slot < len(t.values)
	})
	return mapped
}

// bind asks arg for every value in the builder's order. The caller's argument
// list then grows as a direct build would grow it. bind writes the caller's
// positions over the template's.
func (t clauseTemplate) bind(arg func(any) int) string {
	positions := make([]int, len(t.values))
	for i, v := range t.values {
		positions[i] = arg(v)
	}
	var out strings.Builder
	out.Grow(len(t.sql))
	t.walk(func(literal string) { out.WriteString(literal) }, func(slot int) {
		out.WriteString(strconv.Itoa(positions[slot]))
	})
	return out.String()
}

// walk splits the template into literal text and the slot index of each
// placeholder the builder numbered, in order.
func (t clauseTemplate) walk(literal func(string), placeholder func(slot int)) {
	rest := t.sql
	for {
		at := strings.IndexByte(rest, '$')
		if at < 0 {
			literal(rest)
			return
		}
		literal(rest[:at+1])
		rest = rest[at+1:]
		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		if n, err := strconv.Atoi(rest[:digits]); err == nil && n > templateBase {
			placeholder(n - templateBase - 1)
		} else {
			literal(rest[:digits])
		}
		rest = rest[digits:]
	}
}
