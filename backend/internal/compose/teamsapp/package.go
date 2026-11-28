// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package teamsapp builds the installable Teams channel app package.
package teamsapp

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The frontend asset is mirrored because Go can embed only package-local files.
// TestPackageUsesTheProductIcon binds this copy to the product's canonical icon.
//
//go:embed assets/color.png
var colorIcon []byte

// Config binds the Teams app package to its operator-reviewed public metadata.
type Config struct {
	AppID      string
	Origin     string
	PrivacyURL string
	TermsURL   string
}

type appName struct {
	Short string `json:"short"`
	Full  string `json:"full"`
}
type appIcons struct {
	Color   string `json:"color"`
	Outline string `json:"outline"`
}

//nolint:tagliatelle // Teams app manifests require Microsoft's JSON field names.
type developer struct {
	Name    string `json:"name"`
	Website string `json:"websiteUrl"`
	Privacy string `json:"privacyUrl"`
	Terms   string `json:"termsOfUseUrl"`
}

//nolint:tagliatelle // Teams app manifests require Microsoft's JSON field names.
type bot struct {
	ID               string   `json:"botId"`
	Scopes           []string `json:"scopes"`
	NotificationOnly bool     `json:"isNotificationOnly"`
	SupportsFiles    bool     `json:"supportsFiles"`
}
type webApp struct {
	ID string `json:"id"`
}
type permission struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

//nolint:tagliatelle // Teams app manifests require Microsoft's JSON field names.
type permissions struct {
	ResourceSpecific []permission `json:"resourceSpecific"`
}
type authorization struct {
	Permissions permissions `json:"permissions"`
}

//nolint:tagliatelle // Teams app manifests require Microsoft's JSON field names.
type manifest struct {
	Schema          string        `json:"$schema"`
	ManifestVersion string        `json:"manifestVersion"`
	Version         string        `json:"version"`
	ID              string        `json:"id"`
	Name            appName       `json:"name"`
	Description     appName       `json:"description"`
	Developer       developer     `json:"developer"`
	Icons           appIcons      `json:"icons"`
	AccentColor     string        `json:"accentColor"`
	Bots            []bot         `json:"bots"`
	Domains         []string      `json:"validDomains"`
	WebApp          webApp        `json:"webApplicationInfo"`
	Authorization   authorization `json:"authorization"`
}

// Archive builds the Teams package from the reviewed application and origin.
func Archive(cfg Config) ([]byte, error) {
	if _, err := ids.Parse(cfg.AppID); err != nil {
		return nil, apperrors.ErrInvalidArgument
	}
	for _, address := range []string{cfg.Origin, cfg.PrivacyURL, cfg.TermsURL} {
		if !httpsURL(address) {
			return nil, apperrors.ErrInvalidArgument
		}
	}
	origin, err := url.Parse(cfg.Origin)
	if err != nil {
		return nil, apperrors.ErrInvalidArgument
	}
	grants := []permission{}
	for _, scope := range teams.BotScopes {
		grants = append(grants, permission{Name: scope, Type: "Application"})
	}
	app := manifest{Schema: "https://developer.microsoft.com/en-us/json-schemas/teams/v1.20/MicrosoftTeams.schema.json", ManifestVersion: "1.20", Version: "1.0.0", ID: cfg.AppID, Name: appName{Short: "Margince", Full: "Margince record messaging"}, Description: appName{Short: "Company, deal and project updates", Full: "Link approved team channels to companies, deals and projects in Margince. Keep CRM History current with useful conversation recaps and send eligible record updates to the channel. Personal Messenger access requires separate consent."}, Developer: developer{Name: "Margince", Website: cfg.Origin, Privacy: cfg.PrivacyURL, Terms: cfg.TermsURL}, Icons: appIcons{Color: "color.png", Outline: "outline.png"}, AccentColor: "#07584c", Bots: []bot{{ID: cfg.AppID, Scopes: []string{"team"}}}, Domains: []string{origin.Hostname()}, WebApp: webApp{ID: cfg.AppID}, Authorization: authorization{Permissions: permissions{ResourceSpecific: grants}}}
	raw, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		return nil, err
	}
	outline, err := outlineIcon()
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, entry := range []struct {
		name string
		body []byte
	}{{"manifest.json", raw}, {"color.png", colorIcon}, {"outline.png", outline}} {
		writer, err := archive.Create(entry.name)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(entry.body); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func httpsURL(address string) bool {
	u, err := url.Parse(address)
	return err == nil && len(address) <= 2048 && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && !strings.ContainsAny(address, "\r\n")
}

func outlineIcon() ([]byte, error) {
	source, err := png.Decode(bytes.NewReader(colorIcon))
	if err != nil {
		return nil, err
	}
	outline := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			r, _, _, a := source.At(x*source.Bounds().Dx()/32, y*source.Bounds().Dy()/32).RGBA()
			if r > 0x8000 && a > 0 {
				outline.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: uint8((a >> 8) & 0xff)})
			}
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, outline); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
