// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The company, mail-content and role reads over real Postgres, from a
// subscription delivery. Each runs as the unit's service member.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	// seatReaderGrants lets a seat read companies, mail and the role roster.
	seatReaderGrants = `{"objects":{"company":{"read":true},"activity":{"read":true},"user_admin":{"read":true}},"row_scope":"all"}`
	// seatOwnGrants limits a seat to the companies it owns.
	seatOwnGrants = `{"objects":{"company":{"read":true},"activity":{"read":true}},"row_scope":"own"}`
)

// serviceSeat makes the live service member of the dealmirror unit. It holds
// the grants given and has connected the unit.
func (s *systemReadEnv) serviceSeat(t *testing.T, grants string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	s.WsExec(t, `INSERT INTO app_user (id, email, display_name, status, is_agent, service_extension)
		VALUES ($1, $2, 'Feed', 'active', true, 'dealmirror')`, id, "svc-"+id.String()+"@members.invalid")
	s.grant(t, id, grants)
	s.connect(t, id)
	return id
}

func (s *systemReadEnv) company(t *testing.T, name string, owner ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	s.WsExec(t, `INSERT INTO company (id, display_name, legal_name, source, captured_by, owner_id, visibility, source_system, source_id)
		VALUES ($1, $2, $2 || ' AG', 'system', 'system', $3, 'owner', 'erp', $4)`, id, name, owner, "c-"+name)
	return id
}

// email adds a mail with a body, and sets what the columns of audience,
// archive and hold say about it.
func (s *systemReadEnv) email(t *testing.T, audience, extra string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	s.WsExec(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by, audience, audience_reason, source_id)
		VALUES ($1, 'email', 'Quote', 'Hello', 'system', 'system', $2, CASE WHEN $2 = 'workspace' THEN NULL ELSE 'manual' END, 'm-1')`, id, audience)
	if extra != "" {
		s.WsExec(t, `UPDATE activity SET `+extra+` WHERE id = $1`, id)
	}
	return id
}

// heldEmail adds a mail under a statutory hold, with the evidence the table asks for.
func (s *systemReadEnv) heldEmail(t *testing.T) ids.UUID {
	t.Helper()
	id := s.email(t, "workspace", "")
	s.WsExec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, decided_by_name, reason, qualified_at)
		VALUES ($1, 'controller_pin', 'Test Controller', 'statutory retention under test', now())`, id)
	s.WsExec(t, `UPDATE activity SET retention_class = 'commercial_correspondence', retention_class_at = now(), restricted_at = now(),
		restricted_reason = 'statutory retention', restricted_until = now() + interval '6 years', archived_at = now() WHERE id = $1`, id)
	return id
}

