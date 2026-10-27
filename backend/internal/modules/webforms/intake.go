// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MaxBody bounds what one anonymous post may make this installation read. A
// contact form is a few hundred bytes; a builder's payload with its form
// definition is a few kilobytes.
const MaxBody = 64 << 10

// ErrPaused refuses a post to a form its admin paused.
var ErrPaused = errors.New("webforms: this form is paused")

var formKeyGrammar = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// ValidFormKey reports whether a path segment can be a form key at all. The
// edge asks it before metering, so a made-up key never becomes a limiter key.
func ValidFormKey(key string) bool { return formKeyGrammar.MatchString(key) }

// submissionIDGrammar bounds the id a browser snippet sends with its post.
var submissionIDGrammar = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// Landing is one admitted submission, ready to become a lead.
type Landing struct {
	FormID   ids.UUID
	FormName string
	// SourceKey is stable across a sender's retries of one submission, so a
	// second delivery finds the lead and note the first one wrote.
	SourceKey            string
	LeadOrigin           *string
	OwnerID              *ids.UUID
	Enquiry              Enquiry
	ConsentWording       *string
	ConsentPolicyVersion *string
}

// LeadLander writes an admitted submission as a lead and its timeline note.
// compose implements it over the contacts and activities stores; an email a
// lead or contact already holds lands on that record instead.
type LeadLander interface {
	Land(ctx context.Context, l Landing) error
}

// Receipt is what the sender is told. A new lead, an existing one and a
// dropped submission all read the same, so the address tells nobody who is in
// the CRM.
type Receipt struct {
	Message     *string
	RedirectURL *string
	// AllowOrigin is the website whose script may read the response. It is
	// set for a browser form posted from its own website, refusals included.
	AllowOrigin string
}

// Intake admits anonymous posts to a form's address.
type Intake struct {
	store  *Store
	lander LeadLander
}

// NewIntake wires the public intake over the forms store and the lead writer.
func NewIntake(store *Store, lander LeadLander) *Intake {
	return &Intake{store: store, lander: lander}
}

// Submit admits one post and lands it. The caller has bound the workspace and
// a system principal. The form's own admission rule is what decides here.
func (in *Intake) Submit(ctx context.Context, a Arrival) (Receipt, error) {
	f, err := in.store.PublicForm(ctx, a.FormKey)
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{Message: f.SuccessMessage, RedirectURL: f.RedirectURL}
	if origin := a.Header.Get("Origin"); f.AllowsOrigin(origin) {
		receipt.AllowOrigin = origin
	}
	e, key, err := in.admit(ctx, f, a)
	if err != nil || key == "" {
		return receipt, err
	}
	if err := checkEnquiry(f, e); err != nil {
		return receipt, err
	}
	return receipt, in.lander.Land(ctx, Landing{
		FormID: f.ID, FormName: f.Name, SourceKey: sourceKey(f.ID, key),
		LeadOrigin: f.LeadOrigin, OwnerID: f.OwnerID, Enquiry: e,
		ConsentWording: f.ConsentWording, ConsentPolicyVersion: f.ConsentPolicyVersion,
	})
}

// admit runs the form's admission rule and reads the fields. An empty key with
// no error is a browser post the spam checks dropped: answered as received,
// landed nowhere.
func (in *Intake) admit(ctx context.Context, f Form, a Arrival) (Enquiry, string, error) {
	switch {
	case f.Sender == SenderBrowser:
		if err := admitBrowser(f, a); err != nil {
			return Enquiry{}, "", err
		}
		fields, err := parseFields(a.ContentType, a.Body)
		if err != nil {
			return Enquiry{}, "", err
		}
		e := readEnquiry(fields, f.FieldMap)
		if looksAutomated(e, a.Now) {
			return Enquiry{}, "", nil
		}
		return e, browserKey(e), nil
	case f.Sender == SenderServer:
		secret, err := in.store.openSecret(ctx, f)
		if err != nil {
			return Enquiry{}, "", unverified(err)
		}
		nonce, err := verifyServer(secret, a)
		if err != nil {
			return Enquiry{}, "", err
		}
		fields, err := parseFields(a.ContentType, a.Body)
		return readEnquiry(fields, f.FieldMap), nonce, err
	default:
		return in.admitBuilder(ctx, f, a)
	}
}

func (in *Intake) admitBuilder(ctx context.Context, f Form, a Arrival) (Enquiry, string, error) {
	secret, err := in.store.openSecret(ctx, f)
	if err != nil {
		return Enquiry{}, "", unverified(err)
	}
	if err := verifyBuilder(f.Sender, secret, a); err != nil {
		return Enquiry{}, "", err
	}
	payload, err := readBuilder(f.Sender, a.Body)
	if err != nil {
		return Enquiry{}, "", err
	}
	key := payload.ID
	if key == "" {
		sum := sha256.Sum256(a.Body)
		key = hex.EncodeToString(sum[:])
	}
	return readEnquiry(payload.Fields, f.FieldMap), key, nil
}

// unverified folds a secret that cannot be read into the one opaque refusal:
// a missing secret is a form nobody can sign for. Any other failure is the
// installation's, and is reported as one.
func unverified(err error) error {
	if errors.Is(err, errUnverified) || errors.Is(err, apperrors.ErrNotFound) {
		return errUnverified
	}
	return fmt.Errorf("webforms: reading the form's secret: %w", err)
}

// browserKey is the id the snippet minted for this post, so a double click or
// a retried fetch lands once. A post without one is its own submission.
func browserKey(e Enquiry) string {
	if id := e.controls[controlID]; submissionIDGrammar.MatchString(id) {
		return id
	}
	return ids.NewV7().String()
}

// sourceKey is the submission's source_id on the lead and its note. It is the
// form and a digest of the sender's key, so any key fits the column's bound.
func sourceKey(form ids.UUID, key string) string {
	sum := sha256.Sum256([]byte(key))
	return form.String() + ":" + hex.EncodeToString(sum[:16])
}

func checkEnquiry(f Form, e Enquiry) error {
	if e.Email == "" {
		return badInput(fieldEmail, "an email address is required")
	}
	if !plausibleEmail(e.Email) {
		return badInput(fieldEmail, "is not an email address")
	}
	if f.ConsentRequired && !e.Consent {
		return badInput(fieldConsent, "this form needs the consent box ticked")
	}
	return nil
}

// PublicForm answers the live form at an address, for the intake and the CORS
// answer. An absent or archived form is ErrNotFound, a paused one ErrPaused.
// The edge's system principal passes the read gate. A member reaching it is
// held to the forms grant like any other read.
func (s *Store) PublicForm(ctx context.Context, key string) (Form, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return Form{}, err
	}
	if !ValidFormKey(key) {
		return Form{}, apperrors.ErrNotFound
	}
	var f Form
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		f, err = scanForm(tx.QueryRow(ctx,
			"SELECT "+formColumns+" FROM web_form WHERE form_key = $1 AND archived_at IS NULL", key))
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Form{}, apperrors.ErrNotFound
	case err != nil:
		return Form{}, err
	case f.State == StatePaused:
		return Form{}, ErrPaused
	}
	return f, nil
}
