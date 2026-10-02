// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var quoteReaderPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"voice_profile": {Create: true, Read: true, Update: true},
		"contact":       {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

func TestAQuotedNameIsSomebodyTheOwnerCanSee(t *testing.T) {
	e := integration.Setup(t)
	e.SeedContact(t, "Sam Weber", &e.Rep2)
	private := e.SeedContact(t, "Dana Ortiz", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", private, e.Rep3)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Lena Fischer' WHERE id = $1`, e.Rep2)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Nav Playa' WHERE id = $1`, e.Rep1)
	known := voiceKnownSpeakers(e.Pool)
	labels := []string{"Sam", "Dana", "fischer", "Nav", "Frage"}

	named, err := known(e.As(e.Rep1, []ids.UUID{e.Team1}, quoteReaderPerms), labels)
	if err != nil {
		t.Fatal(err)
	}
	// Dana is another rep's private capture, the owner is not a quotation of
	// themselves, and a heading is nobody.
	if got := strings.Join(named, ","); got != "Sam,fischer" {
		t.Fatalf("named = %q, want the visible contact and the colleague only", got)
	}

	noContacts := quoteReaderPerms
	noContacts.Objects = map[string]principal.ObjectGrant{"voice_profile": {Create: true, Read: true, Update: true}}
	named, err = known(e.As(e.Rep1, []ids.UUID{e.Team1}, noContacts), labels)
	if err != nil {
		t.Fatalf("a caller who cannot read contacts still ingests: %v", err)
	}
	if got := strings.Join(named, ","); got != "fischer" {
		t.Fatalf("named = %q, want only the colleague — contacts the caller cannot read name nobody", got)
	}
}
