// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

//nolint:tagliatelle // The fixture implements Google’s calendar wire format.
package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (b *bookingProviderTransport) event(r *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, id, _ := strings.Cut(r.URL.Path, "/events/")
	if r.Method == http.MethodPost || r.Method == http.MethodPatch {
		return b.saveEvent(r, id)
	}
	if id != "" {
		event, found := b.events[id]
		if r.Method == http.MethodDelete {
			delete(b.events, id)
			return bookingProviderResponse(r, `{}`), nil
		}
		response := bookingProviderResponse(r, string(event))
		if !found {
			response.StatusCode = http.StatusNotFound
		}
		return response, nil
	}
	events := make([]json.RawMessage, 0, len(b.events))
	for _, event := range b.events {
		events = append(events, event)
	}
	encoded, err := json.Marshal(struct {
		Zone   string            `json:"timeZone"`
		Events []json.RawMessage `json:"items"`
	}{"UTC", events})
	if err != nil {
		return nil, err
	}
	return bookingProviderResponse(r, string(encoded)), nil
}

func (b *bookingProviderTransport) saveEvent(r *http.Request, id string) (*http.Response, error) {
	if r.URL.Query().Get("sendUpdates") != "all" {
		return nil, fmt.Errorf("calendar write omitted attendee notification")
	}
	var event map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		return nil, err
	}
	if id == "" {
		if err := json.Unmarshal(event["id"], &id); err != nil {
			return nil, err
		}
	}
	encodedID, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	event["id"] = encodedID
	encoded, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	b.events[id] = encoded
	return bookingProviderResponse(r, string(encoded)), nil
}