func (s *systemReadEnv) traceRows(t *testing.T, read string, actAs ids.UUID) int {
	t.Helper()
	var rows int
	s.read(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM system_log WHERE action = 'extension_system_read'
			AND detail->>'extension' = 'dealmirror' AND detail->>'read' = $1 AND detail->>'act_as' = $2`, read, actAs.String()).Scan(&rows)
	})
	return rows
}

func TestASeatReadIsRefusedForAnyoneButTheUnitsServiceMember(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadCompanies, extension.SystemReadMailContent, extension.SystemReadDirectory)
	company := s.company(t, "Shown GmbH", s.Rep1)
	mail := s.email(t, "workspace", "")
	s.connect(t, s.Rep1)
	unconnected := s.serviceSeat(t, seatReaderGrants)
	s.WsExec(t, `DELETE FROM extension_secret WHERE user_id = $1`, unconnected)
	otherUnit := s.serviceSeat(t, seatReaderGrants)
	s.WsExec(t, `UPDATE app_user SET service_extension = 'another' WHERE id = $1`, otherUnit)
	s.connect(t, otherUnit)
	archived := s.serviceSeat(t, seatReaderGrants)
	s.WsExec(t, `UPDATE app_user SET archived_at = now(), status = 'deactivated' WHERE id = $1`, archived)

	for name, actAs := range map[string]ids.UUID{
		"a connected colleague": s.Rep1, "a service member that never connected": unconnected,
		"the service member of another unit": otherUnit, "an archived service member": archived,
	} {
		seat := extension.UserID(actAs.String())
		s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
			_, err := rt.Companies().Get(ctx, seat, company.String())
			_, list := rt.Companies().List(ctx, seat, extension.CompanyPageQuery{})
			_, content := rt.MailContent().Get(ctx, seat, mail.String())
			_, roles := rt.Directory().ByRole(ctx, seat, []string{"reviewer"})
			for read, got := range map[string]error{"Get": err, "List": list, "MailContent": content, "ByRole": roles} {
				if !errors.Is(got, extension.ErrForbidden) {
					t.Errorf("%s as %s: err = %v, want ErrForbidden", read, name, got)
				}
			}
			return nil
		})
	}
}

func TestALimitedSeatSeesOnlyTheCompaniesTheAppShowsIt(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadCompanies)
	seat := s.serviceSeat(t, seatOwnGrants)
	mine, theirs := s.company(t, "Mine GmbH", seat), s.company(t, "Theirs GmbH", s.Rep1)
	as := extension.UserID(seat.String())
	var got extension.CompanySummary
	var hidden error
	var page extension.Page[extension.CompanySummary]
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		if got, err = rt.Companies().Get(ctx, as, mine.String()); err != nil {
			return err
		}
		_, hidden = rt.Companies().Get(ctx, as, theirs.String())
		page, err = rt.Companies().List(ctx, as, extension.CompanyPageQuery{})
		return err
	})
	if got.Name != "Mine GmbH" || got.SourceSystem != "erp" || got.SourceID != "c-Mine GmbH" || got.OwnerID != as {
		t.Errorf("Get = %+v, want the seat's company with its outside id", got)
	}
	if !errors.Is(hidden, extension.ErrNotFound) {
		t.Errorf("a colleague's company: err = %v, want ErrNotFound", hidden)
	}
	if len(page.Items) != 1 || page.Items[0].ID != mine.String() {
		t.Errorf("List = %+v, want only the seat's own company", page.Items)
	}
	if rows := s.traceRows(t, "companies", seat); rows != 3 {
		t.Errorf("three company reads left %d trace rows naming actAs, want 3", rows)
	}
}

func TestMailContentAnswersAsTheSeatMayReadTheMail(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadMailContent)
	seat := s.serviceSeat(t, seatReaderGrants)
	open := s.email(t, "workspace", "body = repeat('x', 9000)")
	refused := map[string]ids.UUID{
		"participants-only": s.email(t, "participants", ""),
		"archived":          s.email(t, "workspace", "archived_at = now()"),
		"held":              s.heldEmail(t),
		"not an email":      s.email(t, "workspace", "kind = 'note'"),
		"redacted":          s.email(t, "workspace", "redacted_fields = '{body}'"),
	}
	restricted := s.email(t, "workspace", "")
	contact := ids.NewV7()
	s.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by, visibility) VALUES ($1, 'Restricted', 'manual', 'system', 'workspace')`, contact)
	s.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'attendee')`, restricted, contact)
	s.WsExec(t, `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'processing_restriction', 'test', 'human:test', 'subject')`, contact)
	refused["Art. 18 restricted"] = restricted

	as := extension.UserID(seat.String())
	errs := map[string]error{}
	var got, withheld extension.MailContent
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		if got, err = rt.MailContent().Get(ctx, as, open.String()); err != nil {
			return err
		}
		for name, id := range refused {
			var c extension.MailContent
			c, errs[name] = rt.MailContent().Get(ctx, as, id.String())
			if name == "redacted" {
				withheld = c
			}
		}
		return nil
	})
	if got.Subject != "Quote" || got.ContentState != extension.MailContentAvailable {
		t.Errorf("open mail = %+v, want its subject", got)
	}
	if len([]rune(got.Body)) != extension.MaxMailBody || !got.BodyTruncated || !strings.HasPrefix(got.Body, "xxx") {
		t.Errorf("a 9000 character body came as %d characters, truncated %v", len(got.Body), got.BodyTruncated)
	}
	for name, err := range errs {
		if name == "redacted" {
			continue
		}
		if !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("%s mail: err = %v, want ErrNotFound", name, err)
		}
	}
	if withheld.ContentState != extension.MailContentWithheld || withheld.Body != "" || withheld.Subject != "" {
		t.Errorf("redacted mail = %+v, want withheld with no words", withheld)
	}
	if rows := s.traceRows(t, "mail_content", seat); rows != 1+len(refused) {
		t.Errorf("%d mail reads left %d trace rows naming actAs", 1+len(refused), rows)
	}
}

func TestMailContentIsHiddenFromASeatWithoutTheActivityGrant(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadMailContent)
	seat := s.serviceSeat(t, `{"objects":{"company":{"read":true}},"row_scope":"all"}`)
	mail := s.email(t, "workspace", "")
	var err error
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		_, err = rt.MailContent().Get(ctx, extension.UserID(seat.String()), mail.String())
		return nil
	})
	if !errors.Is(err, extension.ErrForbidden) && !errors.Is(err, extension.ErrNotFound) {
		t.Errorf("a seat with no activity grant: err = %v, want a refusal", err)
	}
}

func TestTheRoleReadNeedsTheRosterAndHidesWhichRolesExist(t *testing.T) {
	s := setupSystemRead(t)
	composeReader(t, extension.SystemReadDirectory)
	s.WsExec(t, `INSERT INTO role (key, name) VALUES ('reviewer', 'Reviewer')`)
	s.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = 'reviewer'`, s.Rep1)
	reader := s.serviceSeat(t, seatReaderGrants)
	blind := s.serviceSeat(t, seatOwnGrants)
	var got, unknown []extension.UserID
	var noRoster error
	s.onDelivery(t, func(ctx context.Context, rt extension.Runtime) error {
		var err error
		if got, err = rt.Directory().ByRole(ctx, extension.UserID(reader.String()), []string{"reviewer"}); err != nil {
			return err
		}
		if unknown, err = rt.Directory().ByRole(ctx, extension.UserID(reader.String()), []string{"no-such-role"}); err != nil {
			return err
		}
		_, noRoster = rt.Directory().ByRole(ctx, extension.UserID(blind.String()), []string{"reviewer"})
		return nil
	})
	if len(got) != 1 || got[0] != extension.UserID(s.Rep1.String()) {
		t.Errorf("ByRole = %v, want only the reviewer %s", got, s.Rep1)
	}
	if len(unknown) != 0 {
		t.Errorf("an unknown role gave %v, want nobody and no error", unknown)
	}
	if !errors.Is(noRoster, extension.ErrForbidden) {
		t.Errorf("a seat without the roster: err = %v, want ErrForbidden", noRoster)
	}
	var keys string
	s.read(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT detail->'requested'::text FROM system_log WHERE action = 'extension_system_read'
			AND detail->>'read' = 'directory' AND detail->>'act_as' = $1 ORDER BY id LIMIT 1`, reader.String()).Scan(&keys)
	})
	if keys != `["reviewer"]` {
		t.Errorf("the trace names the keys %s, want [\"reviewer\"]", keys)
	}
	if rows := s.traceRows(t, "directory", reader); rows != 2 {
		t.Errorf("two role reads left %d trace rows naming actAs, want 2", rows)
	}
}
