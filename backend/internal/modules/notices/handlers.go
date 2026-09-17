// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the notices transport: the reader's own centre and the settling
// of it, the delivery settings behind it, and one coach raising a notice for a
// teammate.
//
// Every read and every write here is the ACTING contact's own, and none of them
// takes a seat as a parameter — another contact's notices cannot be expressed on
// this wire. The transport carries no authorization of its own: the store asks
// the principal question once, for all four of these and for the lane.
type Handlers struct {
	store *Store
	mates Teammates
}

// NewHandlers binds the transport to its store and to the membership question
// coaching is gated on.
func NewHandlers(store *Store, mates Teammates) Handlers {
	return Handlers{store: store, mates: mates}
}

// RaiseNotice records one colleague's coaching nudge to a teammate.
func (h Handlers) RaiseNotice(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.RaiseNoticeRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	var note string
	if req.Note != nil {
		note = *req.Note
	}
	notice, err := h.store.RaiseCoachNotice(
		r.Context(), h.mates, ids.From[ids.UserKind](ids.UUID(req.RecipientUserId)), req.Kind, note)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, noticeWire(notice))
}

// noticeWire is the raised notice as the contract returns it. An empty body is
// ABSENT rather than an empty string: the coach added no note, which is a
// different fact from adding a blank one.
func noticeWire(n Notice) crmcontracts.Notice {
	out := crmcontracts.Notice{
		Id:        openapi_types.UUID(n.ID),
		Kind:      crmcontracts.NoticeKind(n.Kind),
		Subject:   n.Subject,
		CreatedAt: n.CreatedAt,
	}
	if n.Body != "" {
		body := n.Body
		out.Body = &body
	}
	return out
}

// MarkNoticeRead settles one notice for the acting contact.
func (h Handlers) MarkNoticeRead(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if err := h.store.MarkRead(r.Context(), ids.UUID(id)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListNotices answers one window of the acting contact's notification centre.
//
// An absent limit travels as zero rather than as the contract's default spelled
// a second time here: the store treats a limit it was not given as the full
// page, and copying the number into the transport is where the two spellings
// start to disagree.
func (h Handlers) ListNotices(w http.ResponseWriter, r *http.Request, params crmcontracts.ListNoticesParams) {
	var limit int
	if params.Limit != nil {
		limit = *params.Limit
	}
	var cursor string
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	page, err := h.store.ListFor(r.Context(), limit, cursor)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, centrePageWire(page))
}

// MarkAllNoticesRead clears the acting contact's centre in one act.
//
// The store answers how many lines the reader could SEE, and this wire carries
// no number: the contract answers 204, so the figure is dropped here rather than
// rendered into a body whose only honest reading would be "the notifications you
// could see" — a caller wanting a count re-reads the centre, where unread_count
// is already the reader's own.
func (h Handlers) MarkAllNoticesRead(w http.ResponseWriter, r *http.Request) {
	if _, err := h.store.MarkAllRead(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListNotificationPreferences answers the acting contact's own delivery
// settings, one entry per class the product sends.
func (h Handlers) ListNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	chosen, err := h.store.MyNotificationPreferences(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, preferenceListWire(chosen))
}

// SaveNotificationPreference records where the acting contact wants ONE class
// delivered and answers their whole set, so a screen rendering one row per class
// cannot take a stale copy of the others from its own memory.
func (h Handlers) SaveNotificationPreference(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.SaveNotificationPreferenceJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	chosen, err := h.store.SaveNotificationPreference(r.Context(), string(req.Class), string(req.Delivery))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, preferenceListWire(chosen))
}

// centrePageWire is one window of the centre as the contract returns it.
//
// The token is ABSENT on a final page rather than empty, because the contract
// makes that absence a claim — nothing was left behind — and an empty string
// beside it is a page a client can ask for and never receive.
func centrePageWire(page CentrePage) crmcontracts.NotificationPage {
	items := make([]crmcontracts.NotificationItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, centreItemWire(item))
	}
	out := crmcontracts.NotificationPage{Items: items, UnreadCount: page.UnreadCount}
	if page.NextCursor != "" {
		next := page.NextCursor
		out.NextCursor = &next
	}
	return out
}

// centreItemWire is one line of the centre.
//
// Kind travels as the plain string the row holds and never as NoticeKind: that
// vocabulary is the coaching one and a system flow's kind is deliberately absent
// from it, so a cast would publish a value the enum does not admit.
func centreItemWire(item CentreItem) crmcontracts.NotificationItem {
	out := crmcontracts.NotificationItem{
		Id:        openapi_types.UUID(item.ID),
		Kind:      item.Kind,
		Subject:   item.Subject,
		CreatedAt: item.CreatedAt,
		ReadAt:    item.ReadAt,
		Origin:    item.Origin,
	}
	if item.Body != "" {
		body := item.Body
		out.Body = &body
	}
	// Both halves or neither, which is what Named answers: half a target cannot
	// be routed to a screen, and the contract declares the pair required.
	if item.Target.Named() {
		out.Target = &struct {
			Id   openapi_types.UUID `json:"id"` //nolint:staticcheck // matches the generated NotificationItem.Target shape
			Type string             `json:"type"`
		}{Id: openapi_types.UUID(item.Target.ID), Type: item.Target.Type}
	}
	return out
}

// preferenceListWire is the seat's whole set as both the read and the save
// answer it — one shape, because a client that renders the list must not have to
// tell which call produced it.
func preferenceListWire(chosen []Preference) crmcontracts.NotificationPreferenceList {
	items := make([]crmcontracts.NotificationPreference, 0, len(chosen))
	for _, p := range chosen {
		items = append(items, crmcontracts.NotificationPreference{
			Class:    crmcontracts.NotificationClass(p.Class),
			Delivery: crmcontracts.NotificationDelivery(p.Delivery),
			Chosen:   p.Chosen,
		})
	}
	return crmcontracts.NotificationPreferenceList{Items: items}
}
