// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestAJobTickCannotSendANotice(t *testing.T) {
	_, err := extensionCore{unattended: true}.Notices().Send(context.Background(), validNotice())
	if !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestANoticeNamesOneRecipientAndAKnownRecord(t *testing.T) {
	good := validNotice()
	for name, mutate := range map[string]func(*extension.NoticeRequest){
		"no recipient":      func(r *extension.NoticeRequest) { r.Member = "" },
		"two recipients":    func(r *extension.NoticeRequest) { r.Role = "sales" },
		"member not a uuid": func(r *extension.NoticeRequest) { r.Member = "ann@example.com" },
		"record not a uuid": func(r *extension.NoticeRequest) { r.Record.ID = "x" },
		"unknown record":    func(r *extension.NoticeRequest) { r.Record.Type = "invoice" },
		"no subject":        func(r *extension.NoticeRequest) { r.Subject = "" },
	} {
		t.Run(name, func(t *testing.T) {
			in := good
			mutate(&in)
			if err := validateNotice(in); !errors.Is(err, extension.ErrInvalid) {
				t.Errorf("validateNotice = %v, want ErrInvalid", err)
			}
		})
	}
	if err := validateNotice(good); err != nil {
		t.Errorf("a well-formed notice was refused: %v", err)
	}
}

func TestAUnitThatSendsTooManyNoticesIsRefused(t *testing.T) {
	const unit = "spammy-unit"
	for range extensionNoticesPerMinute {
		if !extensionNotices.Allow(unit) {
			t.Fatal("refused before the limit")
		}
	}
	core := extensionCore{unit: unit, authority: func(ctx context.Context) (context.Context, error) { return ctx, nil }}
	if _, err := core.Notices().Send(context.Background(), validNotice()); !errors.Is(err, extension.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func validNotice() extension.NoticeRequest {
	return extension.NoticeRequest{
		Member:  ids.NewV7().String(),
		Record:  extension.RecordRef{Type: "deal", ID: ids.NewV7().String()},
		Subject: "Check this deal",
	}
}
