// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rbacObject governs the forms admin surface. A form is integration config,
// so it takes the integrations grant rather than an object of its own.
const rbacObject = "integrations"

// auditEntity is the entity type every audit row of this module names.
const auditEntity = "web_form"

// The senders a form can admit, as stored in web_form.sender.
const (
	SenderBrowser  = "browser"
	SenderServer   = "server"
	SenderWebflow  = "webflow"
	SenderTypeform = "typeform"
	SenderTally    = "tally"
)

// The states a form can be in.
const (
	StateActive = "active"
	StatePaused = "paused"
)

// Bounds held here and by the table's checks.
const (
	maxNameLength           = 200
	maxConsentWordingLength = 2000
	maxSuccessMessage       = 500
	maxAllowedOrigins       = 20
	maxFieldMapEntries      = 50
	maxFieldNameLength      = 100
	maxSenderSecretLength   = 512
	minSenderSecretLength   = 16
)

// Form is one website form. The secret is never on it: SecretConfigured says
// whether one is stored.
type Form struct {
	ID                   ids.UUID
	Name                 string
	Key                  string
	Sender               string
	AllowedOrigins       []string
	SecretRef            *string
	LeadOrigin           *string
	OwnerID              *ids.UUID
	FieldMap             map[string]string
	ConsentRequired      bool
	ConsentPolicyVersion *string
	ConsentWording       *string
	RedirectURL          *string
	SuccessMessage       *string
	State                string
	Version              int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ArchivedAt           *time.Time
}

// SecretConfigured reports whether the form has a signing secret stored.
func (f Form) SecretConfigured() bool { return f.SecretRef != nil }

const formColumns = `id, name, form_key, sender, allowed_origins, secret_ref, lead_origin, owner_id,
	field_map, consent_required, consent_policy_version, consent_wording, redirect_url,
	success_message, state, version, created_at, updated_at, archived_at`

func scanForm(r pgx.Row) (Form, error) {
	var f Form
	err := r.Scan(&f.ID, &f.Name, &f.Key, &f.Sender, &f.AllowedOrigins, &f.SecretRef, &f.LeadOrigin,
		&f.OwnerID, &f.FieldMap, &f.ConsentRequired, &f.ConsentPolicyVersion, &f.ConsentWording,
		&f.RedirectURL, &f.SuccessMessage, &f.State, &f.Version, &f.CreatedAt, &f.UpdatedAt, &f.ArchivedAt)
	return f, err
}

// BadInputError maps to a 422 naming the offending field.
type BadInputError struct {
	Field  string
	Reason string
}

func (e *BadInputError) Error() string { return e.Field + ": " + e.Reason }

func badInput(field, format string, args ...any) error {
	return &BadInputError{Field: field, Reason: fmt.Sprintf(format, args...)}
}

// signedSender reports whether a sender is admitted by a signature, and so
// needs a stored secret.
func signedSender(sender string) bool { return sender != SenderBrowser }

// builderSender reports whether the secret is one a form builder issued,
// pasted in by the admin, rather than one this installation mints.
func builderSender(sender string) bool {
	return sender == SenderWebflow || sender == SenderTypeform || sender == SenderTally
}

// MintsSecret says whether this installation makes the form's secret. A server
// form always gets one. Typeform and Tally get one when none was pasted, since
// both take a typed secret; Webflow issues its own.
func MintsSecret(sender string, pasted *string) bool {
	switch sender {
	case SenderServer:
		return true
	case SenderTypeform, SenderTally:
		return pasted == nil || *pasted == ""
	}
	return false
}

func checkSender(sender string) error {
	switch sender {
	case SenderBrowser, SenderServer, SenderWebflow, SenderTypeform, SenderTally:
		return nil
	}
	return badInput("sender", "must be one of browser, server, webflow, typeform or tally")
}

func checkName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", badInput("name", "is required")
	}
	if utf8.RuneCountInString(trimmed) > maxNameLength {
		return "", badInput("name", "must be at most %d characters", maxNameLength)
	}
	return trimmed, nil
}

// normalizeOrigins reduces each entry to a bare origin, the form the browser
// sends in its Origin header, so a pasted page address still matches.
func normalizeOrigins(raw []string) ([]string, error) {
	if len(raw) > maxAllowedOrigins {
		return nil, badInput("allowed_origins", "may list at most %d websites", maxAllowedOrigins)
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		origin, err := originOf(strings.TrimSpace(entry))
		if err != nil {
			return nil, badInput("allowed_origins", "%q is not a website address such as https://example.com", entry)
		}
		if !slices.Contains(out, origin) {
			out = append(out, origin)
		}
	}
	return out, nil
}

// originOf answers scheme://host[:port] for an absolute http(s) address. Plain
// http is admitted only for localhost, where a site is built before it ships.
func originOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("webforms: %q has no host", raw)
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	if scheme != "https" && (scheme != "http" || !local) {
		return "", fmt.Errorf("webforms: %q is not https", raw)
	}
	return scheme + "://" + host, nil
}

func checkFieldMap(m map[string]string) error {
	if len(m) > maxFieldMapEntries {
		return badInput("field_map", "may map at most %d fields", maxFieldMapEntries)
	}
	for from, to := range m {
		if strings.TrimSpace(from) == "" || utf8.RuneCountInString(from) > maxFieldNameLength {
			return badInput("field_map", "a field name must be 1 to %d characters", maxFieldNameLength)
		}
		if !slices.Contains(leadFieldNames, to) {
			return badInput("field_map", "%q maps to %q, which is not one of %s", from, to, strings.Join(leadFieldNames, ", "))
		}
	}
	return nil
}

func checkRedirect(raw *string) error {
	if raw == nil {
		return nil
	}
	u, err := url.Parse(*raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return badInput(colRedirectURL, "must be an https address")
	}
	return nil
}

// checkSenderSecret holds a pasted builder secret to a length no one guesses.
// Typeform and Tally let the admin type it, so a short one is possible.
func checkSenderSecret(secret *string) error {
	if secret != nil && *secret != "" && utf8.RuneCountInString(*secret) < minSenderSecretLength {
		return badInput("sender_secret", "must be at least %d characters, so it cannot be guessed", minSenderSecretLength)
	}
	return checkBounded("sender_secret", secret, maxSenderSecretLength)
}

func checkBounded(field string, value *string, limit int) error {
	if value != nil && utf8.RuneCountInString(*value) > limit {
		return badInput(field, "must be at most %d characters", limit)
	}
	return nil
}

// checkConsent holds the evidence rule: a form that requires consent must
// say what the visitor agreed to, or the note on the lead proves nothing.
func checkConsent(required bool, wording *string) error {
	if required && (wording == nil || strings.TrimSpace(*wording) == "") {
		return badInput(colConsentWording, "is required when consent_required is true")
	}
	return checkBounded(colConsentWording, wording, maxConsentWordingLength)
}
