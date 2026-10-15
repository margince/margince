// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"fmt"
	"net/textproto"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	fieldCustomHeaders  = "custom_headers"
	maxCustomHeaders    = 10
	maxHeaderNameBytes  = 64
	maxHeaderValueBytes = 4 << 10
)

// reservedHeaders are set by Margince or by the HTTP transport on every
// delivery. A custom value would forge the signature, misstate the body, or
// reshape the connection. The value says what governs the header instead.
type reservedHeader struct{ name, why string }

var reservedHeaders = []reservedHeader{
	{"webhook-id", "Margince sets webhook-id"},
	{"webhook-timestamp", "Margince sets webhook-timestamp"},
	{"webhook-signature", "Margince sets webhook-signature"},
	{"x-margince-event", "Margince sets X-Margince-Event"},
	{"user-agent", "Margince sets User-Agent"},
	{"content-type", "the body format sets Content-Type"},
	{"content-length", "the body sets Content-Length"},
	{"content-encoding", "the body sets Content-Encoding"},
	{"host", "the target URL sets Host"},
	{"cookie", "a delivery carries no cookies"},
	{"connection", "the HTTP transport sets Connection"},
	{"keep-alive", "the HTTP transport sets Keep-Alive"},
	{"transfer-encoding", "the HTTP transport sets Transfer-Encoding"},
	{"te", "the HTTP transport sets TE"},
	{"trailer", "the HTTP transport sets Trailer"},
	{"upgrade", "the HTTP transport sets Upgrade"},
	{"expect", "the HTTP transport sets Expect"},
}

// CustomHeaderInput is one header as a create or update names it. A nil
// Value on update keeps the value already stored under that name.
type CustomHeaderInput struct {
	Name  string
	Value *string
}

// storedHeader is one element of webhook_subscription.custom_headers.
type storedHeader struct {
	Name        string `json:"name"`
	SealedValue string `json:"sealed_value"`
}

type openedHeader struct {
	Name  string
	Value string
}

func headerNames(stored []storedHeader) []string {
	names := make([]string, len(stored))
	for i, h := range stored {
		names[i] = h.Name
	}
	return names
}

// headerBinding ties a sealed value to its subscription and header name.
func headerBinding(sub ids.UUID, name string) []byte {
	return []byte(sub.String() + "\x00" + name)
}

// sealHeaders validates the requested set and seals it for sub, carrying
// over the stored ciphertext of every row that names no new value.
func (c *Cipher) sealHeaders(sub ids.UUID, in []CustomHeaderInput, existing []storedHeader) ([]storedHeader, error) {
	if len(in) > maxCustomHeaders {
		return nil, headerError(fmt.Sprintf("a subscription carries at most %d custom headers", maxCustomHeaders))
	}
	kept := make(map[string]string, len(existing))
	for _, h := range existing {
		kept[h.Name] = h.SealedValue
	}
	out := make([]storedHeader, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, h := range in {
		name, err := checkHeaderName(h.Name)
		if err != nil {
			return nil, err
		}
		if seen[name] {
			return nil, headerError(fmt.Sprintf("%s is listed more than once", name))
		}
		seen[name] = true
		sealed, err := c.sealHeaderValue(sub, name, h.Value, kept)
		if err != nil {
			return nil, err
		}
		out = append(out, storedHeader{Name: name, SealedValue: sealed})
	}
	return out, nil
}

func (c *Cipher) sealHeaderValue(sub ids.UUID, name string, value *string, kept map[string]string) (string, error) {
	if value == nil {
		if sealed, ok := kept[name]; ok {
			return sealed, nil
		}
	}
	if value == nil || *value == "" {
		return "", headerError(fmt.Sprintf("%s needs a value", name))
	}
	if len(*value) > maxHeaderValueBytes {
		return "", headerError(fmt.Sprintf("the value of %s is longer than %d KiB", name, maxHeaderValueBytes>>10))
	}
	if !httpguts.ValidHeaderFieldValue(*value) {
		return "", headerError(fmt.Sprintf("the value of %s holds a line break or control character", name))
	}
	return c.sealBound(*value, headerBinding(sub, name))
}

func checkHeaderName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > maxHeaderNameBytes || !httpguts.ValidHeaderFieldName(name) {
		return "", headerError(fmt.Sprintf("%q is not a header name; use letters, digits and dashes, up to %d characters", raw, maxHeaderNameBytes))
	}
	lower := strings.ToLower(name)
	for _, r := range reservedHeaders {
		if r.name == lower {
			return "", headerError(fmt.Sprintf("%s is reserved: %s", name, r.why))
		}
	}
	if strings.HasPrefix(lower, "webhook-") || strings.HasPrefix(lower, "proxy-") {
		return "", headerError(fmt.Sprintf("%s is reserved for the signature and transport headers", name))
	}
	return textproto.CanonicalMIMEHeaderKey(name), nil
}

// openHeaders unseals every value for an attempt. A value that fails to
// open fails the attempt; the delivery never goes out without it.
func (c *Cipher) openHeaders(sub ids.UUID, stored []storedHeader) ([]openedHeader, error) {
	out := make([]openedHeader, len(stored))
	for i, h := range stored {
		v, err := c.openBound(h.SealedValue, headerBinding(sub, h.Name))
		if err != nil {
			return nil, fmt.Errorf("custom header %s could not be opened: %w", h.Name, err)
		}
		if !httpguts.ValidHeaderFieldValue(v) {
			return nil, fmt.Errorf("custom header %s holds a value that cannot be sent", h.Name)
		}
		out[i] = openedHeader{Name: h.Name, Value: v}
	}
	return out, nil
}

func headerError(reason string) *BadInputError {
	return &BadInputError{Field: fieldCustomHeaders, Reason: reason}
}
