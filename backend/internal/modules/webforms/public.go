// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// errBodyTooLarge refuses a post over MaxBody.
var errBodyTooLarge = errors.New("the submission is larger than a form post can be")

// SubmitWebForm implements (POST /public/forms/{form_key}).
func (h Handlers) SubmitWebForm(w http.ResponseWriter, r *http.Request, formKey string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBody))
	if err != nil {
		h.AnswerRefusal(w, r, errBodyTooLarge)
		return
	}
	receipt, err := h.intake.Submit(r.Context(), Arrival{
		FormKey: formKey, ContentType: r.Header.Get("Content-Type"), Header: r.Header, Body: body, Now: h.now(),
	})
	allowReading(w, receipt.AllowOrigin)
	if err != nil {
		h.AnswerRefusal(w, r, err)
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
	message := systemtext.Copy("webform.thanks", systemtext.Locale(r.Context()))
	if receipt.Message != nil {
		message = *receipt.Message
	}
	h.writePage(w, r, http.StatusOK, message)
}

// AnswerRefusal writes a refusal as JSON for a script, or as a page for a
// plain form post. The page is in the language the edge bound for the
// visitor, and names the reason only where the visitor can act on it.
func (h Handlers) AnswerRefusal(w http.ResponseWriter, r *http.Request, err error) {
	if !wantsPage(r) {
		writeErr(w, r, err)
		return
	}
	status := http.StatusInternalServerError
	if fault, ok := httperr.Classify(refusal(err)); ok {
		status = fault.Status
		if fault.RetryAfterSeconds > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(fault.RetryAfterSeconds))
		}
	}
	h.writePage(w, r, status, refusalWords(err, systemtext.Locale(r.Context())))
}

// refusalWords is the visitor's sentence for a refusal. The English reason a
// script reads never reaches the page.
func refusalWords(err error, lang textlang.Lang) string {
	if bad, ok := errors.AsType[*BadInputError](err); ok {
		switch bad.Field {
		case fieldEmail:
			return systemtext.Copy("webform.needs_email", lang)
		case fieldConsent:
			return systemtext.Copy("webform.needs_consent", lang)
		}
	}
	if throttled, ok := errors.AsType[*throttledError](err); ok {
		return fmt.Sprintf(systemtext.Copy("webform.too_many", lang), throttled.wait)
	}
	switch {
	case errors.Is(err, ErrPaused):
		return systemtext.Copy("webform.paused", lang)
	case errors.Is(err, apperrors.ErrNotFound):
		return systemtext.Copy("webform.gone", lang)
	}
	return systemtext.Copy("webform.unavailable_now", lang)
}

// ServePreflight answers a browser's CORS preflight for f, the form the
// throttle resolved. A website on the form's list may post with any
// Content-Type; any other website gets no CORS headers, so its browser refuses
// to send. A key that names no form resolves to a zero Form, which allows none.
func (h Handlers) ServePreflight(w http.ResponseWriter, r *http.Request, f Form) {
	origin := r.Header.Get("Origin")
	if f.AllowsOrigin(origin) {
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
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>{{.Message}}</title>
<style>body{font:16px/1.5 system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;color:#1f2933}</style>
</head><body><p>{{.Message}}</p></body></html>`))

// writePage renders the message through html/template. An admin's success
// message or a reason about the visitor's input is text, never markup. The
// page forbids every script and frame.
func (h Handlers) writePage(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	page := struct{ Lang, Message string }{Lang: string(systemtext.Locale(r.Context())), Message: message}
	if err := pageTemplate.Execute(w, page); err != nil {
		h.store.log.DebugContext(r.Context(), "webforms: writing the response page failed; the visitor's connection is gone", "err", err)
	}
}
