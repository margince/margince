// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The three readings the company page leads with: whose move it is, when each
// side last wrote, and how the relationship stands in parts.
//
// They sit together because they answer one question between them — is this
// account healthy, and whose turn is it — and because each replaced a piece of
// the old header: a 0-100 score nobody could scale, and a single "last touch"
// date that hid which side it belonged to.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// readLastTouch is LastTouchFor over a set of one, so the header and a queue
// row naming this account read the same statement.
func (a *assembly) readLastTouch() error {
	touched, err := LastTouchFor(a.ctx, a.tx, []ids.CompanyID{a.companyID}, a.now, a.opts)
	if err != nil {
		return err
	}
	// An absent direction is how the null reaches the wire: nothing of that
	// direction was ever captured.
	touch := touched[a.companyID]
	a.out.LastInboundAt = touch.InboundAt
	a.out.LastOutboundAt = touch.OutboundAt
	return nil
}

// readStateStrip is the three readings the overview leads with (AC-company-13).
//
// The account half needs no grant beyond the company the caller already
// read. The other two are gated independently and answer NULL rather than a
// zero when refused: "no open deals" and "you may not see the deals" are
// different facts, and only one of them is about the account.
func (a *assembly) readStateStrip() error {
	in, err := a.suggestionInputsOnce()
	if err != nil {
		return err
	}
	strip := crmcontracts.Company360StateStrip{}
	if lc := a.out.Company.Lifecycle; lc != nil {
		strip.Account.Lifecycle = crmcontracts.Company360StateStripAccountLifecycle(*lc)
	}
	if types := a.out.Company.RelationshipTypes; types != nil {
		for _, relType := range *types {
			strip.Account.RelationshipTypes = append(strip.Account.RelationshipTypes,
				crmcontracts.Company360StateStripAccountRelationshipTypes(relType))
		}
	}

	if in.timeline {
		strip.Engagement = new(struct {
			LastInboundAt  *time.Time                                       `json:"last_inbound_at,omitempty"`
			LastOutboundAt *time.Time                                       `json:"last_outbound_at,omitempty"`
			State          crmcontracts.Company360StateStripEngagementState `json:"state"`
		})
		strip.Engagement.LastInboundAt = a.out.LastInboundAt
		strip.Engagement.LastOutboundAt = a.out.LastOutboundAt
		strip.Engagement.State = engagementState(in, a.now)
	}
	if in.pipeline {
		strip.Commercial = new(struct {
			BaseCurrency          *string             `json:"base_currency,omitempty"`
			ConvertedCount        int                 `json:"converted_count"`
			FxAsOf                *openapi_types.Date `json:"fx_as_of,omitempty"`
			NextCloseOn           *openapi_types.Date `json:"next_close_on,omitempty"`
			OpenCount             int                 `json:"open_count"`
			OpenPipelineMinorBase *int                `json:"open_pipeline_minor_base,omitempty"`
			PricedCount           int                 `json:"priced_count"`
			StalledCount          int                 `json:"stalled_count"`
		})
		fillCommercialStrip(strip.Commercial, in.open)
	}
	if in.contracts {
		strip.Contracts = new(struct {
			ActiveCount              int                 `json:"active_count"`
			AnnualizedValueMinorBase *int                `json:"annualized_value_minor_base,omitempty"`
			BaseCurrency             *string             `json:"base_currency,omitempty"`
			CancellationEffectiveOn  *openapi_types.Date `json:"cancellation_effective_on,omitempty"`
			CancellationPending      bool                `json:"cancellation_pending"`
			NearestRenewalOn         *openapi_types.Date `json:"nearest_renewal_on,omitempty"`
			PricedCount              *int                `json:"priced_count,omitempty"`
			TotalBasisValueMinorBase *int                `json:"total_basis_value_minor_base,omitempty"`
		})
		fillContractStrip(strip.Contracts, in.contractStrip)
	}
	// The worst thing standing open, or nothing. Null covers BOTH "no signal"
	// and "you may not read signals" on purpose: a strip that said "nothing is
	// wrong" to someone who cannot look would be answering a question it has
	// no standing to answer. The signals card is where the difference shows.
	facts, err := a.signalFactsOnce()
	if err != nil {
		return err
	}
	if facts.HasWorst {
		strip.Signal = new(struct {
			Kind     string                                          `json:"kind"`
			Severity crmcontracts.Company360StateStripSignalSeverity `json:"severity"`
			Summary  string                                          `json:"summary"`
		})
		strip.Signal.Kind = facts.Worst.Kind
		strip.Signal.Severity = crmcontracts.Company360StateStripSignalSeverity(facts.Worst.Severity)
		strip.Signal.Summary = facts.Worst.Summary
	}
	a.out.StateStrip = &strip
	return nil
}

