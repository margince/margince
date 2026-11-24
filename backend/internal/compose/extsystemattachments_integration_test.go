// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The attachment reads from a subscription delivery, as the unit's service
// member. systemmail's own suite holds every way a file is hidden.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestAMailFileListsAndOpensForTheServiceMemberAndNobodyElse(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadMailAttachments)
	blob := blobstore.NewMemory()
	previous := boundExtensionRuntime().blob
	BindExtensionObjects(blob)
	t.Cleanup(func() { BindExtensionObjects(previous) })
	seat := s.serviceSeat(t, seatReaderGrants)
	mail, hidden := s.email(t, "workspace", ""), s.email(t, "participants", "")
	quote := s.CaptureMailFile(t, blob, mail, "quote.bin", "application/octet-stream", []byte("%PDF-1.4 quote"))
	s.connect(t, s.Rep1)

	var (
		listed     []extension.MailAttachment
		opened     extension.MailAttachmentFile
		privateErr error
		refused    = map[string]error{}
	)
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		as := extension.UserID(seat.String())
		var err error
		if listed, err = rt.Activities().Attachments(ctx, as, mail.String()); err != nil {
			return err
		}
		if opened, err = rt.Activities().OpenAttachment(ctx, as, quote.String()); err != nil {
			return err
		}
		_, privateErr = rt.Activities().Attachments(ctx, as, hidden.String())
		colleague := extension.UserID(s.Rep1.String())
		_, refused["Attachments"] = rt.Activities().Attachments(ctx, colleague, mail.String())
		_, refused["OpenAttachment"] = rt.Activities().OpenAttachment(ctx, colleague, quote.String())
		return nil
	})
	if len(listed) != 1 || listed[0].ID != quote.String() || string(opened.Content) != "%PDF-1.4 quote" || opened.ContentType != "application/pdf" {
		t.Errorf("Attachments = %+v, OpenAttachment = %q as %q; want the one file and its bytes", listed, opened.Content, opened.ContentType)
	}
	if !errors.Is(privateErr, extension.ErrNotFound) {
		t.Errorf("a participants-only mail: err = %v, want ErrNotFound", privateErr)
	}
	for read, err := range refused {
		if !errors.Is(err, extension.ErrForbidden) {
			t.Errorf("%s as a connected colleague: err = %v, want ErrForbidden", read, err)
		}
	}
	if rows := s.traceRows(t, "mail_attachments", seat); rows != 3 {
		t.Errorf("three reads left %d trace rows naming actAs, want 3", rows)
	}
	if rows := s.WsCount(t, `SELECT count(*) FROM system_log WHERE action = 'export' AND detail->>'act_as' = $1`, seat.String()); rows != 2 {
		t.Errorf("a list and an open left %d export rows, want 2", rows)
	}
}
