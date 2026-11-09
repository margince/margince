// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

type domainEvidenceFixture struct {
	address                      string
	at                           time.Time
	precision                    string
	restricted, archived, capped bool
}

func seedDomainEvidence(ctx context.Context, tx pgx.Tx, f domainEvidenceFixture) (ids.ActivityID, error) {
	folded := owndomain.StoredOfAddress(f.address)
	var id ids.ActivityID
	err := tx.QueryRow(ctx, `WITH message AS (
 INSERT INTO activity(kind,source,captured_by,occurred_at,occurred_precision,restricted_at,archived_at,restricted_reason,restricted_until,retention_class,retention_class_at)
 VALUES ('email','manual','test:domain-evidence',@at,@precision,
 CASE WHEN @restricted THEN now() END,CASE WHEN @archived THEN now() END,
 CASE WHEN @restricted THEN 'test hold' END,CASE WHEN @restricted THEN now()+interval '1 year' END,
 CASE WHEN @restricted THEN 'commercial_correspondence' END,CASE WHEN @restricted THEN now() END) RETURNING id), party AS (
 INSERT INTO activity_participant(activity_id,role,address,address_domain_folded,past_party_cap)
 SELECT id,'from',NULLIF(@address,''),@folded,@capped FROM message WHERE NULLIF(@address,'') IS NOT NULL
 RETURNING activity_id) SELECT id FROM message`, pgx.NamedArgs{
		"at": f.at, "precision": f.precision, "restricted": f.restricted,
		"archived": f.archived || f.restricted, "address": f.address, "folded": folded, "capped": f.capped,
	}).Scan(&id)
	return id, err
}

func TestDomainEvidenceProjectionMatchesTheExactReaderAcrossDateAndPrivacyBoundaries(t *testing.T) {
	for _, zone := range []string{"Pacific/Kiritimati", "Pacific/Pago_Pago"} {
		t.Run(zone, func(t *testing.T) {
			e := setupDedupe(t)
			ctx := e.as()
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ctx, `INSERT INTO setting(key,value) VALUES ('installation.timezone',to_jsonb(@zone::text))
    ON CONFLICT(key) DO UPDATE SET value=excluded.value`, pgx.NamedArgs{"zone": zone}); err != nil {
					return err
				}
				var now time.Time
				if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
					return err
				}
				loc, err := time.LoadLocation(zone)
				if err != nil {
					return err
				}
				instant := now.Add(4 * time.Minute)
				today := occurredday.AnchorIn(now.Add(5*time.Minute), loc)
				fixtures := []domainEvidenceFixture{
					{address: "a@instant.test", at: now.Add(-time.Hour), precision: "instant"},
					{address: "b@sub.instant.test", at: instant, precision: "instant"},
					{address: "c@instant.test", at: now.Add(6 * time.Minute), precision: "instant"},
					{address: "d@instant.test", at: instant.Add(time.Second), precision: "instant", restricted: true},
					{address: "e@instant.test", at: instant.Add(2 * time.Second), precision: "instant", archived: true},
					{address: "f@instant.test", at: instant.Add(3 * time.Second), precision: "instant", capped: true},
					{address: "a@day.test", at: today, precision: "day"},
					{address: "b@day.test", at: today.AddDate(0, 0, 1), precision: "day"},
					{address: "c@day.test", at: today.AddDate(0, 0, -1), precision: "day"},
					{address: "a@sub.bücher.example", at: now.Add(-time.Hour), precision: "instant"},
					{address: "a@one..two.malformed.test", at: now.Add(-time.Hour), precision: "instant"},
					{address: "a@notboundary.test", at: now, precision: "instant"},
					{address: "a@amazonaws.com", at: now.Add(-time.Hour), precision: "instant"},
					{address: "b@deep.x.s3.amazonaws.com", at: now, precision: "instant"},
				}
				for _, fixture := range fixtures {
					if _, err := seedDomainEvidence(ctx, tx, fixture); err != nil {
						return err
					}
				}
				floor, err := seedDomainEvidence(ctx, tx, domainEvidenceFixture{at: now, precision: "instant"})
				if err != nil {
					return err
				}
				future, err := seedDomainEvidence(ctx, tx, domainEvidenceFixture{at: now.Add(6 * time.Minute), precision: "instant"})
				if err != nil {
					return err
				}
				cases := []struct {
					domain string
					floor  ids.ActivityID
					want   *time.Time
				}{
					{"instant.test", ids.From[ids.ActivityKind](ids.Nil), &instant},
					{"day.test", ids.From[ids.ActivityKind](ids.Nil), &today},
					{"xn--bcher-kva.example", ids.From[ids.ActivityKind](ids.Nil), new(now.Add(-time.Hour))},
					{"malformed.test", ids.From[ids.ActivityKind](ids.Nil), new(now.Add(-time.Hour))},
					{"boundary.test", ids.From[ids.ActivityKind](ids.Nil), nil},
					{"amazonaws.com", ids.From[ids.ActivityKind](ids.Nil), &now},
					{"x.s3.amazonaws.com", ids.From[ids.ActivityKind](ids.Nil), &now},
					{"absent.test", floor, &now},
					{"absent.test", future, nil},
				}
				for _, readZone := range []string{zone, "UTC"} {
					if _, err := tx.Exec(ctx, `UPDATE setting SET value=to_jsonb(@zone::text) WHERE key='installation.timezone'`, pgx.StrictNamedArgs{"zone": readZone}); err != nil {
						return err
					}
					for _, in := range cases {
						var exact *time.Time
						if err := tx.QueryRow(ctx, domainEvidenceLegacySQL(), pgx.NamedArgs{"domain": in.domain, "activity": in.floor, "skew": mailClockSkewAllowance}).Scan(&exact); err != nil {
							return err
						}
						fast := newestEvidenceFor(ctx, tx, in.domain, in.floor)
						if !sameEvidenceDate(fast, exact) || (readZone == zone && !sameEvidenceDate(fast, in.want)) {
							return fmt.Errorf("domain %s: fast=%v exact=%v want=%v", in.domain, fast, exact, in.want)
						}
					}
				}

				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func sameEvidenceDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
