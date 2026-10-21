// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMergePreviewNamesCarriedNoticesAndWithholdsOtherRecipients(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.Rep1, nil, integration.AdminPerms)
			source, target := mergeContentPair(t, e, kind)
			store := notices.NewStore(e.DB())
			id, err := store.Create(ctx, notices.NewNotice{Recipient: ids.From[ids.UserKind](e.Rep1), Kind: "automation", Subject: "Review this record", Target: notices.Target{Type: kind, ID: source}})
			if err != nil {
				t.Fatal(err)
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Notices) != 1 || ids.UUID(preview.Notices[0].Id) != id || preview.Notices[0].Subject != "Review this record" {
				t.Fatalf("notices=%+v", preview.Notices)
			}
			hidden, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.Rep2, nil, integration.AdminPerms), kind, source, target, nil, nil)
			if !errors.Is(err, apperrors.ErrNotFound) || hidden.Notices != nil {
				t.Fatalf("other recipient preview=%+v error=%v", hidden, err)
			}
			e.Contacts.WithMergeNoticeReader(nil)
			partial, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
			if !errors.Is(err, apperrors.ErrInvalidArgument) || partial.Notices != nil {
				t.Fatalf("unwired preview=%+v error=%v", partial, err)
			}
			e.Contacts.WithMergeNoticeReader(notices.PreviewNoticeCarryTx)
			if kind == "contact" {
				_, err = e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), &preview.SourceVersion)
			} else {
				_, err = e.Contacts.MergeCompany(ctx, ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target), &preview.SourceVersion)
			}
			if err != nil {
				t.Fatal(err)
			}
			page, err := store.ListFor(ctx, 50, "")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range page.Items {
				if item.ID == id {
					found = true
					if item.Target.ID != target {
						t.Fatalf("notice target=%+v", item.Target)
					}
				}
			}
			if !found {
				t.Fatal("carried notice missing from recipient history")
			}
		})
	}
}
