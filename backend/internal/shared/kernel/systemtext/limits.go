// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Label permits a catalog label where a template expects text.
func Label(value string) Argument { return Argument{Kind: argumentLabel, Text: value} }

// Fit bounds display metadata without truncating a template’s argument binding.
func Fit(message *Message, subject, body string, subjectLimit, bodyLimit int) (*Message, string, string, error) {
	if message == nil {
		return nil, subject, body, nil
	}
	out := &Message{Version: message.Version}
	var err error
	out.Subject, subject, err = fitComponent(message.Subject, subject, subjectLimit)
	if err != nil {
		return nil, "", "", err
	}
	out.Body, body, err = fitComponent(message.Body, body, bodyLimit)
	if err != nil {
		return nil, "", "", err
	}
	return out, subject, body, nil
}

func fitComponent(source *Component, original string, limit int) (*Component, string, error) {
	if source == nil || source.Origin != Template || limit <= 0 {
		return source, original, nil
	}
	if source.Digest != "" && !source.Matches(original) {
		return nil, original, nil
	}
	component := cloneComponent(source)
	language, matched, err := canonicalLanguage(component, original)
	if err != nil {
		return nil, "", err
	}
	if !matched {
		return nil, original, nil
	}
	for {
		fits, err := fitsLanguages(component, original, limit)
		if err != nil {
			return nil, "", err
		}
		if fits {
			canonical, err := component.renderBounded(language, 0, original, true)
			return component, canonical, err
		}
		label := longestLabel(component)
		if label == nil || utf8.RuneCountInString(label.Text) <= 1 {
			return nil, "", ErrDisplayLimit
		}
		runes := []rune(strings.TrimSuffix(label.Text, "…"))
		if len(runes) <= 1 {
			return nil, "", ErrDisplayLimit
		}
		label.Text = textcut.Runes(string(runes), len(runes)-1) + "…"
	}
}

func canonicalLanguage(component *Component, original string) (textlang.Lang, bool, error) {
	for _, language := range textlang.Shipped {
		rendered, err := component.renderBounded(language, 0, original, false)
		if err != nil {
			return textlang.Unknown, false, err
		}
		if rendered == original {
			return language, true, nil
		}
	}
	return textlang.Unknown, false, nil
}

func fitsLanguages(component *Component, original string, limit int) (bool, error) {
	fits := true
	for _, language := range textlang.Shipped {
		rendered, err := component.renderBounded(language, 0, original, true)
		if errors.Is(err, ErrDisplayLimit) {
			fits = false
			continue
		}
		if err != nil {
			return false, err
		}
		fits = fits && utf8.RuneCountInString(rendered) <= limit
	}
	return fits, nil
}

func longestLabel(component *Component) *Argument {
	var longest *Argument
	for i := range component.Args {
		argument := &component.Args[i]
		if argument.Kind == argumentLabel && (longest == nil || utf8.RuneCountInString(argument.Text) > utf8.RuneCountInString(longest.Text)) {
			longest = argument
		}
	}
	for i := range component.Args {
		if component.Args[i].Component == nil {
			continue
		}
		label := longestLabel(component.Args[i].Component)
		if label != nil && (longest == nil || utf8.RuneCountInString(label.Text) > utf8.RuneCountInString(longest.Text)) {
			longest = label
		}
	}
	for i := range component.Following {
		label := longestLabel(&component.Following[i])
		if label != nil && (longest == nil || utf8.RuneCountInString(label.Text) > utf8.RuneCountInString(longest.Text)) {
			longest = label
		}
	}
	return longest
}
