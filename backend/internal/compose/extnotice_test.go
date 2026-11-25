// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestAJobTickCannotSendANotice(t *testing.T) {
	_, err := extensionCore{unattended: true}.Notices().Send(context.Background(), validNotice())
	if !errors.Is(err, extension.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

// A delivery has nobody behind it and may still send, so a flag found while a
// mail is delivered reaches someone. The send gets past the refusal and meets
// the rate limit, which an unattended send is held to like any other.
func TestADeliveryMaySendANoticeAndATickMayNot(t *testing.T) {
	const unit = "delivery-unit"
	for range extensionNoticesPerMinute {
		extensionNotices.Allow(unit)
	}
	authority := func(ctx context.Context) (context.Context, error) { return ctx, nil }
	delivery := deliveryRuntimeFor(context.Background(), unit, "1.0.0", "subscription/flag", extensionRuntimeBinding{})
	core := extensionCore{unit: unit, unattended: delivery.unattended, notifies: delivery.deliversNotices, authority: authority}
	if _, err := core.Notices().Send(context.Background(), validNotice()); !errors.Is(err, extension.ErrRateLimited) {
		t.Errorf("a delivery's send = %v, want it past the refusal and on to ErrRateLimited", err)
	}
	tick := jobRuntimeFor(context.Background(), unit, "1.0.0", "job/nightly", extensionRuntimeBinding{})
	if tick.deliversNotices {
		t.Error("a job tick may send a notice")
	}
}

const testNoticeUnit = "delivery-unit"

func TestANoticeMayNameARecordKindOnlyItsOwnUnitDeclared(t *testing.T) {
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	recordkinds.Register("delivery-unit", []extension.RecordKindDeclaration{{Kind: "room"}})
	recordkinds.Register("other-unit", []extension.RecordKindDeclaration{{Kind: "room"}})
	for name, c := range map[string]struct {
		recordType string
		wantErr    bool
	}{
		"its own kind":           {"ext:delivery-unit:room", false},
		"another unit's kind":    {"ext:other-unit:room", true},
		"a kind nobody declared": {"ext:delivery-unit:ghost", true},
	} {
		t.Run(name, func(t *testing.T) {
			in := validNotice()
			in.Record.Type = c.recordType
			err := validateNotice(testNoticeUnit, in)
			if c.wantErr != errors.Is(err, extension.ErrInvalid) {
				t.Errorf("validateNotice(%s) = %v, want refusal %v", c.recordType, err, c.wantErr)
			}
		})
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
			if err := validateNotice(testNoticeUnit, in); !errors.Is(err, extension.ErrInvalid) {
				t.Errorf("validateNotice = %v, want ErrInvalid", err)
			}
		})
	}
	if err := validateNotice(testNoticeUnit, good); err != nil {
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
