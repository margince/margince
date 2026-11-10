// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// Writing to an address on two separate threads is the exchange; any number of
// messages on one thread is not. Mail that does not count as ours, or that the
// external evidence rule hides, never makes the second thread.
func TestWritingOnTwoThreadsIsCountedByThreadNotByMessage(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	type mail struct {
		thread     string
		attested   bool
		restricted bool
		archived   bool
	}
	cases := []struct {
		name string
		mail []mail
		want bool
	}{
		{"nothing written", nil, false},
		{"three messages on one thread", []mail{{"t1", true, false, false}, {"t1", true, false, false}, {"t1", true, false, false}}, false},
		{"two threads", []mail{{"t1", true, false, false}, {"t1", true, false, false}, {"t2", true, false, false}}, true},
		{"the second thread is not ours", []mail{{"t1", true, false, false}, {"t2", false, false, false}}, false},
		{"the second thread is restricted", []mail{{"t1", true, false, false}, {"t2", true, true, false}}, false},
		{"a blank key is no thread", []mail{{"t1", true, false, false}, {"", true, false, false}}, false},
		{"a missing key is no thread", []mail{{"t1", true, false, false}, {"null", true, false, false}}, false},
		{"archived mail that is not restricted still counts", []mail{{"t1", true, false, false}, {"t2", true, false, true}}, true},
		{"the first thread found is restricted", []mail{{"t1", true, true, false}, {"t2", true, false, false}, {"t3", true, false, false}}, true},
	}
	for i, tc := range cases {
		address := "two-threads-" + string(rune('a'+i)) + "@example.com"
		if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			for _, m := range tc.mail {
				if _, err := tx.Exec(ctx, `INSERT INTO activity (kind, occurred_at, source, captured_by, direction,
					counterparty_email, counterparty_outbound_attested, thread_key, restricted_at, archived_at, restricted_reason, restricted_until,
					retention_class, retention_class_at)
					VALUES ('email', now(), 'manual', 'human:test', 'outbound', $1, $2,
					        CASE WHEN $3 = 'null' THEN NULL WHEN $3 = '' THEN '' ELSE $1 || $3 END, CASE WHEN $4 THEN now() END,
					        CASE WHEN $4 OR $5 THEN now() END,
					        CASE WHEN $4 THEN 'commercial_correspondence' END, CASE WHEN $4 THEN now() + interval '1 year' END,
					        CASE WHEN $4 THEN 'commercial_correspondence' END, CASE WHEN $4 THEN now() END)`,
					address, m.attested, m.thread, m.restricted, m.archived); err != nil {
					return err
				}
			}
			got, err := wroteOnTwoThreadsTx(ctx, tx, address)
			if err != nil {
				return err
			}
			if got != tc.want {
				t.Errorf("%s: wrote on two threads = %v, want %v", tc.name, got, tc.want)
			}
			return nil
		}); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
}
