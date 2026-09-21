// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The connection row on the wire.
//
// It lives beside its handlers rather than inside them: connectors.go is the
// routes, and one mapping read by every one of them is the file's own concern
// rather than any single route's.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// toContractConnection maps a registry connection row onto the wire shape.
// Storage now uses the contract's own status vocabulary (CAP-DDL-2 reconciled
// capture_connection to it), so status is a straight cast — no translation. The
// credential is never present.
func toContractConnection(v capture.ConnectionView) crmcontracts.CaptureConnection {
	c := crmcontracts.CaptureConnection{
		Id:             openapi_types.UUID(v.ID),
		Provider:       crmcontracts.CaptureConnectionProvider(v.Provider),
		Status:         crmcontracts.CaptureConnectionStatus(v.Status),
		Scopes:         v.ProviderScopes,
		WatchExpiresAt: v.WatchExpiresAt,
		AccountLabel:   v.AccountLabel,
		// Carried as the pointer it is: null on the wire is this mailbox
		// following the tenant default, not a field the read forgot.
		SignatureEnrichEnabled: v.SignatureEnrichEnabled,
		MailPosture:            postureOnWire(v.MailPosture),
		ContextTag:             contextTagOnWire(v.ContextTag),
	}
	if c.Scopes == nil {
		c.Scopes = []string{}
	}
	if len(v.Cursor) > 0 {
		s := string(v.Cursor)
		c.SyncCursor = &s
	}
	c.LastSyncedAt = v.LastSyncedAt
	c.LastSyncErrorClass = v.LastErrorClass
	c.SyncFailingSince = v.FailingSince
	c.NextSyncDueAt = v.NextSyncDueAt
	bf := backfillStatusPayload(v.Backfill)
	c.Backfill = &bf
	return c
}
