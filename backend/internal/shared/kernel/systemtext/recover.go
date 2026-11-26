// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var recoveryVerb = regexp.MustCompile(`%(?:\[([1-9][0-9]*)\])?([sd%])`)

// RecoverCanonicalText accepts only an unambiguous catalog sentence with recoverable arguments.
//
//nolint:nilnil // An unmatched or ambiguous legacy sentence has no recoverable descriptor.
func RecoverCanonicalText(value string) (*Component, error) {
	var recovered *Component
	best, ambiguous := 0, false
	for code, definition := range catalog {
		candidate, literal, err := recoverCode(code, definition, value, best)
		if err != nil {
			return nil, err
		}
		if candidate == nil {
			continue
		}
		bound, err := Bind(&Message{Subject: candidate}, value, "")
		if errors.Is(err, ErrDisplayLimit) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if bound == nil || bound.Subject == nil {
			continue
		}
		candidate = bound.Subject
		if literal == best {
			ambiguous = true
			continue
		}
		recovered, best, ambiguous = candidate, literal, false
	}
	if ambiguous {
		return nil, nil
	}
	return recovered, nil
}

// recoverCode matches value against code's English wordings that hold no fewer
// literal bytes than best, today's first, and reports the match's literal bytes.
func recoverCode(code string, definition Definition, value string, best int) (*Component, int, error) {
	for _, template := range storedWordings(definition, textlang.English) {
		literal := len(strings.TrimSpace(recoveryVerb.ReplaceAllString(template, "")))
		if literal == 0 || literal < best {
			continue
		}
		candidate, err := recoverCanonicalComponent(code, definition, template, value)
		if err != nil || candidate != nil {
			return candidate, literal, err
		}
	}
	return nil, 0, nil
}

//nolint:nilnil // Pattern mismatches are expected nonmatches.
func recoverCanonicalComponent(code string, definition Definition, template, value string) (*Component, error) {
	verbs := recoveryVerb.FindAllStringSubmatchIndex(template, -1)
	if len(verbs) == 0 && template != value {
		return nil, nil
	}
	if !recoveryEndsMatch(template, value, verbs) {
		return nil, nil
	}
	var pattern strings.Builder
	pattern.WriteString("^")
	last, next := 0, 0
	positions := []int{}
	for _, verb := range verbs {
		pattern.WriteString(regexp.QuoteMeta(template[last:verb[0]]))
		last = verb[1]
		if template[verb[4]:verb[5]] == "%" {
			pattern.WriteString("%")
			continue
		}
		if verb[2] >= 0 {
			index, err := strconv.Atoi(template[verb[2]:verb[3]])
			if err != nil {
				return nil, fmt.Errorf("system text recovery argument: %w", err)
			}
			next = index - 1
		}
		if next >= len(definition.Arguments) {
			return nil, nil
		}
		positions = append(positions, next)
		next++
		pattern.WriteString("(.+?)")
	}
	pattern.WriteString(regexp.QuoteMeta(template[last:]))
	pattern.WriteString("$")
	match, err := regexp.Compile(pattern.String())
	if err != nil {
		return nil, fmt.Errorf("system text recovery pattern: %w", err)
	}
	parts := match.FindStringSubmatch(value)
	if parts == nil {
		return nil, nil
	}
	arguments, ok := recoveryArguments(definition, positions, parts)
	if !ok {
		return nil, nil
	}
	component := Sentence(code, arguments...)
	rendered, err := component.renderings(reading{lang: textlang.English, original: value, asStored: true}, 0)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(rendered, value) || strings.Contains(template[last:], "%") {
		return nil, nil
	}
	return component, nil
}

func recoveryEndsMatch(template, value string, verbs [][]int) bool {
	if len(verbs) == 0 {
		return true
	}
	return strings.HasPrefix(value, template[:verbs[0][0]]) && strings.HasSuffix(value, template[verbs[len(verbs)-1][1]:])
}

func recoveryArguments(definition Definition, positions []int, parts []string) ([]Argument, bool) {
	values := make([]string, len(definition.Arguments))
	for i, position := range positions {
		if values[position] != "" && values[position] != parts[i+1] {
			return nil, false
		}
		values[position] = parts[i+1]
	}
	arguments := make([]Argument, len(values))
	for i, text := range values {
		if text == "" {
			return nil, false
		}
		switch definition.Arguments[i] {
		case argumentLabel:
			arguments[i] = Label(text)
		case argumentText:
			arguments[i] = Text(text)
		case argumentCount:
			count, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return nil, false
			}
			arguments[i] = Count(count)
		default:
			return nil, false
		}
	}
	return arguments, true
}
