// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The ask greets the colleague by the name they chose to be greeted by, read
// from their own seat, and by their display name's first word when they chose
// none.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnIntroDraftGreetsTheColleagueByTheirGreetingName(t *testing.T) {
	e := integration.Setup(t)
	company, contact := seedIntroducibleContact(t, e, "Ute Sommer")
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, company360NoDealPerms)

	body, err := draftIntro(rep, t, e, company, contact)
	if err != nil {
		t.Fatalf("the baseline draft was refused: %v", err)
	}
	if !strings.HasPrefix(body, "Hi Rep,") {
		t.Fatalf("with no greeting name the draft should greet the display name's first word:\n%s", body)
	}

	e.WsExec(t, `UPDATE app_user SET greeting_name = 'Sofia' WHERE id = $1`, e.Rep2)

	body, err = draftIntro(rep, t, e, company, contact)
	if err != nil {
		t.Fatalf("drafting with a greeting name set: %v", err)
	}
	if !strings.HasPrefix(body, "Hi Sofia,") {
		t.Fatalf("the draft does not greet the colleague's chosen greeting name:\n%s", body)
	}
}
