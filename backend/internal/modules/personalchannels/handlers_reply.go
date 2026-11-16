// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PullPersonalChannelReplies implements (POST /personal-channels/replies/pull).
func (h Handlers) PullPersonalChannelReplies(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.PullPersonalChannelRepliesRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	ans, err := h.store.Pull(r.Context(), req.Max)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.PullPersonalChannelRepliesResponse{
		Replies: make([]crmcontracts.PersonalChannelReply, 0, len(ans.Replies)),
		Connection: connectionWire(Connection{
			Enabled: ans.Enabled, RepliesWaiting: ans.RepliesWaiting, OldestReplyWaitingAt: ans.OldestReplyWaitingAt,
		}),
	}
	for _, rep := range ans.Replies {
		out.Replies = append(out.Replies, crmcontracts.PersonalChannelReply{
			DeliveryId: openapi_types.UUID(rep.DeliveryID), Network: rep.Network,
			Recipient: crmcontracts.PersonalChannelParty{Account: rep.Account},
			GroupId:   orNil(rep.GroupID), ReplyTo: orNil(rep.ReplyTo), Body: rep.Body, Attempt: rep.Attempt,
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// AckPersonalChannelReplies implements (POST /personal-channels/replies/ack).
func (h Handlers) AckPersonalChannelReplies(w http.ResponseWriter, r *http.Request, _ crmcontracts.AckPersonalChannelRepliesParams) {
	var req crmcontracts.AckPersonalChannelRepliesRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	acks := make([]Ack, 0, len(req.Acks))
	for _, a := range req.Acks {
		acks = append(acks, Ack{
			DeliveryID: ids.UUID(a.DeliveryId), Outcome: string(a.Outcome),
			ProviderMessageID: deref(a.ProviderMessageId), Reason: deref(a.Reason),
		})
	}
	results, err := h.store.Ack(r.Context(), acks)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.AckPersonalChannelRepliesResponse{Results: make([]crmcontracts.PersonalChannelAckResult, 0, len(results))}
	for _, res := range results {
		out.Results = append(out.Results, crmcontracts.PersonalChannelAckResult{
			DeliveryId: openapi_types.UUID(res.DeliveryID), Result: crmcontracts.PersonalChannelAckResultResult(res.Result),
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
