// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatedge

import (
	"context"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

// The reasons a chat attachment is not kept. The ones named after the mail
// parser's constants are its, so one reason names one cause in every channel.
const (
	DropTooLarge = mailmap.DropPartTooLarge
	DropTooMany  = mailmap.DropTooManyParts
	// DropRefused is a file the provider or the download policy refused: a
	// missing file, a denied token, a host or redirect outside the allowlist.
	DropRefused = "media_refused"
	// DropChecksum is a download whose bytes differ from the stated checksum.
	DropChecksum = "media_checksum_mismatch"
	// DropUnavailable is a file that stayed unreachable through every retry.
	DropUnavailable = "media_unavailable"
	// DropEmpty is a download with no bytes.
	DropEmpty = mailmap.DropUnreadablePart
)

// Refusal is a download failure that repeating cannot cure. The message still
// files, with the file counted as dropped for Reason.
type Refusal struct {
	Reason string
	Err    error
}

func (r *Refusal) Error() string { return "chat media refused (" + r.Reason + "): " + r.Err.Error() }

func (r *Refusal) Unwrap() error { return r.Err }

// Refused marks err as permanent for the file it names.
func Refused(reason string, err error) error { return &Refusal{Reason: reason, Err: err} }

// MediaRef is one file a message carried, as the network read it from the
// stored item. Nothing is fetched until the worker is about to file.
type MediaRef struct {
	// Ordinal is the file's position among all the message's attachments, those
	// that carry no file included, so it does not shift between deliveries.
	Ordinal int
	// Name and DeclaredType are the sender's claims. Neither is trusted.
	Name, DeclaredType string
	// Download fetches up to limit bytes for the connection. It returns a
	// Refusal for a failure repeating cannot cure and any other error for one
	// that may pass.
	Download func(ctx context.Context, connection ids.UUID, limit int64) ([]byte, error)
}

const (
	// filingReserve is what a job keeps for filing after its downloads. A
	// download that runs into it is cut off, so the message still files.
	filingReserve = 20 * time.Second
	// downloadCap bounds all of one message's downloads when the job has no deadline.
	downloadCap = 2 * time.Minute
)

// downloadContext gives the downloads their own deadline, ahead of the job's by
// the filing reserve. A job with little time left gives the downloads half of it.
func downloadContext(ctx context.Context) (context.Context, context.CancelFunc) {
	budget := downloadCap
	if deadline, ok := ctx.Deadline(); ok {
		left := time.Until(deadline)
		budget = min(budget, max(left-filingReserve, left/2))
	}
	return context.WithTimeout(ctx, budget)
}

// attach downloads each file of the message into its Parts and tallies what was
// not kept into its PartDrops. A failure that may pass is returned, except on the
// job's last attempt, where the file is dropped so the message still files.
func attach(ctx context.Context, connection ids.UUID, refs []MediaRef, lastAttempt bool, msg *connector.ChatMessage) error {
	if len(refs) == 0 {
		return nil
	}
	fetchCtx, cancel := downloadContext(ctx)
	defer cancel()
	drops := map[string]int{}
	budget := int64(extension.MaxInboundMessageBytes)
	for i, ref := range refs {
		if i >= extension.MaxInboundFiles {
			drops[DropTooMany] += len(refs) - i
			break
		}
		body, err := ref.Download(fetchCtx, connection, min(int64(extension.MaxInboundFileBytes), budget))
		var refusal *Refusal
		switch {
		case errors.As(err, &refusal):
			drops[refusal.Reason]++
		case err != nil && lastAttempt:
			drops[DropUnavailable]++
		case err != nil:
			return err
		case len(body) == 0:
			drops[DropEmpty]++
		default:
			budget -= int64(len(body))
			sniffed := extension.SniffContentType(body)
			msg.Parts = append(msg.Parts, connector.Part{
				Ordinal:      ref.Ordinal,
				Filename:     extension.SafeFilename(ref.Name, ref.Ordinal),
				ContentType:  sniffed,
				DeclaredType: extension.DeclaredTypeDisagreement(ref.DeclaredType, sniffed),
				Body:         body,
			})
		}
	}
	for _, reason := range []string{DropTooMany, DropTooLarge, DropRefused, DropChecksum, DropUnavailable, DropEmpty} {
		if n := drops[reason]; n > 0 {
			msg.PartDrops = append(msg.PartDrops, connector.PartDrop{Reason: reason, Count: n})
		}
	}
	return nil
}

// MediaClients are the clients that download a chat message's files. A nil
// client is the network's own, aimed at its real hosts. A test passes one aimed
// at a local server.
type MediaClients struct {
	WhatsApp *whatsapp.Client
	Zalo     *zalo.Client
}
