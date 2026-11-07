// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An archived record is closed to reads of what hangs off it. Its tag list
// answers not-found as its lists do, whichever path archived it.
// Each case archives through the product's own writer, so the row is in the
// state that writer leaves it in.
func TestRecordTagsOfAnArchivedRecordAnswerNotFoundLikeItsSiblings(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	ctx := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	tag, err := store.CreateTag(ctx, "Archived Conference", nil, nil)
	if err != nil {
		t.Fatalf("coining the word: %v", err)
	}

	cases := []struct {
		label   string
		entity  string
		seed    func() ids.UUID
		archive func(id ids.UUID) error
	}{
		{
			"contact", "contact",
			func() ids.UUID { return e.SeedContact(t, "Archived Contact", &e.Rep1) },
			func(id ids.UUID) error {
				_, err := e.Contacts.ArchiveContact(ctx, ids.From[ids.ContactKind](id), nil)
				return err
			},
		},
		{
			"company", "company",
			func() ids.UUID { return e.SeedCompany(t, "Archived Company", &e.Rep1) },
			func(id ids.UUID) error {
				_, err := e.Contacts.ArchiveCompany(ctx, ids.From[ids.CompanyKind](id), nil)
				return err
			},
		},
		{
			"deal", "deal",
			func() ids.UUID { return e.SeedDeal(t, "Archived Deal", pipeline, open, &e.Rep1) },
			func(id ids.UUID) error {
				_, err := e.Deals.ArchiveDeal(ctx, ids.From[ids.DealKind](id), nil)
				return err
			},
		},
		{
			"disqualified lead", "lead",
			func() ids.UUID {
				return seedLead(t, e, "Disqualified Lead", "disqualified@prospect.test", &e.Rep1).UUID
			},
			func(id ids.UUID) error {
				_, err := e.Contacts.DisqualifyLead(ctx, leadIDOf(id), contacts.DisqualifyLeadInput{})
				return err
			},
		},
		{
			"promoted lead", "lead",
			func() ids.UUID { return seedLead(t, e, "Promoted Lead", "promoted@prospect.test", &e.Rep1).UUID },
			func(id ids.UUID) error {
				_, _, err := e.Contacts.PromoteLead(ctx, leadIDOf(id), contacts.PromoteLeadInput{
					Trigger: "human_qualify", EvidenceNote: new("replied to outreach"),
				})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			id := tc.seed()
			if _, err := store.ApplyTag(ctx, tag.ID, tc.entity, id); err != nil {
				t.Fatalf("tagging the %s: %v", tc.entity, err)
			}
			if _, err := store.RecordTagsFor(ctx, tc.entity, id); err != nil {
				t.Fatalf("a live %s must still read its tags: %v", tc.entity, err)
			}
			if err := tc.archive(id); err != nil {
				t.Fatalf("archiving the %s: %v", tc.entity, err)
			}
			_, err := store.RecordTagsFor(ctx, tc.entity, id)
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("tags of an archived %s answered %v, want not-found", tc.entity, err)
			}
		})
	}
}
