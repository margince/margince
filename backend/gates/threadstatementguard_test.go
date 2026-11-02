// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"slices"
	"strings"
)

type threadQueryRegion struct {
	start, end int
	negative   bool
}

func threadStatementGuarded(statement string) bool {
	return threadStatementUnguardedAlias(statement) == ""
}

func threadStatementUnguardedAlias(statement string) string {
	tokens := threadStatementTokens(statement)
	regions := []threadQueryRegion{{start: 0, end: len(tokens)}}
	for i, token := range tokens {
		if token == "(" && i+1 < len(tokens) && (tokens[i+1] == "select" || tokens[i+1] == "with") {
			negative := threadNestedNegative(tokens, i)
			end := matchingParen(tokens, i)
			regions = append(regions, threadQueryRegion{start: i + 1, end: end, negative: negative})
		}
	}
	for _, at := range threadActivityPositions(tokens) {
		alias := "activity"
		next := at + 1
		if next < len(tokens) && tokens[next] == "as" {
			next++
		}
		if next < len(tokens) && isName(tokens[next]) && !slices.Contains(clauseWords, tokens[next]) {
			alias = strings.Trim(tokens[next], `"`)
		}
		region := regions[0]
		for _, candidate := range regions {
			if at >= candidate.start && at < candidate.end && candidate.end-candidate.start < region.end-region.start {
				region = candidate
			}
		}
		region = threadRegionArm(tokens, region, at)
		if !threadRegionBoundary(tokens, regions, region, alias, map[string]bool{}) {
			return alias
		}
	}
	return ""
}

func threadRegionGuards(tokens []string, regions []threadQueryRegion, region threadQueryRegion, alias string) bool {
	var clauses [][]string
	for i := region.start; i < region.end; i++ {
		if tokens[i] == "(" {
			i = matchingParen(tokens, i)
			continue
		}
		if tokens[i] != "where" && tokens[i] != "on" {
			continue
		}
		if tokens[i] == "on" && !threadJoinCanGuard(tokens, region, i, alias) {
			continue
		}
		start, end := i+1, region.end
		depth := 0
		for j := start; j < region.end; j++ {
			if tokens[j] == "(" {
				depth++
			}
			if tokens[j] == ")" {
				depth--
			}
			if depth == 0 && slices.Contains([]string{"where", "on", "for", "offset", "fetch", "window", "group", "order", "limit", "having", "returning", "union", "except", "intersect", "join", "left", "right", "full", "inner", "cross"}, tokens[j]) {
				end = j
				break
			}
		}
		clauses = append(clauses, tokens[start:end])
		i = end - 1
	}
	for _, clause := range clauses {
		if !region.negative && threadBoolean(clause, alias) == 1 {
			return true
		}
		if region.negative {
			for _, factor := range threadBooleanParts(clause, "and") {
				if threadContainsGuard(factor, alias) && threadBoolean(factor, alias) == 2 {
					return true
				}
			}
		}
	}
	return false
}

func threadBoolean(tokens []string, alias string) uint8 {
	tokens = threadUnwrap(tokens)
	for _, operator := range []string{"or", "and"} {
		parts := threadBooleanParts(tokens, operator)
		if len(parts) > 1 {
			result := threadBoolean(parts[0], alias)
			for _, part := range parts[1:] {
				result = threadBooleanCombine(result, threadBoolean(part, alias), operator)
			}
			return result
		}
	}
	if len(tokens) > 0 && tokens[0] == "not" {
		value := threadBoolean(tokens[1:], alias)
		return (value&1)<<1 | (value&2)>>1
	}
	if len(tokens) == 4 && tokens[0] == "activity_thread_available" && tokens[1] == "(" && threadGuardAlias(tokens[2], alias) && tokens[3] == ")" {
		return 1
	}
	if len(tokens) == 1 && tokens[0] == "true" {
		return 2
	}
	if len(tokens) == 1 && tokens[0] == "false" {
		return 1
	}
	return 3
}

func threadUnwrap(tokens []string) []string {
	for len(tokens) > 1 && tokens[0] == "(" && matchingParen(tokens, 0) == len(tokens)-1 {
		tokens = tokens[1 : len(tokens)-1]
	}
	return tokens
}

