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
	clauses := strings.Split(authComment.ReplaceAllString(value, " "), ";")
	server := strings.Fields(clauses[0])
	if len(server) == 0 || strings.Contains(server[0], "=") {
		return connector.SenderAuthentication{}
	}
	out := connector.SenderAuthentication{Server: strings.ToLower(server[0])}
	for _, clause := range clauses[1:] {
		tokens := strings.Fields(strings.ToLower(clause))
		if len(tokens) == 0 || tokens[0] != "dmarc=pass" {
			continue
		}
		for _, token := range tokens[1:] {
			if domain, ok := strings.CutPrefix(token, "header.from="); ok {
				out.DMARCPassDomain = domain
			}
		}
	}
	return out
}
