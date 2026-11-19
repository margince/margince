// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The records around a deal that a review pack carries: its agreements, its
// latest touches and its open tasks. They belong to other modules, so compose
// injects one reader that runs under the caller's own principal.
//
// The pack freezes ids and dates. A title or subject is read when a reviewer
// opens the pack. A record this reader cannot open is left out, so mail
// outside their audience stays out.

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PacketContract is one agreement filed against the deal. Title is read at
// view time and never stored.
type PacketContract struct {
	ContractID           ids.UUID   `json:"contract_id"`
	ContractNumber       *string    `json:"contract_number,omitempty"`
	Status               string     `json:"status"`
	StartsOn             *time.Time `json:"starts_on,omitempty"`
	EndsOn               *time.Time `json:"ends_on,omitempty"`
	RenewalOn            *time.Time `json:"renewal_on,omitempty"`
	CancellationNoticeOn *time.Time `json:"cancellation_notice_on,omitempty"`
	NoticePeriodDays     *int       `json:"notice_period_days,omitempty"`
	AutoRenew            *bool      `json:"auto_renew,omitempty"`
	Title                string     `json:"-"`
}

// PacketTouch is one recent call, meeting, message or note on the deal.
// Subject is read at view time and never stored.
type PacketTouch struct {
	ActivityID ids.UUID  `json:"activity_id"`
	Kind       string    `json:"kind"`
	OccurredAt time.Time `json:"occurred_at"`
	Subject    *string   `json:"-"`
}

// PacketTask is a task that was open on the deal at the request. Subject is
// read at view time and never stored.
type PacketTask struct {
	ActivityID ids.UUID   `json:"activity_id"`
	DueAt      *time.Time `json:"due_at,omitempty"`
	Subject    *string    `json:"-"`
}

// LegalReviewRecords reads the records around a deal for a pack. Every call
// runs under the caller's principal and opens its own connection. A record
// the caller may not read is absent, and a caller who may not read the whole
// kind gets an empty answer.
type LegalReviewRecords interface {
	// ReviewContracts lists the deal's agreements the caller can open, with
	// their titles.
	ReviewContracts(ctx context.Context, dealID ids.UUID) ([]PacketContract, error)
	// ReviewActivity lists the deal's latest touches and open tasks the
	// caller can read.
	ReviewActivity(ctx context.Context, dealID ids.UUID) ([]PacketTouch, []PacketTask, error)
	// OpenableActivity answers which of these activities the caller can read
	// now, with their subjects. The subject is nil when the content is withheld.
	OpenableActivity(ctx context.Context, dealID ids.UUID, activityIDs []ids.UUID) (map[ids.UUID]*string, error)
}

// WithLegalReviewRecords wires the reader of the deal's agreements and
// activity.
func (h Handlers) WithLegalReviewRecords(records LegalReviewRecords) Handlers {
	wired := *h.store
	wired.reviewRecords = records
	h.store = &wired
	return h
}

// packetRecords is everything read outside this module before the pack's
// transaction opens.
type packetRecords struct {
	documents []PacketDocument
	contracts []PacketContract
	touches   []PacketTouch
	tasks     []PacketTask
}

// emptied makes the sections of a new pack non-nil. An empty one then stores
// as [], and only a pack from before the sections decodes them as nil.
func (r packetRecords) emptied() packetRecords {
	if r.contracts == nil {
		r.contracts = []PacketContract{}
	}
	if r.touches == nil {
		r.touches = []PacketTouch{}
	}
	if r.tasks == nil {
		r.tasks = []PacketTask{}
	}
	return r
}

// readPacketRecords reads the pack's outside records under the requester.
func (s *Store) readPacketRecords(ctx context.Context, dealID ids.UUID) (packetRecords, error) {
	var out packetRecords
	var err error
	if out.documents, err = s.reviewDocs.ReviewDocuments(ctx, dealID); err != nil {
		return out, err
	}
	if out.contracts, err = s.reviewRecords.ReviewContracts(ctx, dealID); err != nil {
		return out, err
	}
	out.touches, out.tasks, err = s.reviewRecords.ReviewActivity(ctx, dealID)
	return out, err
}

// recordSight is what this reader can open of a pack's outside records now.
type recordSight struct {
	documents map[ids.UUID]DocumentName
	contracts map[ids.UUID]string
	activity  map[ids.UUID]*string
}

// readRecordSight asks each owner what the reader may open, after the pack's
// transaction, because each reader opens its own connection.
func (s *Store) readRecordSight(ctx context.Context, p reviewPacket) (recordSight, error) {
	var out recordSight
	var err error
	if out.documents, err = s.reviewDocs.OpenableDocuments(ctx, packetDocumentIDs(p)); err != nil {
		return out, err
	}
	current, err := s.reviewRecords.ReviewContracts(ctx, p.Deal.ID)
	if err != nil {
		return out, err
	}
	out.contracts = make(map[ids.UUID]string, len(current))
	for _, c := range current {
		out.contracts[c.ContractID] = c.Title
	}
	out.activity, err = s.reviewRecords.OpenableActivity(ctx, p.Deal.ID, packetActivityIDs(p))
	return out, err
}

func packetActivityIDs(p reviewPacket) []ids.UUID {
	out := make([]ids.UUID, 0, len(p.Touches)+len(p.Tasks))
	for _, t := range p.Touches {
		out = append(out, t.ActivityID)
	}
	for _, t := range p.Tasks {
		out = append(out, t.ActivityID)
	}
	return out
}

// recordsWire renders the frozen records for a reader who may read the deal.
// A record the reader cannot open is left out, and a pack that predates the
// sections leaves them absent.
func recordsWire(p reviewPacket, sight recordSight, out *crmcontracts.LegalReviewPacket) {
	if p.Contracts == nil {
		// A pack asked for before these sections existed has none of the keys.
		return
	}
	contracts := make([]crmcontracts.LegalReviewPacketContract, 0, len(p.Contracts))
	for _, c := range p.Contracts {
		title, ok := sight.contracts[c.ContractID]
		if !ok {
			continue
		}
		contracts = append(contracts, crmcontracts.LegalReviewPacketContract{
			Id: openapiUUID(c.ContractID), Title: title, ContractNumber: c.ContractNumber, Status: crmcontracts.LegalReviewPacketContractStatus(c.Status),
			StartsOn: wireDate(c.StartsOn), EndsOn: wireDate(c.EndsOn), RenewalOn: wireDate(c.RenewalOn),
			CancellationNoticeOn: wireDate(c.CancellationNoticeOn),
			NoticePeriodDays:     c.NoticePeriodDays, AutoRenew: c.AutoRenew,
		})
	}
	touches := make([]crmcontracts.LegalReviewPacketTouch, 0, len(p.Touches))
	for _, t := range p.Touches {
		if subject, ok := sight.activity[t.ActivityID]; ok {
			touches = append(touches, crmcontracts.LegalReviewPacketTouch{
				ActivityId: openapiUUID(t.ActivityID), Kind: crmcontracts.LegalReviewPacketTouchKind(t.Kind), OccurredAt: t.OccurredAt, Subject: subject,
			})
		}
	}
	tasks := make([]crmcontracts.LegalReviewPacketTask, 0, len(p.Tasks))
	for _, t := range p.Tasks {
		if subject, ok := sight.activity[t.ActivityID]; ok {
			tasks = append(tasks, crmcontracts.LegalReviewPacketTask{
				ActivityId: openapiUUID(t.ActivityID), Subject: subject, DueAt: t.DueAt,
			})
		}
	}
	out.Contracts, out.Touches, out.Tasks = &contracts, &touches, &tasks
}
