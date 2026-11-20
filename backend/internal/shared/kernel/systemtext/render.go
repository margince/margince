// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// spelling is how a template quotes an argument. Text stored before the
// templates took each language's own marks was written with Go's %q.
type spelling int

const (
	ownMarks spelling = iota
	goQuoted
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
	return c.renderSpelled(lang, 0, original, bounded, ownMarks)
}

// wordsIn is the template in one language, in the spelling asked for.
func wordsIn(code string, definition Definition, lang textlang.Lang, spell spelling) string {
	words := definition.Words.In(lang)
	if spell == goQuoted && goQuotedCodes[code] {
		return goQuoting.Replace(words)
	}
	return words
}

// renderSpelled writes the sentence in the spelling asked for, so text stored
// with Go's quoting still binds.
func (c Component) renderSpelled(lang textlang.Lang, depth int, original string, bounded bool, spell spelling) (string, error) {
	if len(c.Following) > 8 || depth > 4 || len(c.Args) > 16 {
		return "", errors.New("system text: sentence sequence exceeds limit")
	}

	definition, ok := definitionOf(c.Code)
	if !ok || c.Origin != Template || len(c.Args) != len(definition.Arguments) {
		return "", errors.New("system text: invalid definition")
	}
	values := make([]any, len(c.Args))
	for i, arg := range c.Args {
		if !argumentKindMatches(arg.Kind, definition.Arguments[i]) {
			return "", errors.New("system text: invalid argument")
		}
		if arg.Kind == argumentCount {
			values[i] = arg.Count
			continue
		}
		value, err := arg.renderText(lang, depth, original, bounded, spell)
		if err != nil {
			return "", err
		}
		values[i] = value
	}

	sentence := wordsIn(c.Code, definition, lang, spell)
	if len(values) > 0 {
		sentence = fmt.Sprintf(sentence, values...)
	}
	lines := []string{sentence}
	for _, part := range c.Following {
		line, err := part.renderSpelled(lang, depth+1, original, bounded, spell)
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	separator, err := layoutSeparator(c.Layout)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, separator), nil
}

func (arg Argument) renderText(lang textlang.Lang, depth int, original string, bounded bool, spell spelling) (string, error) {
	if bounded && (arg.Kind == argumentText || arg.Kind == argumentLabel) && utf8.RuneCountInString(arg.Text) > 512 {
		return "", ErrDisplayLimit
	}
	switch arg.Kind {
	case argumentComponent:
		if arg.Component == nil {
			return "", errors.New("system text: missing nested component")
		}
		return arg.Component.renderSpelled(lang, depth+1, original, bounded, spell)
	case argumentSourceSpan:
		return arg.sourceSpan(original)
	case argumentMessage:
		nested, ok := definitionOf(arg.Text)
		if !ok || len(nested.Arguments) != 0 {
			return "", errors.New("system text: invalid nested message")
		}
		return nested.Words.In(lang), nil
	case argumentDate:
		return calendarText(arg.Text)
	case argumentText, argumentLabel:
		return credentialmask.MaskText(arg.Text), nil
	default:
		return "", errors.New("system text: invalid argument kind")
	}
}
