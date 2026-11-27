// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func messengerSendInput(in crmcontracts.MessengerSendRequest) (MessengerSend, error) {
	if err := httperr.RequireBodyID("request_id", ids.UUID(in.RequestId)); err != nil {
		return MessengerSend{}, err
	}
	ack := ids.Nil
	if in.AcknowledgedRequestId != nil {
		ack = ids.UUID(*in.AcknowledgedRequestId)
		if err := httperr.RequireBodyID("acknowledged_request_id", ack); err != nil {
			return MessengerSend{}, err
		}
	}
	return MessengerSend{AcknowledgedRequestID: ack, RequestID: ids.UUID(in.RequestId), GrantVersion: in.GrantVersion, Conversation: in.ConversationId, Root: optionalString(in.RootId), Text: in.Text, AudienceKey: in.AudienceKey}, nil
}

func deliveryReviewInput(in crmcontracts.ReviewChannelDeliveryRequest) (DeliveryReviewInput, error) {
	if err := httperr.RequireBodyID("link_id", ids.UUID(in.LinkId)); err != nil {
		return DeliveryReviewInput{}, err
	}
	deliveries := make([]ids.UUID, 0, len(in.DeliveryIds))
	for _, id := range in.DeliveryIds {
		deliveries = append(deliveries, ids.UUID(id))
	}
	return DeliveryReviewInput{Link: ids.UUID(in.LinkId), Action: string(in.Action), Acknowledged: in.Acknowledged, Deliveries: deliveries}, nil
}

// TeamInstallationID validates the selected installation before channel setup proceeds.
func TeamInstallationID(in crmcontracts.ConnectTeamsChannelsJSONRequestBody) (ids.UUID, error) {
	id := ids.UUID(in.InstallationId)
	return id, httperr.RequireBodyID("installation_id", id)
}
