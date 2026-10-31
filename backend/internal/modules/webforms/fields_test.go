// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"bytes"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var testFormID = ids.MustParse("018f0000-0000-7000-8000-000000000001")

func TestCommonFieldSpellingsReadAsLeadFields(t *testing.T) {
	e := readEnquiry([]Field{
		{"your-email", "Ada@Example.com"},
		{"Vorname", "Ada"},
		{"Nachname", "Lovelace"},
		{"Telefon", "+49 30 1234"},
		{"Firma", "Engines Ltd"},
		{"your-message", "Hi there"},
		{"Datenschutz", "on"},
		{"utm_source", "spring-mailing"},
		{"budget", "10k"},
		{"_mrg_hp", ""},
		{"_mrg_ts", "1"},
	}, nil)
	if e.Email != "ada@example.com" || e.FullName != "Ada Lovelace" || e.Phone != "+49 30 1234" ||
		e.Company != "Engines Ltd" || e.Message != "Hi there" || !e.Consent {
		t.Fatalf("readEnquiry = %+v", e)
	}
	if len(e.UTM) != 1 || e.UTM[0] != (Field{"utm_source", "spring-mailing"}) {
		t.Fatalf("UTM = %v", e.UTM)
	}
	if len(e.Extra) != 1 || e.Extra[0] != (Field{"budget", "10k"}) {
		t.Fatalf("Extra = %v, want only the field no lead column takes", e.Extra)
	}
	if _, kept := e.controls["_mrg_ts"]; !kept {
		t.Fatal("a control field was not read for the spam checks")
	}
}

func TestEachSpellingReadsAsOneTarget(t *testing.T) {
	seen := map[string]string{}
	for target, spellings := range aliasSpellings {
		for spelling := range strings.FieldsSeq(spellings) {
			if other, twice := seen[spelling]; twice {
				t.Errorf("%q reads as both %s and %s", spelling, other, target)
			}
			seen[spelling] = target
			if fold(spelling) != spelling {
				t.Errorf("%q is not folded, so no field name can match it", spelling)
			}
		}
	}
}

func TestAFieldMapNamesWhatTheDefaultsDoNotRead(t *testing.T) {
	e := readEnquiry([]Field{{"field_3", "x@y.example"}, {"Email", "ignored@y.example"}}, map[string]string{"field_3": fieldEmail})
	if e.Email != "x@y.example" {
		t.Fatalf("Email = %q, want the mapped field to win", e.Email)
	}
}

func TestAnOversizedSubmissionIsBounded(t *testing.T) {
	fields := make([]Field, 0, maxFields+50)
	for range maxFields + 50 {
		fields = append(fields, Field{"note", strings.Repeat("x", maxValueLength*2)})
	}
	e := readEnquiry(fields, nil)
	if len(e.Extra) > maxFields {
		t.Fatalf("kept %d fields, over the %d bound", len(e.Extra), maxFields)
	}
	if got := len([]rune(e.Extra[0].Value)); got != maxValueLength {
		t.Fatalf("a value kept %d characters, want %d", got, maxValueLength)
	}
}

func TestANulByteNeverReachesTheEnquiry(t *testing.T) {
	fields, err := parseFields("application/x-www-form-urlencoded",
		[]byte("email=zz-nul%40example.test&message=a%00b&Nick%00name=c%00d"))
	if err != nil {
		t.Fatalf("parse the post: %v", err)
	}
	e := readEnquiry(fields, nil)
	if e.Message != "ab" {
		t.Errorf("the message read %q, want %q", e.Message, "ab")
	}
	if len(e.Extra) != 1 || e.Extra[0] != (Field{Name: "Nickname", Value: "cd"}) {
		t.Errorf("the extra fields read %q, want one Nickname=cd", e.Extra)
	}
}

func TestEachBodyEncodingAFormCanSendIsRead(t *testing.T) {
	var multi bytes.Buffer
	w := multipart.NewWriter(&multi)
	if err := w.WriteField("email", "m@example.com"); err != nil {
		t.Fatal(err)
	}
	file, err := w.CreateFormFile("cv", "cv.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("%PDF")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, contentType string
		body              []byte
		want              string
	}{
		{"urlencoded", "application/x-www-form-urlencoded", []byte("name=A+B&email=u%40example.com"), "u@example.com"},
		{"multipart, file skipped", w.FormDataContentType(), multi.Bytes(), "m@example.com"},
		{"json", "application/json; charset=utf-8", []byte(`{"email":"j@example.com","consent":true}`), "j@example.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fields, err := parseFields(c.contentType, c.body)
			if err != nil {
				t.Fatal(err)
			}
			e := readEnquiry(fields, nil)
			if e.Email != c.want || len(e.Extra) != 0 {
				t.Fatalf("readEnquiry = %+v, want email %q and nothing else", e, c.want)
			}
		})
	}
	if _, err := parseFields("text/plain", []byte("email=x")); err != errUnreadable {
		t.Fatalf("text/plain = %v, want errUnreadable", err)
	}
}

func TestBuilderPayloadsReadIntoFields(t *testing.T) {
	cases := []struct {
		sender, body, wantID string
	}{
		{SenderWebflow, `{"triggerType":"form_submission","payload":{"id":"wf1","data":{"Name":"Ada","Email":"a@example.com"}}}`, "wf1"},
		{SenderTypeform, `{"event_id":"tf1","form_response":{"definition":{"fields":[{"id":"f1","title":"Your name"}]},
			"answers":[{"type":"text","text":"Ada","field":{"id":"f1"}},{"type":"email","email":"a@example.com","field":{"id":"f2","ref":"x"}}],
			"hidden":{"utm_source":"ads"}}}`, "tf1"},
		{SenderTally, `{"eventId":"ty1","eventType":"FORM_RESPONSE","data":{"fields":[{"key":"q1","label":"Name","type":"INPUT_TEXT","value":"Ada"},
			{"key":"q2","label":"Where to reach you","type":"INPUT_EMAIL","value":"a@example.com"}]}}`, "ty1"},
	}
	for _, c := range cases {
		t.Run(c.sender, func(t *testing.T) {
			p, err := readBuilder(c.sender, []byte(c.body))
			if err != nil {
				t.Fatal(err)
			}
			e := readEnquiry(p.Fields, nil)
			if p.ID != c.wantID || e.Email != "a@example.com" || !strings.HasPrefix(e.FullName, "Ada") {
				t.Fatalf("%s read as id %q, %+v", c.sender, p.ID, e)
			}
		})
	}
	if _, err := readBuilder(SenderTally, []byte(`{"eventType":"OTHER"}`)); err != errUnreadable {
		t.Fatalf("a non-submission Tally event = %v, want errUnreadable", err)
	}
}

func TestAnEmailMustLookLikeOne(t *testing.T) {
	for email, want := range map[string]bool{
		"a@example.com": true, "a@b": false, "@example.com": false, "a b@example.com": false,
		"a@example.com,b@example.com": false, "<a@example.com>": false,
	} {
		if got := plausibleEmail(email); got != want {
			t.Errorf("plausibleEmail(%q) = %v, want %v", email, got, want)
		}
	}
}
