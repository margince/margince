// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// goQuotedCodes are the templates that wrote %q before they took each
// language's own marks. Text stored then still binds to them; no other
// template ever had that spelling.
var goQuotedCodes = map[string]bool{
	"agent.accountSendFiled": true, "agent.booking": true, "agent.mergeMembers": true, "agent.mergeTags": true,
	"agent.sendMessage": true, "agent.sendSubject": true, "approval.draftedReplyWaiting": true,
	"approval.heldNotSent": true, "approval.taskDueAsked": true, "commitment.explanation": true,
	"commitment.taken_over": true, "deals.draftFollowUp": true,
}

// goQuoting turns each language's quoted value back into Go's %q.
var goQuoting = func() *strings.Replacer {
	var pairs []string
	for _, lang := range textlang.Shipped {
		pairs = append(pairs, langcopy.Quoted.In(lang), "%q")
	}
	return strings.NewReplacer(pairs...)
}()

func (c Component) renderBounded(lang textlang.Lang, original string, bounded bool) (string, error) {
	rendered, err := c.renderings(reading{lang: lang, original: original, bounded: bounded}, 0)
	if err != nil {
		return "", err
	}
	return rendered[0], nil
}

// reading is a way to write a sentence: in today's words, or asStored in every
// stored wording that original holds. A part stands verbatim in its parent, so
// a part absent from original cannot make it up.
type reading struct {
	lang     textlang.Lang
	original string
	bounded  bool
	asStored bool
}

// storedWordings are the words in lang a stored row may hold, today's first.
func storedWordings(definition Definition, lang textlang.Lang) []string {
	return append([]string{definition.Words.In(lang)}, definition.Retired[lang]...)
}

// wordings are the templates code is written in. Each component reads its own,
// since codes are reworded in different releases.
func (r reading) wordings(code string, definition Definition) []string {
	if !r.asStored {
		return []string{definition.Words.In(r.lang)}
	}
	words := storedWordings(definition, r.lang)
	if goQuotedCodes[code] {
		for _, plain := range words {
			words = append(words, goQuoting.Replace(plain))
		}
	}
	return words
}

func (r reading) keep(kept []string, rendered string) []string {
	if r.asStored && (!strings.Contains(r.original, rendered) || slices.Contains(kept, rendered)) {
		return kept
	}
	return append(kept, rendered)
}

// renderings writes the sentence in each wording the reading allows, and in
// one when reading today's words.
func (c Component) renderings(r reading, depth int) ([]string, error) {
	if len(c.Following) > 8 || depth > 4 || len(c.Args) > 16 {
		return nil, errors.New("system text: sentence sequence exceeds limit")
	}

	definition, ok := definitionOf(c.Code)
	if !ok || c.Origin != Template || len(c.Args) != len(definition.Arguments) {
		return nil, errors.New("system text: invalid definition")
	}
	values := make([][]any, len(c.Args))
	for i, arg := range c.Args {
		if !argumentKindMatches(arg.Kind, definition.Arguments[i]) {
			return nil, errors.New("system text: invalid argument")
		}
		if arg.Kind == argumentCount {
			values[i] = []any{arg.Count}
			continue
		}
		texts, err := arg.renderings(r, depth)
		if err != nil {
			return nil, err
		}
		for _, text := range texts {
			values[i] = append(values[i], text)
		}
	}

	var sentences []string
	for _, words := range r.wordings(c.Code, definition) {
		for chosen := range combinations(values) {
			sentence := words
			if len(chosen) > 0 {
				sentence = fmt.Sprintf(words, chosen...)
			}
			sentences = r.keep(sentences, sentence)
		}
	}
	lines := [][]string{sentences}
	for _, part := range c.Following {
		rendered, err := part.renderings(r, depth+1)
		if err != nil {
			return nil, err
		}
		lines = append(lines, rendered)
	}
	separator, err := layoutSeparator(c.Layout)
	if err != nil {
		return nil, err
	}
	var out []string
	for chosen := range combinations(lines) {
		out = r.keep(out, strings.Join(chosen, separator))
	}
	return out, nil
}

// combinations yields each way of taking one option from every list, reusing
// the slice it yields.
func combinations[T any](options [][]T) iter.Seq[[]T] {
	return func(yield func([]T) bool) {
		chosen := make([]T, len(options))
		var fill func(int) bool
		fill = func(at int) bool {
			if at == len(options) {
				return yield(chosen)
			}
			for _, option := range options[at] {
				chosen[at] = option
				if !fill(at + 1) {
					return false
				}
			}
			return true
		}
		fill(0)
	}
}

func (arg Argument) renderings(r reading, depth int) ([]string, error) {
	if r.bounded && (arg.Kind == argumentText || arg.Kind == argumentLabel) && utf8.RuneCountInString(arg.Text) > 512 {
		return nil, ErrDisplayLimit
	}
	switch arg.Kind {
	case argumentComponent:
		if arg.Component == nil {
			return nil, errors.New("system text: missing nested component")
		}
		return arg.Component.renderings(r, depth+1)
	case argumentSourceSpan:
		return single(arg.sourceSpan(r.original))
	case argumentMessage:
		nested, ok := definitionOf(arg.Text)
		if !ok || len(nested.Arguments) != 0 {
			return nil, errors.New("system text: invalid nested message")
		}
		var kept []string
		for _, words := range r.wordings(arg.Text, nested) {
			kept = r.keep(kept, words)
		}
		return kept, nil
	case argumentDate:
		return single(calendarText(arg.Text))
	case argumentText, argumentLabel:
		return []string{credentialmask.MaskText(arg.Text)}, nil
	default:
		return nil, errors.New("system text: invalid argument kind")
	}
}

func single(text string, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	return []string{text}, nil
}
