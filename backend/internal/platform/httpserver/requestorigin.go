// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"context"
	"net/http"
	"net/netip"
)

// RequestOrigin is the device and address a request came from, kept as
// evidence on the row the request opened: a session, a confirmed consent.
// A nil field was not learned, which a row stores as NULL, never as "".
type RequestOrigin struct {
	UserAgent *string
	IP        *string
}

type requestOriginKey struct{}

// WithRequestOrigin binds r's origin to ctx, so the store that writes the row
// records it without an HTTP type in its signature. The address is ClientIP's,
// the one the throttles trust.
func WithRequestOrigin(ctx context.Context, r *http.Request) context.Context {
	var origin RequestOrigin
	if ua := r.UserAgent(); ua != "" {
		origin.UserAgent = &ua
	}
	if addr, err := netip.ParseAddr(ClientIP(r)); err == nil {
		ip := addr.Unmap().String()
		origin.IP = &ip
	}
	return context.WithValue(ctx, requestOriginKey{}, origin)
}

// RequestOriginFrom returns the bound origin, or the zero value when the row
// is written with no request behind it.
func RequestOriginFrom(ctx context.Context) RequestOrigin {
	origin, ok := ctx.Value(requestOriginKey{}).(RequestOrigin)
	if !ok {
		return RequestOrigin{}
	}
	return origin
}
