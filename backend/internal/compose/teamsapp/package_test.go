// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teamsapp

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
)

func TestPackageUsesTheProductIcon(t *testing.T) {
	canonical, err := os.ReadFile("../../../../frontend/public/icon-192x192.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, colorIcon) {
		t.Fatal("Teams package icon drifted from the product icon")
	}
}

func TestPackagePinsAppAndRequestsOnlyTeamScopedApplicationConsent(t *testing.T) {
	cfg := Config{AppID: "018f3a1b-0000-7000-8000-00000000a001", Origin: "https://crm.example.test", PrivacyURL: "https://example.test/privacy", TermsURL: "https://example.test/terms"}
	packed, err := Archive(cfg)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(packed), int64(len(packed)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("read package entry: %v %v", err, closeErr)
		}
		entries[file.Name] = body
	}
	var app manifest
	if err := json.Unmarshal(entries["manifest.json"], &app); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || app.ID != cfg.AppID || app.WebApp.ID != cfg.AppID || len(app.Bots) != 1 || app.Bots[0].ID != cfg.AppID || len(app.Bots[0].Scopes) != 1 || app.Bots[0].Scopes[0] != "team" {
		t.Fatalf("package is not pinned to the configured team app: %+v", app)
	}
	grants := app.Authorization.Permissions.ResourceSpecific
	if len(grants) != len(teams.BotScopes) {
		t.Fatalf("permission count differs from SDK: %+v", grants)
	}
	for i, scope := range teams.BotScopes {
		if grants[i].Name != scope || grants[i].Type != "Application" || !strings.HasSuffix(scope, ".Group") {
			t.Fatalf("incorrect or tenant-wide permission: %+v", grants[i])
		}
	}
	if app.Developer.Privacy != cfg.PrivacyURL || app.Developer.Terms != cfg.TermsURL || len(app.Domains) != 1 || app.Domains[0] != "crm.example.test" {
		t.Fatalf("operator URLs lost: %+v", app)
	}
	for name, size := range map[string]int{"color.png": 192, "outline.png": 32} {
		icon, err := png.Decode(bytes.NewReader(entries[name]))
		if err != nil || icon.Bounds().Dx() != size || icon.Bounds().Dy() != size {
			t.Fatalf("invalid %s dimensions: %v", name, err)
		}
		if name == "outline.png" {
			visible := 0
			for y := range size {
				for x := range size {
					r, g, b, a := icon.At(x, y).RGBA()
					if a != 0 {
						visible++
						if r != a || g != a || b != a {
							t.Fatal("outline icon contains a nonwhite pixel")
						}
					}
				}
			}
			if visible == 0 || visible == size*size {
				t.Fatal("outline lost either the logo or its transparent background")
			}
		}
	}
	cfg.PrivacyURL = "http://example.test/privacy"
	if _, err := Archive(cfg); err == nil {
		t.Fatal("generated package with insecure legal URL")
	}
}
