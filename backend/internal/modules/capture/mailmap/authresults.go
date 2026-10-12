// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"regexp"
	"strings"

	"github.com/emersion/go-message/mail"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// authComment is an RFC 8601 comment. A comment is the server's free text, so
// nothing inside one is read as a result.
var authComment = regexp.MustCompile(`\([^()]*\)`)

// authQuoted is a quoted string. The server copies sender-chosen values into
// its result, smtp.mailfrom among them, and a quoted local part may carry a
// ";" and a whole forged clause. It is removed before the clauses are split.
var authQuoted = regexp.MustCompile(`"[^"]*"`)

// TopSenderAuthentication reads the topmost Authentication-Results header: the
// one the final receiving server wrote. A receiving server adds its headers
// above everything the sender submitted, so a forged result sits below it.
// Whether the server is one to believe is the caller's question; this answers
// what the header says.
func TopSenderAuthentication(header mail.Header) connector.SenderAuthentication {
	fields := header.Fields()
	for fields.Next() {
		if strings.EqualFold(fields.Key(), "Authentication-Results") {
			return parseAuthenticationResults(fields.Value())
		}
	}
	return connector.SenderAuthentication{}
}

func parseAuthenticationResults(value string) connector.SenderAuthentication {
	value = authComment.ReplaceAllString(authQuoted.ReplaceAllString(value, " "), " ")
	if strings.ContainsAny(value, `"()`) {
		// Quoting or nesting this reading cannot follow: no clause is trusted.
		return connector.SenderAuthentication{}
	}
	clauses := strings.Split(value, ";")
	server := strings.Fields(clauses[0])
	if len(server) == 0 || strings.Contains(server[0], "=") {
		return connector.SenderAuthentication{}
	}
	out := connector.SenderAuthentication{Server: strings.ToLower(server[0])}
	var verdicts [][]string
	for _, clause := range clauses[1:] {
		if tokens := strings.Fields(strings.ToLower(clause)); len(tokens) > 0 && strings.HasPrefix(tokens[0], "dmarc=") {
			verdicts = append(verdicts, tokens)
		}
	}
	// A server writes one dmarc result. Two mean this reading split the header
	// somewhere the server did not, so neither is believed.
	if len(verdicts) != 1 || verdicts[0][0] != "dmarc=pass" {
		return out
	}
	for _, token := range verdicts[0][1:] {
		if domain, ok := strings.CutPrefix(token, "header.from="); ok {
			out.DMARCPassDomain = domain
		}
	}
	return out
}
