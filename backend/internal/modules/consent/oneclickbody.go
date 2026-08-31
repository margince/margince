// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The RFC 8058 request body.
//
// §3.2 says the mailbox provider POSTs "List-Unsubscribe=One-Click", and
// names multipart/form-data as the SHOULD with URL-encoding as the MAY.
// Both are therefore admitted: a check that took only the form encoding
// would turn away a conforming receiver while looking correct against
// Gmail, which happens to send the other one.
//
// Only the URL-encoded form is DECLARED in crm.yaml: naming multipart
// there would class this route as a file upload and oblige it to declare
// a megabyte-scale ceiling, which would misdescribe a body that is one
// short field. The runtime still accepts multipart, which is what keeps
// the RFC's preferred spelling working.
//
// An ABSENT body is admitted too. The product's own unsubscribe page and
// any operator with curl press this endpoint without one, and refusing
// them would break the human path to protect a machine contract. Only a
// body that is present and says something else is refused.

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

const (
	// oneClickField and oneClickValue are the exact pair RFC 8058 §3.2
	// prescribes, spelled as the RFC spells them.
	oneClickField = "List-Unsubscribe"
	oneClickValue = "One-Click"
	// oneClickBodyLimit leaves room for a multipart envelope's boundary
	// and headers around a single short field, and nothing more.
	oneClickBodyLimit = 4096
)

// requireOneClickBody admits an empty body, either RFC 8058 encoding of
// the one-click pair, and nothing else.
func requireOneClickBody(r *http.Request) error {
	raw := strings.TrimSpace(r.Header.Get("Content-Type"))
	if raw == "" {
		// No content type: only an empty body is meaningful, and draining
		// it keeps the connection reusable. A read failure here is the
		// client hanging up on a request already decided, so it changes
		// no verdict — but it is not silently discarded either.
		if _, err := io.Copy(io.Discard, io.LimitReader(r.Body, oneClickBodyLimit)); err != nil {
			slog.DebugContext(r.Context(), "one-click unsubscribe body could not be drained", "error", err)
		}
		return nil
	}
	mediaType, params, err := mime.ParseMediaType(raw)
	if err != nil {
		return httperr.Validation("body", "not_one_click", "unreadable content type on a one-click unsubscribe")
	}
	switch mediaType {
	case "multipart/form-data":
		return oneClickFromMultipart(r, params["boundary"])
	case "application/x-www-form-urlencoded":
		return oneClickFromForm(r)
	default:
		return httperr.Validation("body", "not_one_click", "a one-click unsubscribe carries the RFC 8058 form body")
	}
}

// oneClickFromForm reads the URL-encoded spelling.
func oneClickFromForm(r *http.Request) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, oneClickBodyLimit+1))
	if err != nil {
		return httperr.Validation("body", "not_one_click", "could not read the one-click body")
	}
	if len(body) > oneClickBodyLimit {
		return httperr.Validation("body", "too_large", "a one-click unsubscribe body is a single short field")
	}
	if len(body) == 0 {
		return nil
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return httperr.Validation("body", "not_one_click", "a one-click unsubscribe carries the RFC 8058 form body")
	}
	return admitOneClick(values.Get(oneClickField))
}

// oneClickFromMultipart reads the spelling RFC 8058 names first.
func oneClickFromMultipart(r *http.Request, boundary string) error {
	if boundary == "" {
		return httperr.Validation("body", "not_one_click", "a multipart one-click unsubscribe needs its boundary")
	}
	form, err := multipart.NewReader(io.LimitReader(r.Body, oneClickBodyLimit+1), boundary).
		ReadForm(oneClickBodyLimit)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return httperr.Validation("body", "not_one_click", "a one-click unsubscribe carries the RFC 8058 form body")
	}
	// RemoveAll deletes whatever multipart spilled to disk. It cannot
	// change this request's verdict, but a failure means temp files are
	// accumulating under an anonymous endpoint, which is worth saying out
	// loud rather than dropping.
	defer func() {
		if err := form.RemoveAll(); err != nil {
			slog.WarnContext(r.Context(), "one-click unsubscribe left its multipart scratch files behind",
				"error", err)
		}
	}()
	if len(form.Value[oneClickField]) == 0 {
		return admitOneClick("")
	}
	return admitOneClick(form.Value[oneClickField][0])
}

// admitOneClick holds the field's value to the one the RFC names.
func admitOneClick(value string) error {
	if strings.TrimSpace(value) == oneClickValue {
		return nil
	}
	return httperr.Validation("body", "not_one_click", "a one-click unsubscribe carries the RFC 8058 form body")
}