// fillCommercialStrip writes the open-pipeline reading. Null, not zero, when
// nothing could be priced: a zero would claim a pipeline that exists and is
// worth nothing, where the truth is that no open deal here carries a figure
// this page can convert (plan §4.2).
func fillCommercialStrip(out *struct {
	BaseCurrency          *string             `json:"base_currency,omitempty"`
	ConvertedCount        int                 `json:"converted_count"`
	FxAsOf                *openapi_types.Date `json:"fx_as_of,omitempty"`
	NextCloseOn           *openapi_types.Date `json:"next_close_on,omitempty"`
	OpenCount             int                 `json:"open_count"`
	OpenPipelineMinorBase *int                `json:"open_pipeline_minor_base,omitempty"`
	PricedCount           int                 `json:"priced_count"`
	StalledCount          int                 `json:"stalled_count"`
}, open pipeline,
) {
	out.OpenCount = open.OpenCount
	out.StalledCount = len(open.Stalled)
	out.PricedCount = open.Priced
	if open.Priced > 0 {
		value := int(open.ValueMinorBase)
		out.OpenPipelineMinorBase = &value
		out.BaseCurrency = &open.BaseCurrency
		out.ConvertedCount = open.Converted
		if open.FXAsOf != nil {
			out.FxAsOf = &openapi_types.Date{Time: *open.FXAsOf}
		}
	}
	if open.NextCloseOn != nil {
		out.NextCloseOn = &openapi_types.Date{Time: *open.NextCloseOn}
	}
}

// readHealth decomposes the relationship into the parts a reader can act on
// (AC-company-3), replacing the single 0-100 score the header used to lead
// with. That number was PO-F-3's MAX over the contacts, so one talkative
// contact spoke for the whole account; each part here names a fact instead.
//
// Every part is null when it cannot be computed rather than zero: zero is a
// claim about the ACCOUNT, and "nobody has written" and "you may not read the
// mail" are different answers.
func (a *assembly) readHealth() error {
	if err := auth.Require(a.ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	strengths, err := a.contactStrengths()
	if err != nil {
		return err
	}
	health := crmcontracts.Company360Health{}

	if inbound := a.out.LastInboundAt; inbound != nil {
		days := elapsed.Days(*inbound, a.now)
		health.DaysSinceLastInbound = &days
	}

	// The account's real surface: contacts who have actually interacted, not
	// contacts on file. A roster of ten who have never replied is not ten ways
	// in.
	active := 0
	var inbound90, outbound90 int
	for _, contact := range strengths {
		if contact.Strength.LastInteraction == nil {
			continue
		}
		active++
		inbound90 += contact.Strength.Inbound90d
		outbound90 += contact.Strength.Outbound90d
	}
	health.ActiveContacts = &active
	if total := inbound90 + outbound90; total > 0 {
		balance := float32(inbound90) / float32(total)
		health.ReplyBalance = &balance
	}
	// One contact carrying the whole relationship is the one shape a rep can
	// fix before it costs them the account, so it is named rather than scored.
	if len(strengths) > 0 {
		single := active == 1
		health.SingleThreaded = &single
	}

	// Commitments read out of conversations, either side's, that are still
	// owed and not yet a task. Null for a reader who may not see them.
	open, readable, err := a.svc.contacts.CountAccountCommitments(a.ctx, a.tx, a.companyID.UUID)
	if err != nil {
		return err
	}
	if readable {
		health.OpenCommitments = &open
	}

	lastMeeting, nextMeeting, err := a.readableMeetings()
	if err != nil {
		return err
	}
	health.LastMeetingAt = lastMeeting

	touch := relstrength.ReadInTouch(a.out.LastInboundAt, lastMeeting, nextMeeting, a.now)
	rateHealthDimensions(&health, a.out.StateStrip, touch)

	a.out.Health = &health
	return nil
}
