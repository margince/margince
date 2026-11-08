// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aitest

import (
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// RefBeside is the short ref a request shows for the record whose name,
// label or title is name, read off the request the way a model reads it.
// A request names records by refs and never by id, so a scripted reply that
// cites a record names it by this. False when the request shows no such
// record.
func RefBeside(req model.Request, name string) (string, bool) {
	shown := regexp.MustCompile(`"id":"([a-z]+[0-9]+)"[^{}]*?"(?:label|name|title|display_name)":"` + regexp.QuoteMeta(name) + `"`)
	var text strings.Builder
	for _, message := range req.Messages {
		text.WriteString(message.Content)
	}
	found := shown.FindStringSubmatch(text.String())
	if found == nil {
		return "", false
	}
	return found[1], true
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
