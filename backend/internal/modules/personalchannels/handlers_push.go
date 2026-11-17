// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pushBudget is how long one push may spend landing before it answers the rest
// deferred, well inside the server's response deadline.
const pushBudget = 20 * time.Second

// PushPersonalChannelMessages implements (POST /personal-channels/chats).
func (h Handlers) PushPersonalChannelMessages(w http.ResponseWriter, r *http.Request, _ crmcontracts.PushPersonalChannelMessagesParams) {
	var req crmcontracts.PushPersonalChannelMessagesRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	msgs := make([]PushMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, pushMessageOf(m))
	}
	results, err := h.store.Push(r.Context(), msgs, pushBudget)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, pushWire(results))
}

func pushMessageOf(m crmcontracts.PersonalChannelMessage) PushMessage {
	out := PushMessage{
		MessageID: m.MessageId, GroupID: deref(m.GroupId),
		Network: deref(m.Network), Subject: deref(m.Subject), Body: deref(m.Body),
		From: partyOf(m.From),
	}
	if m.DeliveryId != nil {
		out.DeliveryID = ids.UUID(*m.DeliveryId)
	}
	if m.Direction != nil {
		out.Direction = string(*m.Direction)
	}
	if m.OccurredAt != nil {
		out.OccurredAt = *m.OccurredAt
	}
	if m.To != nil {
		out.To = partyOf(*m.To)
	}
	if m.Participants != nil {
		for _, p := range *m.Participants {
			out.Participants = append(out.Participants, partyOf(p))
		}
	}
	if m.Files != nil {
		for _, f := range *m.Files {
			out.UploadIDs = append(out.UploadIDs, ids.UUID(f.UploadId))
		}
	}
	return out
}

func partyOf(p crmcontracts.PersonalChannelParty) Party {
	return Party{Account: p.Account, Name: deref(p.Name)}
}

func derefInt(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func pushWire(results []PushResult) crmcontracts.PushPersonalChannelMessagesResponse {
	out := crmcontracts.PushPersonalChannelMessagesResponse{Results: make([]crmcontracts.PersonalChannelPushResult, 0, len(results))}
	for _, r := range results {
		row := crmcontracts.PersonalChannelPushResult{MessageId: r.MessageID, Status: crmcontracts.PersonalChannelPushResultStatus(r.Status)}
		if r.ActivityID != nil {
			id := openapi_types.UUID(*r.ActivityID)
			row.ActivityId = &id
		}
		if r.Reason != "" {
			reason := r.Reason
			row.Reason = &reason
		}
		out.Results = append(out.Results, row)
	}
	return out
}
