// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package credentialmask

import "testing"

// Built by concatenation so the fixture is not itself a scannable secret.
var stripeSecret = "sk_" + "live_51H8xN2eZvKYlo2CFAKEfakefake0000"

func TestMaskTextMasksEveryCredentialShapeAndKeepsTheLabel(t *testing.T) {
	for name, tc := range map[string]struct{ text, want string }{
		"password":           {"password: hunter2secret", "password: [masked]"},
		"passwort":           {"Passwort: Quokka7", "Passwort: [masked]"},
		"kennwort":           {"Kennwort = Quokka7", "Kennwort = [masked]"},
		"pin":                {"PIN: 4711", "PIN: [masked]"},
		"bracket in value":   {"password: ab[cd1234", "password: [masked]"},
		"quoted passphrase":  {`password: "correct horse battery staple" ok`, `password: "[masked]" ok`},
		"single-quoted":      {"pwd='correct horse'", "pwd='[masked]'"},
		"passphrase to eol":  {"Passwort: correct horse battery\nBis bald", "Passwort: [masked]\nBis bald"},
		"secret value":       {"Secret: s3cr3t-Value9 thanks", "Secret: [masked] thanks"},
		"api key value":      {"API key: abc123def456", "API key: [masked]"},
		"prose secret":       {"The secret: preparation wins deals", "The secret: preparation wins deals"},
		"prose token":        {"Token: unit economics", "Token: unit economics"},
		"short token":        {"Token: ab12", "Token: ab12"},
		"value that is [x":   {"password: [hunter2]", "password: [masked]"},
		"pin code":           {"PIN-Code: 4711", "PIN-Code: [masked]"},
		"passcode":           {"Passcode: 993311", "Passcode: [masked]"},
		"token":              {"Token: abcd1234efgh", "Token: [masked]"},
		"label line, value":  {"Your access\nPassword:\n  Quokka7\nthanks", "Your access\nPassword:\n  [masked]\nthanks"},
		"login above":        {"Login: anna.portal\nPassword: Quokka7", "Login: [masked]\nPassword: [masked]"},
		"user above, crlf":   {"Benutzer: anna\r\nKennwort: Quokka7\r\n", "Benutzer: [masked]\r\nKennwort: [masked]\r\n"},
		"url userinfo":       {"postgres://svc:S3cr3tPass@db.internal:5432/prod", "postgres://svc:[masked]@db.internal:5432/prod"},
		"vendor key":         {"use " + stripeSecret + " now", "use [masked] now"},
		"github token":       {"ghp_16C7e42F292c6912E7710c838347Ae178B4a", "[masked]"},
		"aws access key":     {"id AKIAIOSFODNN7EXAMPLE here", "id [masked] here"},
		"jwt":                {"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "[masked]"},
		"pem block":          {"key:\n-----BEGIN PRIVATE KEY-----\nMIIEow\n-----END PRIVATE KEY-----\nbye", "key:\n[masked]\nbye"},
		"prose about it":     {"Reset your password via the portal.", "Reset your password via the portal."},
		"heading over prose": {"Password\nClick the link below to choose one.", "Password\nClick the link below to choose one."},
		"login without pair": {"Login: anna.portal\nSee you soon", "Login: anna.portal\nSee you soon"},
		"a word with pin":    {"Spin: 4711", "Spin: 4711"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := MaskText(tc.text); got != tc.want {
				t.Errorf("MaskText(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// No rule finds a masked value again, so a second pass over stored text changes
// nothing and reports nothing.
func TestMaskedTextMasksToItself(t *testing.T) {
	text := "Login: anna\nPasswort: Quokka7\nPassword:\nQuokka8\nToken: abcd1234\npwd: ab[cd1234\npassword: \"correct horse\"\nPIN: 47 11\nhttps://u:pw12@h.example/ " + stripeSecret
	once := MaskText(text)
	twice, found := Replace([]byte(once), Text, func(string) string { return Marker })
	if string(twice) != once || len(found) != 0 {
		t.Errorf("a second pass found %v in masked text:\nonce  %q\ntwice %q", found, once, twice)
	}
}

// The JSON lane reads an encoded document. A "pin" or "token" key there is the
// payload's own, so only the shared rules apply.
func TestTheJSONLaneSkipsTheTextOnlyRules(t *testing.T) {
	marker := func(kind string) string { return "<" + kind + ">" }
	for name, tc := range map[string]struct{ text, want string }{
		"pin key":       {`{"pin": true}`, `{"pin": true}`},
		"token key":     {`{"token": "abcd1234"}`, `{"token": "abcd1234"}`},
		"password key":  {`{"password": "hunter2"}`, `{"password": "<credential_assignment>"}`},
		"bracket value": {`{"content": "password: ab[cd1234"}`, `{"content": "password: <credential_assignment>"}`},
		"escaped break": {`"Password:\nQuokka7"`, `"Password:\nQuokka7"`},
	} {
		t.Run(name, func(t *testing.T) {
			if got, _ := Replace([]byte(tc.text), JSON, marker); string(got) != tc.want {
				t.Errorf("Replace(%q, JSON) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
