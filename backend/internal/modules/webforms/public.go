// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// errBodyTooLarge refuses a post over MaxBody.
var errBodyTooLarge = errors.New("the submission is larger than a form post can be")

// defaultThanks is the thank-you text for a form with no message of its own.
const defaultThanks = "Thank you. Your message has been received."

// SubmitWebForm implements (POST /public/forms/{form_key}).
func (h Handlers) SubmitWebForm(w http.ResponseWriter, r *http.Request, formKey string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBody))
	if err != nil {
		h.answerRefusal(w, r, errBodyTooLarge)
		return
	}
	receipt, err := h.intake.Submit(r.Context(), Arrival{
		FormKey: formKey, ContentType: r.Header.Get("Content-Type"), Header: r.Header, Body: body, Now: h.now(),
	})
	allowReading(w, receipt.AllowOrigin)
	if err != nil {
		h.answerRefusal(w, r, err)
		return
	}
	if !wantsPage(r) {
		httperr.WriteJSON(w, http.StatusAccepted, crmcontracts.WebFormReceipt{
			Status: crmcontracts.WebFormReceiptStatusReceived, Message: receipt.Message, RedirectUrl: receipt.RedirectURL,
		})
		return
	}
	if receipt.RedirectURL != nil {
		http.Redirect(w, r, *receipt.RedirectURL, http.StatusSeeOther)
		return
	}
	message := defaultThanks
	if receipt.Message != nil {
		message = *receipt.Message
	}
	h.writePage(w, r, http.StatusOK, message)
}

// answerRefusal writes a refusal as JSON for a script, or as a page for a
// plain form post. The page names the reason only where the visitor can fix
// it: a missing email, an unticked consent.
func (h Handlers) answerRefusal(w http.ResponseWriter, r *http.Request, err error) {
	if !wantsPage(r) {
		writeErr(w, r, err)
		return
	}
	status, message := http.StatusInternalServerError, "This form could not be sent right now."
	if fault, ok := httperr.Classify(refusal(err)); ok {
		status = fault.Status
	}
	if bad, ok := errors.AsType[*BadInputError](err); ok && (bad.Field == fieldEmail || bad.Field == fieldConsent) {
		message = "This form could not be sent: " + bad.Reason + ". Please go back and try again."
	}
	h.writePage(w, r, status, message)
}

// ServePreflight answers a browser's CORS preflight for a form's address. A
// website on the form's list may post with any Content-Type; any other
// website gets no CORS headers, so its browser refuses to send.
func (h Handlers) ServePreflight(w http.ResponseWriter, r *http.Request, formKey string) {
	f, err := h.store.PublicForm(r.Context(), formKey)
	origin := r.Header.Get("Origin")
	if err == nil && f.AllowsOrigin(origin) {
		allowReading(w, origin)
		w.Header().Set("Access-Control-Allow-Methods", http.MethodPost)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
	}
	w.WriteHeader(http.StatusNoContent)
}

// allowReading lets one website's script read this response. No credentials:
// the intake never reads a cookie, so a browser is never told it may send one.
func allowReading(w http.ResponseWriter, origin string) {
	w.Header().Add("Vary", "Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
}

// wantsPage reports a plain HTML form post: a browser navigating, which reads
// a page or follows a redirect, rather than a script reading JSON.
func wantsPage(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html") &&
		!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>{{.}}</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;color:#1f2933}</style>
</head><body><p>{{.}}</p></body></html>`))

// writePage renders the message through html/template. An admin's success
// message or a reason about the visitor's input is text, never markup. The
// page forbids every script and frame.
func (h Handlers) writePage(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := pageTemplate.Execute(w, message); err != nil {
		h.store.log.DebugContext(r.Context(), "webforms: writing the response page failed; the visitor's connection is gone", "err", err)
	}
}
