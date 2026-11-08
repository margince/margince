// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
)

func pngFile(t *testing.T) string {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 8, 8))
	picture.Set(1, 1, color.RGBA{R: 200, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func fileArgs(action, name, contentType, encoded string) map[string]any {
	return map[string]any{"action": action, "filename": name, "content_type": contentType, "content_base64": encoded}
}

// A vCard an agent uploads runs the import a user's upload runs. The contact
// lands and names the agent. The card overwrites no value typed on a contact it matches.
func TestAVCardFromAnAgentImportsAsTheUploadDoes(t *testing.T) {
	d := newTwoDoors(t, "agent-vcard", "read", "write")
	typed := createdRecord(t, d.e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Dana Typed", "title": "Typed title",
		"emails": []AnyMap{{"email": "dana@cards.test", "email_type": "work", "is_primary": true}},
	})
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Nora Card\r\nEMAIL:nora@cards.test\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Dana Typed\r\nEMAIL:dana@cards.test\r\nTITLE:Card title\r\nEND:VCARD\r\n"
	report := string(d.tool(t, "change_capture", fileArgs("import_vcards", "cards.vcf", "text/vcard", base64.StdEncoding.EncodeToString([]byte(card)))))
	if !strings.Contains(report, "Nora Card") {
		t.Fatalf("the import answered %s", report)
	}
	var created, capturedBy string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT id::text, captured_by FROM contact WHERE full_name = 'Nora Card'`).Scan(&created, &capturedBy); err != nil {
		t.Fatalf("reading the imported contact: %v", err)
	}
	if !isAgentStamp(capturedBy) {
		t.Errorf("the imported contact was captured by %q, want the agent", capturedBy)
	}
	var title string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT coalesce(title, '') FROM contact WHERE id = $1`, typed).Scan(&title); err != nil {
		t.Fatalf("reading the typed contact: %v", err)
	}
	if title != "Typed title" {
		t.Errorf("the card overwrote a typed title with %q", title)
	}

	refused := d.mcp.Call(t, "change_capture", fileArgs("import_vcards", "cards.vcf", "text/vcard", "not base64!"))
	if !refused.IsError || !strings.Contains(refused.Text, "content_base64") {
		t.Errorf("bad base64 answered %+v", refused)
	}
}

// A logo an agent uploads is the logo the app draws, and a file the route refuses
// answers the route's own reason.
func TestALogoFromAnAgentIsTheLogoTheAppDraws(t *testing.T) {
	d := newTwoDoors(t, "agent-logo", "read", "write")
	if status := d.e.Call(t, "PUT", "/v1/company", wellFormedCompany(), nil, nil); status != 200 {
		t.Fatalf("describing the company → %d", status)
	}
	var answer struct {
		LogoURL *string `json:"logo_url"`
	}
	if err := json.Unmarshal(d.tool(t, "change_settings", fileArgs("upload_company_logo", "logo.png", "image/png", pngFile(t))), &answer); err != nil {
		t.Fatal(err)
	}
	var viaREST struct {
		LogoURL *string `json:"logo_url"`
	}
	if status := d.e.Call(t, "GET", "/v1/company", nil, nil, &viaREST); status != 200 || viaREST.LogoURL == nil || answer.LogoURL == nil || *viaREST.LogoURL != *answer.LogoURL {
		t.Errorf("the user's door reads %d %+v, the upload answered %+v", status, viaREST, answer)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><text>Words</text></svg>`
	refused := d.mcp.Call(t, "change_settings", fileArgs("upload_company_logo", "logo.svg", "image/svg+xml", base64.StdEncoding.EncodeToString([]byte(svg))))
	if !refused.IsError || !strings.Contains(strings.ToLower(refused.Text), "outline") {
		t.Errorf("an SVG with live text answered %+v", refused)
	}
	text := d.mcp.Call(t, "change_settings", fileArgs("upload_company_logo", "logo.txt", "text/plain", base64.StdEncoding.EncodeToString([]byte("plain words"))))
	if !text.IsError {
		t.Errorf("a text file as a logo answered %+v", text)
	}
}

// A file past its route's ceiling is refused as a browser's is. A fixed ceiling and
// one an operator sets are held the same way, and neither file changes anything.
func TestAnAgentsUploadPastItsRoutesCeilingIsRefused(t *testing.T) {
	limits := compose.DefaultUploadLimits()
	limits.CSVImport = 4 << 10
	d := newTwoDoorsWith(t, "agent-upload-ceiling", []compose.Option{compose.WithUploadLimits(limits)}, "read", "write")
	if status := d.e.Call(t, "PUT", "/v1/company", wellFormedCompany(), nil, nil); status != 200 {
		t.Fatalf("describing the company → %d", status)
	}
	logo := d.mcp.Call(t, "change_settings", fileArgs("upload_company_logo", "logo.png", "image/png",
		base64.StdEncoding.EncodeToString(make([]byte, imagenorm.MaxMarkBytes+1))))
	if !logo.IsError || !strings.Contains(logo.Text, "exceeds") {
		t.Errorf("a logo past its ceiling answered %+v", logo)
	}
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Past Ceiling\r\nEMAIL:past@cards.test\r\nNOTE:" + strings.Repeat("x", 8<<10) + "\r\nEND:VCARD\r\n"
	refused := d.mcp.Call(t, "change_capture", fileArgs("import_vcards", "cards.vcf", "text/vcard", base64.StdEncoding.EncodeToString([]byte(card))))
	if !refused.IsError {
		t.Errorf("a vCard past its ceiling answered %+v", refused)
	}
	var imported int
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM contact WHERE full_name = 'Past Ceiling'`).Scan(&imported); err != nil {
		t.Fatal(err)
	}
	if imported != 0 {
		t.Errorf("the refused card created %d contacts", imported)
	}
}

// A read-only passport uploads nothing, whichever action it names.
func TestAReadOnlyPassportUploadsNoFile(t *testing.T) {
	d := newTwoDoors(t, "agent-upload-readonly", "read")
	for _, call := range []struct{ tool, action string }{
		{"change_settings", "upload_company_logo"}, {"change_capture", "import_vcards"}, {"change_capture", "import_linkedin_connections"},
	} {
		got := d.mcp.Call(t, call.tool, fileArgs(call.action, "a.png", "image/png", pngFile(t)))
		if !got.IsError || !strings.Contains(got.Text, "scope") {
			t.Errorf("%s answered a read-only passport %+v", call.action, got)
		}
	}
}
