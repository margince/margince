// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package textbounds holds the length policy of every text column and builds
// the database guards and frontend limits from it.
package textbounds

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

//go:embed columns.txt
var columnPolicies string

var qualifiedColumn = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)$`)

// checkedMarker opens the note on a bounded line, so a stray word after the
// limit stays an error instead of reading as a note.
const checkedMarker = "checked:"

var limits = map[string]int{
	"label":    values.TextLabelCharacters,
	"line":     values.TextLineCharacters,
	"note":     values.TextNoteCharacters,
	"document": values.TextDocumentCharacters,
}

// A bounded column may carry Checked: what its writers were read for, so the
// limit records a decision and not only a default.
type Policy struct {
	Limit     int
	Exception string
	Checked   string
}

type Column struct {
	Key, Table, Name string
	Array, Generated bool
}

func Policies() (map[string]Policy, error) { return ParsePolicies(strings.NewReader(columnPolicies)) }

// embeddedPolicies panics on a malformed columns.txt, which the package tests refuse
// before any build ships it.
var embeddedPolicies = sync.OnceValue(func() map[string]Policy {
	policies, err := Policies()
	if err != nil {
		panic(err)
	}
	return policies
})

// LimitOf is the character limit of a column such as public.contact.title, or 0
// when the column is exempt or has no policy.
func LimitOf(column string) int { return embeddedPolicies()[column].Limit }

// Fit cuts text a machine wrote to the limit of its column, so a write nobody
// can shorten lands cut instead of failing. An exempt column keeps it whole.
func Fit(column, text string) string {
	if limit := LimitOf(column); limit > 0 {
		return textcut.Runes(text, limit)
	}
	return text
}

func ParsePolicies(input io.Reader) (map[string]Policy, error) {
	policies := map[string]Policy{}
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !qualifiedColumn.MatchString(fields[0]) {
			return nil, fmt.Errorf("invalid text policy: %s", line)
		}
		if _, exists := policies[fields[0]]; exists {
			return nil, fmt.Errorf("duplicate text policy: %s", fields[0])
		}
		policy := Policy{Limit: limits[fields[1]]}
		switch {
		case fields[1] == "exempt" && len(fields) > 2:
			policy.Exception = strings.Join(fields[2:], " ")
		case policy.Limit != 0 && len(fields) > 3 && fields[2] == checkedMarker:
			policy.Checked = strings.Join(fields[3:], " ")
		case policy.Limit == 0 || len(fields) != 2:
			return nil, fmt.Errorf("invalid text limit: %s", line)
		}
		policies[fields[0]] = policy
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read text policies: %w", err)
	}
	return policies, nil
}

func Columns(input io.Reader) ([]Column, error) {
	var columns []Column
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 65536), 4*1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 || (fields[1] != "text" && fields[1] != "text[]") {
			continue
		}
		match := qualifiedColumn.FindStringSubmatch(fields[0])
		if match == nil {
			continue
		}
		generated := false
		for _, field := range fields[2:] {
			if strings.HasPrefix(field, "gen=") {
				generated = field != "gen=-"
			}
		}
		columns = append(columns, Column{Key: fields[0], Table: match[1], Name: match[2], Array: fields[1] == "text[]", Generated: generated})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read text columns: %w", err)
	}
	if len(columns) == 0 {
		return nil, errors.New("the catalog contains no text columns")
	}
	return columns, nil
}
