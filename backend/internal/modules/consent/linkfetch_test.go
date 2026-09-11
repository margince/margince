// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the request admits about who sent it.
//
// The rule is one-sided on purpose: every arm turns a person into a machine and
// none turns a machine into a person, because the defect being fixed is
// recording openings nobody made. These cases pin both halves of that — the
// requests that must be refused an opening, and the ones that must still get
// one.

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestARequestThatSaysItIsNotAPersonDoesNotOpenTheLink(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{
			// The standard header, set by the browser and not by the page,
			// which exists so a server can decline to treat a speculative load
			// as a visit.
			name:    "a browser prefetching the link",
			headers: map[string]string{"Sec-Purpose": "prefetch"},
		},
		{
			name:    "Chrome's older spelling",
			headers: map[string]string{"Purpose": "prefetch"},
		},
		{
			name:    "the X- spelling still in the field",
			headers: map[string]string{"X-Purpose": "preview"},
		},
		{
			name:    "Firefox's own link prefetch",
			headers: map[string]string{"X-Moz": "prefetch"},
		},
		{
			name: "a navigation that is nonetheless a prefetch",
			headers: map[string]string{
				"Sec-Fetch-Mode": "navigate",
				"Sec-Fetch-Dest": "document",
				"Sec-Purpose":    "prefetch;prerender",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/v1/public/confirm/cfm_x", nil)
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := WhatFetchedThis(r); got != FetchByAMachine {
				t.Errorf("%s was recorded as a person opening their consent link, which writes a "+
					"line of evidence about a moment that did not happen", tc.name)
			}
		})
	}
}

func TestARequestThatPresentsAsAPersonOpensTheLink(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{
			// What a browser sends when somebody clicks the link in their mail.
			name: "somebody navigating to the page",
			headers: map[string]string{
				"Sec-Fetch-Mode": "navigate",
				"Sec-Fetch-Dest": "document",
			},
		},
		{
			// THE LENIENT DIRECTION, and it is deliberate. An older browser or
			// a proxy that strips headers sends nothing, and refusing to record
			// their opening would swing the defect the other way — a real
			// person's click going unrecorded because of their network.
			name:    "a request that says nothing either way",
			headers: nil,
		},
		{
			// THE CASE THAT MATTERS MOST, and the one that nearly shipped
			// backwards. The confirm page is an SPA: the subject navigates to
			// it, and the PAGE calls this endpoint with fetch(), which carries
			// cors/empty. Judging those would have classified every genuine
			// opening as a machine.
			name: "the confirm page's own call on the subject's behalf",
			headers: map[string]string{
				"Sec-Fetch-Mode": "cors",
				"Sec-Fetch-Dest": "empty",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/v1/public/confirm/cfm_x", nil)
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := WhatFetchedThis(r); got != FetchByAPerson {
				t.Errorf("%s was refused an opening, so a real click goes unrecorded and the "+
					"ask-to-click chain loses its middle", tc.name)
			}
		})
	}
}
