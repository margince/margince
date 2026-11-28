// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The WhatsApp webhook edge's job and wiring. The edge itself, and the worker
// that reads a stored item back, live in whatsappedge.

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/whatsappedge"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WhatsAppIngestArgs names one webhook item the edge stored. The worker reads
// the item back by RawCaptureID.
type WhatsAppIngestArgs struct {
	ConnectionID ids.UUID `json:"connection_id"`
	RawCaptureID ids.UUID `json:"raw_capture_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (WhatsAppIngestArgs) Kind() string { return "whatsapp_ingest" }

// RawCapture names the stored item.
func (a WhatsAppIngestArgs) RawCapture() ids.UUID { return a.RawCaptureID }

// Connection names the connection the item was stored under.
func (a WhatsAppIngestArgs) Connection() ids.UUID { return a.ConnectionID }

// WithWhatsAppEdge composes Meta's webhook for WhatsApp Business. A delivery
// stores each item and queues its ingest through inserter. The vault is read per
// call, so this option may come before WithKeyvault.
func WithWhatsAppEdge(inserter *jobs.Runner) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		db := InstallationDB(pool)
		s.whatsAppEventHandlers = capture.NewWhatsAppEventHandlers(&whatsappedge.Receiver{
			Endpoints: capture.NewWhatsAppEdge(db, func() keyvault.Vault { return s.vault }, s.log),
			DB:        db,
			Inserter:  inserter,
			Job: func(workspace, connection, rawCapture ids.UUID) river.JobArgs {
				return WhatsAppIngestArgs{ConnectionID: connection, RawCaptureID: rawCapture}
			},
			MaxAttempts: oneOffJobMaxAttempts,
			Log:         s.log,
		})
	}
}