func threadBooleanParts(tokens []string, operator string) [][]string {
	tokens = threadUnwrap(tokens)
	depth, start, cases, between := 0, 0, 0, false
	var parts [][]string
	for i, token := range tokens {
		if token == "(" {
			depth++
		}
		if token == ")" {
			depth--
		}
		if token == "case" {
			cases++
		}
		if token == "end" && cases > 0 {
			cases--
		}
		if depth == 0 && cases == 0 && token == "between" {
			between = true
		}
		if depth == 0 && cases == 0 && token == "and" && between {
			between = false
			continue
		}
		if depth == 0 && cases == 0 && token == operator {
			parts = append(parts, tokens[start:i])
			start = i + 1
		}
	}
	return append(parts, tokens[start:])
}

func threadBooleanCombine(left, right uint8, operator string) uint8 {
	var result uint8
	for a := range 2 {
		for b := range 2 {
			if left&(1<<a) == 0 || right&(1<<b) == 0 {
				continue
			}
			truth := a == 1 && b == 1
			if operator == "or" {
				truth = a == 1 || b == 1
			}
			if truth {
				result |= 2
			} else {
				result |= 1
			}
		}
	}
	return result
}

func threadContainsGuard(tokens []string, alias string) bool {
	for i := 0; i+3 < len(tokens); i++ {
		if tokens[i] == "activity_thread_available" && tokens[i+1] == "(" && threadGuardAlias(tokens[i+2], alias) && tokens[i+3] == ")" {
			return true
		}
	}
	return false
}

func threadGuardAlias(column, alias string) bool {
	return column == alias+".thread_key" || (alias == "activity" && column == "thread_key")
}
func threadRegionArm(tokens []string, region threadQueryRegion, at int) threadQueryRegion {
	depth := 0
	for i := region.start; i < region.end; i++ {
		if tokens[i] == "(" {
			depth++
		}
		if tokens[i] == ")" {
			depth--
		}
		if depth == 0 && slices.Contains([]string{"union", "except", "intersect"}, tokens[i]) {
			if i < at {
				region.start = i + 1
			} else {
				region.end = i
				break
			}
		}
	}
	return region
}

func threadNestedNegative(tokens []string, at int) bool {
	var stack []int
	for index := 0; index <= at; index++ {
		switch tokens[index] {
		case "(":
			stack = append(stack, index)
		case ")":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	negative := false
	for _, open := range stack {
		if open > 0 && tokens[open-1] == "not" || open > 1 && tokens[open-1] == "exists" && tokens[open-2] == "not" {
			negative = !negative
		}
	}
	return negative
}

func threadJoinCanGuard(tokens []string, region threadQueryRegion, on int, alias string) bool {
	depth, join := 0, -1
	for at := on - 1; at >= region.start; at-- {
		if tokens[at] == ")" {
			depth++
		}
		if tokens[at] == "(" {
			depth--
		}
		if depth == 0 && tokens[at] == "join" {
			join = at
			break
		}
		if depth == 0 && tokens[at] == "where" {
			break
		}
	}
	if join < 0 {
		return false
	}
	kind := "inner"
	before := join - 1
	if before >= 0 && tokens[before] == "outer" {
		before--
	}
	if before >= 0 && slices.Contains([]string{"left", "right", "full"}, tokens[before]) {
		kind = tokens[before]
	}
	if kind == "inner" {
		return true
	}
	if alias == "__identity" || kind == "full" {
		return false
	}
	table := join + 1
	for table < len(tokens) && slices.Contains(tableQualifiers, tokens[table]) {
		table++
	}
	if table >= len(tokens) {
		return false
	}
	name := ""
	if tokens[table] == "(" {
		at := matchingParen(tokens, table) + 1
		if at < len(tokens) && tokens[at] == "as" {
			at++
		}
		if at < len(tokens) {
			name = tokens[at]
		}
	} else {
		name = threadTableAlias(tokens, table)
	}
	if kind == "left" {
		return alias == name
	}
	return alias != name
}

func threadStatementTokens(statement string) []string {
	tokens := sqlToken.FindAllString(strings.ToLower(stripNoise(statement)), -1)
	for i, token := range tokens {
		if isName(token) {
			tokens[i] = strings.Join(identParts(token), ".")
		}
	}
	return tokens
}

func threadActivityPositions(tokens []string) []int {
	var positions []int
	for i, token := range tokens {
		if !slices.Contains([]string{"from", "join", "using", "update"}, token) {
			continue
		}
		for _, at := range tableTargets(tokens, i) {
			if at >= len(tokens) {
				continue
			}
			parts := strings.Split(tokens[at], ".")
			if parts[len(parts)-1] == "activity" {
				positions = append(positions, at)
			}
		}
	}
	return positions
}
