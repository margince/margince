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

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

//go:embed columns.txt
var columnPolicies string

var qualifiedColumn = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*)$`)

var limits = map[string]int{
	"label":    values.TextLabelCharacters,
	"line":     values.TextLineCharacters,
	"note":     values.TextNoteCharacters,
	"document": values.TextDocumentCharacters,
}

type Policy struct {
	Limit     int
	Exception string
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
		if fields[1] == "exempt" && len(fields) > 2 {
			policy.Exception = strings.Join(fields[2:], " ")
		} else if policy.Limit == 0 || len(fields) != 2 {
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
