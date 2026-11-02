// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"slices"
	"strconv"
	"strings"
)

func threadRegionBoundary(tokens []string, regions []threadQueryRegion, region threadQueryRegion, alias string, reached map[string]bool) bool {
	key := strconv.Itoa(region.start) + ":" + strconv.Itoa(region.end) + ":" + alias
	if reached[key] {
		return false
	}
	next := cloneThreadSQLReach(reached, key)
	if threadRegionGuards(tokens, regions, region, alias) {
		return true
	}
	if threadCorrelatedBoundary(tokens, regions, region, alias) {
		return true
	}
	if region.negative {
		return false
	}
	for i := region.start; i < region.end; i++ {
		if tokens[i] != "activity" || i == 0 || !slices.Contains([]string{"from", "join", "using"}, tokens[i-1]) {
			continue
		}
		other := threadTableAlias(tokens, i)
		if other != alias && threadRegionGuards(tokens, regions, region, other) && (threadRequiredEquality(tokens[region.start:region.end], alias+".id", other+".id") || threadRequiredEquality(tokens[region.start:region.end], alias+".thread_key", other+".thread_key")) {
			return true
		}
	}
	for _, child := range regions {
		if child.start <= region.start || child.end >= region.end || child.negative {
			continue
		}
		parent := region
		for _, candidate := range regions {
			if candidate.start < child.start && candidate.end > child.end && candidate.end-candidate.start < parent.end-parent.start {
				parent = candidate
			}
		}
		if parent.start != region.start || parent.end != region.end {
			continue
		}
		source, field := threadProjectedActivityID(tokens, child)
		if source == "" || !threadRegionBoundary(tokens, regions, child, source, next) {
			continue
		}
		name := threadDerivedName(tokens, child)
		if name != "" && threadRequiredEquality(tokens[region.start:region.end], alias+".id", name+"."+field) {
			return true
		}
	}
	return false
}

func threadTableAlias(tokens []string, at int) string {
	next := at + 1
	if next < len(tokens) && tokens[next] == "as" {
		next++
	}
	if next < len(tokens) && isName(tokens[next]) && !slices.Contains(clauseWords, tokens[next]) {
		return tokens[next]
	}
	parts := strings.Split(tokens[at], ".")
	return parts[len(parts)-1]
}

func threadProjectedActivityID(tokens []string, region threadQueryRegion) (string, string) {
	for at := region.start; at+1 < region.end; at++ {
		if tokens[at] == "from" {
			break
		}
		if tokens[at+1] == "," || tokens[at+1] == "from" || tokens[at+1] == "as" {
			for i := region.start; i+1 < region.end; i++ {
				if tokens[i] != "from" && tokens[i] != "join" {
					continue
				}
				if tokens[i+1] != "activity" {
					continue
				}
				alias := threadTableAlias(tokens, i+1)
				if tokens[at] != alias+".id" && tokens[at] != alias+".*" && tokens[at] != "id" && tokens[at] != "*" {
					continue
				}
				field := "id"
				if tokens[at+1] == "as" && at+2 < region.end {
					field = tokens[at+2]
				}
				return alias, field
			}
		}
	}
	return "", ""
}

func threadDerivedName(tokens []string, region threadQueryRegion) string {
	after := region.end + 1
	if after < len(tokens) && tokens[after] == "as" {
		after++
	}
	if after < len(tokens) && isName(tokens[after]) && !slices.Contains(clauseWords, tokens[after]) {
		return tokens[after]
	}
	open := region.start - 1
	if open >= 2 && tokens[open-1] == "as" && isName(tokens[open-2]) {
		return tokens[open-2]
	}
	return ""
}

func threadRequiredEquality(tokens []string, left, right string) bool {
	rewritten := append([]string(nil), tokens...)
	found := false
	for i := 0; i+2 < len(rewritten); i++ {
		if rewritten[i+1] == "=" && ((rewritten[i] == left && rewritten[i+2] == right) || (rewritten[i] == right && rewritten[i+2] == left)) {
			replacement := []string{"activity_thread_available", "(", "__identity.thread_key", ")"}
			rewritten = append(append(append([]string(nil), rewritten[:i]...), replacement...), rewritten[i+3:]...)
			found = true
			i += 3
		}
	}
	if !found {
		return false
	}
	region := threadQueryRegion{start: 0, end: len(rewritten)}
	return threadRegionGuards(rewritten, nil, region, "__identity")
}

func threadCorrelatedBoundary(tokens []string, regions []threadQueryRegion, region threadQueryRegion, alias string) bool {
	for _, parent := range regions {
		parent = threadRegionArm(tokens, parent, region.start)
		if parent.start >= region.start || parent.end <= region.end || parent.negative {
			continue
		}
		names := map[string]bool{}
		for index := parent.start; index < parent.end; index++ {
			if strings.HasSuffix(tokens[index], ".thread_key") {
				names[strings.TrimSuffix(tokens[index], ".thread_key")] = true
			}
		}
		for other := range names {
			if other == alias || !threadRegionGuards(tokens, regions, parent, other) {
				continue
			}
			sameKey := threadRequiredEquality(tokens[region.start:region.end], alias+".thread_key", other+".thread_key")
			sameRow := threadRequiredEquality(tokens[region.start:region.end], alias+".id", other+".id")
			if sameKey || sameRow {
				return true
			}
		}
	}
	return false
}
