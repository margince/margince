// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

func TestMailTopicsSplitOnlyWithoutSharedMeaningfulWords(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	cases := []struct {
		parent, reply string
		differs       bool
	}{
		{"[ERP-27] kickoff", "Re: kickoff", false},
		{"RE: [erp-27] Kickoff", "Kickoff notes", false},
		{"Kickoff [ERP-27] notes", "Re: kickoff", false},
		{"[2026] budget", "Budget updated", false},
		{"Kickoff [ERP 27]", "AW: KICKOFF!", false},
		{"Signature requested", "Signature completed", false},
		{"Product materials", "Invoice and dinner", true},
		{"The product", "The invoice", true},
		{"Die Rechnung", "Die Lieferung", true},
		{"Angebot März", "Rechnung März", false},
		{"[ERP-27]", "Rechnung", false},
		{"", "Invoice", false},
		{"Invoice", "Re:", false},
		{"123 / !", "Invoice", false},
		{"the and", "invoice", false},
		{"Rechnung-März", "RECHNUNG April", false},
		{"Invoice", "Invoiced", true},
		{"Größe", "Grüße", true},
		{"GRÖSSE", "Größe", false},
		{"HỢP ĐỒNG", "Hợp đồng ký", false},
		{"FW: Product materials", "FW: Invoice dinner", true},
		{"SV: Product", "VS: Invoice", true},
	}
	for _, prefix := range append(mailsubject.ReplyPrefixes(), mailsubject.ForwardPrefixes()...) {
		cases = append(cases, struct {
			parent, reply string
			differs       bool
		}{prefix, "Invoice", false})
	}
	for _, tc := range cases {
		var got, reverse bool
		if err := conn.QueryRow(context.Background(), `SELECT mail_topics_differ($1, $2), mail_topics_differ($2, $1)`, tc.parent, tc.reply).Scan(&got, &reverse); err != nil {
			t.Fatal(err)
		}
		if got != tc.differs || reverse != tc.differs {
			t.Errorf("topics %q / %q: differs=%v reverse=%v, want %v", tc.parent, tc.reply, got, reverse, tc.differs)
		}
	}
}

func TestHistoricalTopicRepairsRequireReview(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	for file, message := range map[string]string{
		"datafix/2026-11-03_a_reply_on_a_new_topic_is_its_own_conversation.up.sql": "Superseded",
		replyTopicRegroupUp: "Topic repair is on hold",
	} {
		script, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(context.Background(), string(script)); err == nil || !strings.Contains(err.Error(), message) {
			t.Fatalf("%s: expected repair hold, got %v", file, err)
		}
	}
}
