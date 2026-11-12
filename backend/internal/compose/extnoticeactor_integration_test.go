// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A notice sent with nobody behind it is tied to an actor: the unit's service
// member, or a member who connected the unit. Without one it is refused.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

const noticeUnit = "dealmirror"

// sendUnattended sends as a bus delivery would: the core port the unit gets,
// with its caller taken away and notices allowed.
func (w noticeWorld) sendUnattended(t *testing.T, in extension.NoticeRequest) (extension.NoticeSent, error) {
	t.Helper()
	composeReader(t, extension.SystemReadDeals)
	var sent extension.NoticeSent
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, "deal", "company"), func(ctx context.Context, core extension.Core) error {
		unattended, ok := core.(extensionCore)
		if !ok {
			t.Fatalf("the port is a %T, want extensionCore", core)
		}
		unattended.unattended, unattended.notifies, unattended.unit = true, true, noticeUnit
		var err error
		sent, err = unattended.Notices().Send(ctx, in)
		return err
	})
	return sent, err
}

func (w noticeWorld) aboutTheDeal(member string, actAs extension.UserID) extension.NoticeRequest {
	return extension.NoticeRequest{
		Member: member, ActAs: actAs, Subject: "Look", Record: extension.RecordRef{Type: "deal", ID: w.mineDeal.String()},
	}
}

func (w noticeWorld) connect(t *testing.T, user string) {
	t.Helper()
	if _, err := w.owner.Exec(context.Background(), `INSERT INTO extension_secret (id, extension_name, user_id, key, vault_ref)
		VALUES (uuidv7(), $1, $2, 'token', 'vault/x')`, noticeUnit, user); err != nil {
		t.Fatal(err)
	}
}

func TestAnUnattendedNoticeNeedsAnActor(t *testing.T) {
	w := setupNotices(t)
	if _, err := w.sendUnattended(t, w.aboutTheDeal(w.reader.String(), "")); !errors.Is(err, extension.ErrForbidden) {
		t.Errorf("a send with no service member and no ActAs = %v, want ErrForbidden", err)
	}
	if got, _ := w.noticesFor(t, w.reader); len(got) != 0 {
		t.Errorf("a refused send left %v", got)
	}
	declareServiceMemberFor(t, noticeUnit)
	sent, err := w.sendUnattended(t, w.aboutTheDeal(w.reader.String(), ""))
	if err != nil || sent.Recipients != 1 {
		t.Errorf("a send as the service member = %+v, %v; want one recipient", sent, err)
	}
}

func TestANoticeSentForAMemberNeedsThemToHaveConnectedTheUnitAndOpenTheRecord(t *testing.T) {
	w := setupNotices(t)
	if _, err := w.sendUnattended(t, w.aboutTheDeal(w.reader.String(), extension.UserID(w.reader.String()))); !errors.Is(err, extension.ErrForbidden) {
		t.Errorf("ActAs a member who did not connect the unit = %v, want ErrForbidden", err)
	}
	w.connect(t, w.blind.String())
	if _, err := w.sendUnattended(t, w.aboutTheDeal(w.reader.String(), extension.UserID(w.blind.String()))); !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("ActAs a member who cannot open the deal = %v, want ErrNotFound", err)
	}
	w.connect(t, w.reader.String())
	sent, err := w.sendUnattended(t, w.aboutTheDeal(w.reader.String(), extension.UserID(w.reader.String())))
	if err != nil || sent.Recipients != 1 {
		t.Errorf("ActAs a member who connected the unit and can open the deal = %+v, %v; want one recipient", sent, err)
	}
}

func TestASendWithACallerCannotNameAnotherActor(t *testing.T) {
	w := setupNotices(t)
	if _, err := w.send(t, w.aboutTheDeal(w.reader.String(), extension.UserID(w.reader.String()))); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("ActAs on a send with a caller = %v, want ErrInvalid", err)
	}
}
