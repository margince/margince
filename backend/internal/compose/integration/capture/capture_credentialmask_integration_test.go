// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
)

const credentialMailID = "portal-access-1@acme.example"

func portalAccessMail() []byte {
	return []byte(strings.Join([]string{
		"From: Anna Acme <anna@acme.example>",
		"To: " + captureOwner,
		"Subject: Portal access, password: Quokka7Subject",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + credentialMailID + ">",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hallo, here is your Kolibri portal access.",
		"",
		"Login: anna.portal",
		"Passwort: Quokka7Body",
		"",
	}, "\r\n"))
}

// A captured mail stores its subject and body with the login and password
// masked, so no reader of the mail and no search finds them. The provider
// original keeps them until its own retention ends.
func TestACapturedMailStoresItsCredentialsMasked(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	env.sync(t, portalAccessMail())
	activityID := oneActivityID(t, e)

	var subject, body string
	var raw []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT subject, body FROM activity WHERE id = $1`, activityID).Scan(&subject, &body); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT payload FROM raw_capture WHERE source_id = $1`, credentialMailID).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the captured mail: %v", err)
	}
	if subject != "Portal access, password: [masked]" {
		t.Errorf("subject = %q, want the password masked behind its label", subject)
	}
	for _, secret := range []string{"Quokka7Body", "anna.portal"} {
		if strings.Contains(body, secret) {
			t.Errorf("body holds %q: %q", secret, body)
		}
	}
	for _, masked := range []string{"Login: [masked]", "Passwort: [masked]"} {
		if !strings.Contains(body, masked) {
			t.Errorf("body = %q, want %q", body, masked)
		}
	}
	if !strings.Contains(string(raw), "Passwort: Quokka7Body") {
		t.Errorf("raw_capture.payload lost the original: %q", raw)
	}

	owner := readerCtx(e, e.Rep1)
	for query, want := range map[string]int{"Kolibri": 1, "Quokka7Body": 0, "Quokka7Subject": 0} {
		page, err := e.Store.Search(owner, search.Input{Query: query, Types: []string{"activity"}})
		if err != nil {
			t.Fatalf("search %q: %v", query, err)
		}
		if len(page.Hits) != want {
			t.Errorf("search %q found %d hits, want %d", query, len(page.Hits), want)
		}
	}
}
