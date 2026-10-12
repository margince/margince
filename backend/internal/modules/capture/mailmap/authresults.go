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

// authQuoted is a quoted string, escaped characters included. The server
// copies sender-chosen values into its result, smtp.mailfrom among them, and a
// quoted local part may carry a ";" and a whole forged clause. It is removed
// before the clauses are split.
var authQuoted = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

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
	// Every dmarc clause, in each spelling RFC 8601 allows ("dmarc=", "dmarc =",
	// "dmarc/1="). A server writes one; two mean this reading split the header
	// somewhere the server did not, so neither is believed.
	var verdicts [][]string
	for _, clause := range clauses[1:] {
		name, result, found := strings.Cut(clause, "=")
		method, version, versioned := strings.Cut(name, "/")
		if !found || !strings.EqualFold(strings.TrimSpace(method), "dmarc") {
			continue
		}
		verdict := strings.Fields(strings.ToLower(result))
		// Version 1 is the only DMARC this reads; another version's pass is
		// not one, though its clause still counts against a second result.
		if versioned && strings.TrimSpace(version) != "1" {
			verdict = []string{"unsupported"}
		}
		verdicts = append(verdicts, verdict)
	}
	if len(verdicts) != 1 || len(verdicts[0]) == 0 || verdicts[0][0] != "pass" {
		return out
	}
	// One header.from: a pass naming two domains says nothing about either.
	var domains []string
	for _, token := range verdicts[0][1:] {
		if domain, ok := strings.CutPrefix(token, "header.from="); ok {
			domains = append(domains, domain)
		}
	}
	if len(domains) == 1 {
		out.DMARCPassDomain = domains[0]
	}
	return out
}
