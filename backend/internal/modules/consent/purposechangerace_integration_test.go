// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestAPurposeArchiveWaitsForAConcurrentReference(t *testing.T) {
	for _, reference := range []string{"foreign key", "scheduled key"} {
		t.Run(reference, func(t *testing.T) {
			e := setupChannelConsent(t)
			ctx := purposeConfigurer(e)
			purpose, err := e.store.CreatePurpose(ctx, "archive_race", "Archive race", false, "")
			if err != nil {
				t.Fatal(err)
			}
			writer, err := e.owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := writer.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Error(err)
				}
			}()
			if reference == "foreign key" {
				_, err = writer.Exec(ctx, `INSERT INTO consent_text_version(key,body,version,purpose_id) VALUES('race','Shown wording','v1',$1)`, purpose.ID)
			} else {
				_, err = writer.Exec(ctx, `INSERT INTO scheduled_send(scheduled_by,scheduled_at,scheduled_tz,origin_kind,origin_links,principal_kind,payload) VALUES($1,now(),'UTC','account','[]','human',jsonb_build_object('marketing_purpose',$2::text))`, e.user, purpose.Key)
			}
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			returned := make(chan struct{})
			go func() { result <- e.store.ArchivePurpose(ctx, purpose.ID); close(returned) }()
			var pid int32
			if err := writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			testdb.AwaitQueuedBehind(t, e.owner, pid, 1, returned, "purpose archive")
			if err := writer.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var used *PurposeInUseError
			if err := <-result; !errors.As(err, &used) {
				t.Fatalf("archive raced past %s: %v", reference, err)
			}
		})
	}
}
