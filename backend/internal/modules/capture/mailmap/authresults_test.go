// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The shape Gmail writes, folded across lines, comes first; the rest are the
// ways a header could claim more than the receiving server said.
func TestOnlyTheTopmostAuthenticationResultIsRead(t *testing.T) {
	gmail := []string{
		"Authentication-Results: mx.google.com;",
		"       dkim=pass header.i=@acme.example header.s=google header.b=abc;",
		"       spf=pass (google.com: domain of sam@acme.example designates 1.2.3.4 as permitted sender) smtp.mailfrom=sam@acme.example;",
		"       dmarc=pass (p=QUARANTINE sp=QUARANTINE dis=NONE) header.from=acme.example",
	}
	for _, tc := range []struct {
		name  string
		lines []string
		want  connector.SenderAuthentication
	}{
		{
			name:  "Gmail's own result",
			lines: append(append([]string{"Received: by 2002:a05"}, gmail...), "From: sam@acme.example"),
			want:  connector.SenderAuthentication{Server: "mx.google.com", DMARCPassDomain: "acme.example"},
		},
		{
			name: "a sender's forged result below the server's",
			lines: []string{
				"Authentication-Results: mx.google.com; dmarc=fail header.from=acme.example",
				"Authentication-Results: mx.google.com; dmarc=pass header.from=acme.example",
				"From: sam@acme.example",
			},
			want: connector.SenderAuthentication{Server: "mx.google.com"},
		},
		{
			name:  "DMARC passed for another domain",
			lines: []string{"Authentication-Results: mx.google.com; dmarc=pass header.from=elsewhere.example"},
			want:  connector.SenderAuthentication{Server: "mx.google.com", DMARCPassDomain: "elsewhere.example"},
		},
		{
			name:  "a pass named only inside a comment",
			lines: []string{"Authentication-Results: mx.google.com; dmarc=none (dmarc=pass header.from=acme.example)"},
			want:  connector.SenderAuthentication{Server: "mx.google.com"},
		},
		{
			name:  "no server name, as Microsoft writes it",
			lines: []string{"Authentication-Results: spf=pass smtp.mailfrom=acme.example; dmarc=pass action=none header.from=acme.example"},
			want:  connector.SenderAuthentication{},
		},
		{
			name: "a forged pass smuggled in a quoted envelope sender",
			lines: []string{
				`Authentication-Results: mx.google.com; spf=pass smtp.mailfrom="x; dmarc=pass header.from=acme.example y"@evil.example;`,
				"       dmarc=fail header.from=acme.example",
			},
			want: connector.SenderAuthentication{Server: "mx.google.com"},
		},
		{
			name:  "two dmarc results",
			lines: []string{"Authentication-Results: mx.google.com; dmarc=pass header.from=acme.example; dmarc=fail header.from=acme.example"},
			want:  connector.SenderAuthentication{Server: "mx.google.com"},
		},
		{
			name:  "a second dmarc result spelled with spaces or a version",
			lines: []string{"Authentication-Results: mx.google.com; dmarc=pass header.from=acme.example; dmarc/1 = fail header.from=acme.example"},
			want:  connector.SenderAuthentication{Server: "mx.google.com"},
		},
		{
			name:  "a pass under a DMARC version this does not read",
			lines: []string{"Authentication-Results: mx.google.com; dmarc/2=pass header.from=acme.example"},
			want:  connector.SenderAuthentication{Server: "mx.google.com"},
		},
		{
			name:  "a pass under version 1, spelled out",
			lines: []string{"Authentication-Results: mx.google.com; dmarc/1=pass header.from=acme.example"},
			want:  connector.SenderAuthentication{Server: "mx.google.com", DMARCPassDomain: "acme.example"},
		},
		{
			name:  "a result that only starts with pass",
			lines: []string{"Authentication-Results: mx.google.com; dmarc=pass-extra header.from=acme.example"},
			want:  connector.SenderAuthentication{Server: "mx.google.com"},
		},
		{
			name:  "a property that only ends in header.from",
			lines: []string{"Authentication-Results: mx.google.com; dmarc=pass xheader.from=acme.example"},
			want:  connector.SenderAuthentication{Server: "mx.google.com"},
		},
		{
			name:  "a quote this reading cannot pair",
			lines: []string{`Authentication-Results: mx.google.com; spf=pass smtp.mailfrom="x@evil.example; dmarc=pass header.from=acme.example`},
			want:  connector.SenderAuthentication{},
		},
		{
			name:  "no result at all",
			lines: []string{"From: sam@acme.example"},
			want:  connector.SenderAuthentication{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TopSenderAuthentication(headerOf(t, tc.lines...)); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
