// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aitest

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// nameFields are the fields a request shows a record's name under.
var nameFields = []string{"label", "name", "title", "display_name"}

// RefWhere is the short ref a request's JSON shows for the record whose field
// holds value, read off the request the way a model reads it. A request names
// records by refs and never by id, so a scripted reply that cites a record a
// test seeded names it by this. False when the request shows no such record.
func RefWhere(req model.Request, field, value string) (string, bool) {
	for _, message := range req.Messages {
		for _, payload := range jsonIn(message.Content) {
			if ref, ok := refIn(payload, field, value); ok {
				return ref, true
			}
		}
	}
	return "", false
}

// RefBeside is RefWhere for the record whose name, label, title or display
// name is name.
func RefBeside(req model.Request, name string) (string, bool) {
	for _, field := range nameFields {
		if ref, ok := RefWhere(req, field, name); ok {
			return ref, true
		}
	}
	return "", false
}

// jsonIn is every JSON object text carries, in order, past the fence and the
// prose around it.
func jsonIn(text string) []any {
	var out []any
	for at := strings.IndexByte(text, '{'); at >= 0; {
		decoder := json.NewDecoder(strings.NewReader(text[at:]))
		var value any
		next := at + 1
		if decoder.Decode(&value) == nil {
			out = append(out, value)
			next = at + int(decoder.InputOffset())
		}
		found := strings.IndexByte(text[next:], '{')
		if found < 0 {
			break
		}
		at = next + found
	}
	return out
}

func refIn(node any, field, value string) (string, bool) {
	switch v := node.(type) {
	case map[string]any:
		if v[field] == value {
			ref, ok := v["id"].(string)
			return ref, ok
		}
		for _, child := range v {
			if ref, ok := refIn(child, field, value); ok {
				return ref, true
			}
		}
	case []any:
		for _, child := range v {
			if ref, ok := refIn(child, field, value); ok {
				return ref, true
			}
		}
	}
	return "", false
}

// RefBefore is the ref of the record whose rendering holds text: the last
// ref the request shows as an "id" before text first appears. For a record
// that has no name of its own, such as a thread named by what was said on it.
func RefBefore(req model.Request, text string) (string, bool) {
	var shown strings.Builder
	for _, message := range req.Messages {
		shown.WriteString(message.Content)
	}
	at := strings.Index(shown.String(), text)
	if at < 0 {
		return "", false
	}
	found := shownID.FindAllStringSubmatch(shown.String()[:at], -1)
	if len(found) == 0 {
		return "", false
	}
	return found[len(found)-1][1], true
}

var shownID = regexp.MustCompile(`"id":"([a-z]+[0-9]+)"`)

// Cite is reply as a model answering req writes it: each record id in names
// spelled as the ref req shows beside that record's name. An id whose record
// req does not show stays as written, as a model inventing one would write it.
func Cite(req model.Request, reply string, names map[string]string) string {
	for id, name := range names {
		if ref, ok := RefBeside(req, name); ok {
			reply = strings.ReplaceAll(reply, id, ref)
		}
	}
	return reply
}
