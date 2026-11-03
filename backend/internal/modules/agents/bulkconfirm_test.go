// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "testing"

func TestTheBulkFormNamesWhatChangesInPlainWords(t *testing.T) {
	cases := []struct {
		name string
		args bulkUpdateRecordsArgs
		want string
	}{
		{
			name: "more than one company",
			args: bulkUpdateRecordsArgs{Mode: bulkModeExecute, Verb: "archive", RecordType: "company", Items: make([]BulkItem, 3)},
			want: "Archive 3 companies? You can undo it afterwards.",
		},
		{
			name: "more than one deal",
			args: bulkUpdateRecordsArgs{Mode: bulkModeExecute, Verb: "add_tag", RecordType: "deal", Items: make([]BulkItem, 2)},
			want: "Tag 2 deals? You can undo it afterwards.",
		},
		{
			name: "an undo",
			args: bulkUpdateRecordsArgs{Mode: bulkModeUndo},
			want: "Undo the bulk change? Records changed since then are left as they are.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bulkConfirmMessage(c.args); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
