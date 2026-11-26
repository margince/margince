// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// territories are the territorial-test values, each one a Territory the
// engine decides. AnySender is the empty Territory, so it has a name here.
var territories = map[string]messaging.Territory{
	"any_sender":       messaging.AnySender,
	"sender_here":      messaging.SenderHere,
	"sender_elsewhere": messaging.SenderElsewhere,
}

// apply lays the keys this layer states over r. Instruments and citations add
// to the layers below; every other key replaces what they said, and Compile
// then checks the result is no looser.
func (l layerFile) apply(name string, r *messaging.Rules) error {
	if l.Jurisdiction != nil {
		if r.Jurisdiction != "" && string(r.Jurisdiction) != *l.Jurisdiction {
			return fmt.Errorf("%s states jurisdiction %q over %q; a layer cannot move a pack to another country",
				name, *l.Jurisdiction, string(r.Jurisdiction))
		}
		r.Jurisdiction = jurisdiction.Code(*l.Jurisdiction)
	}
	set(&r.Version, l.Version)
	if err := l.addInstruments(name, r); err != nil {
		return err
	}
	if err := l.addCitations(name, r); err != nil {
		return err
	}
	l.applyWindows(r)
	l.applyFlags(r)
	if l.MarketingMode != nil {
		r.MarketingMode = messaging.MarketingMode(*l.MarketingMode)
	}
	set(&r.SubjectPrefix, l.SubjectPrefix)
	set(&r.DisclosureLanguage, l.DisclosureLanguage)
	set(&r.ClaimsNeedProof, l.ClaimsNeedProof)
	if l.ConsentRequestFirstFor != nil {
		r.ConsentRequestFirstFor = convert(*l.ConsentRequestFirstFor, func(s string) messaging.PurposeClass {
			return messaging.PurposeClass(s)
		})
	}
	if l.FrequencyCap != nil {
		r.FrequencyCap = &messaging.FrequencyCap{Messages: l.FrequencyCap.Messages, Window: l.FrequencyCap.Window}
	}
	if l.MarketingExceptions != nil {
		r.MarketingExceptions = convert(*l.MarketingExceptions, exceptionEntry.rule)
	}
	if l.Disclosures != nil {
		r.Disclosures = convert(*l.Disclosures, disclosureEntry.rule)
	}
	return nil
}

func (l layerFile) applyWindows(r *messaging.Rules) {
	set(&r.ReplyWindow, l.ReplyWindow)
	set(&r.DealFollowUpWindow, l.DealFollowUpWindow)
	set(&r.OptOutAcknowledgementWithin, l.OptOutAcknowledgementWithin)
	set(&r.OptOutAcknowledgementUTCOffset, l.OptOutAcknowledgementUTCOffset)
	set(&r.OptOutRecordsKeep, l.OptOutRecordsKeep)
}

func (l layerFile) applyFlags(r *messaging.Rules) {
	set(&r.ConsentOnly, l.ConsentOnly)
	set(&r.RefusalSectionLast, l.RefusalSectionLast)
	set(&r.ForeignAdvertiserNeedsLocalAgency, l.ForeignAdvertiserNeedsLocalAgency)
	set(&r.OptOutAcknowledgement, l.OptOutAcknowledgement)
	set(&r.DataProtectionFollowsRecipient, l.DataProtectionFollowsRecipient)
	set(&r.NoLegitimateInterests, l.NoLegitimateInterests)
	set(&r.RecordConfirmationWithoutNews, l.RecordConfirmationWithoutNews)
	set(&r.ConsentConfirmationNeedsRequest, l.ConsentConfirmationNeedsRequest)
}

func (l layerFile) addInstruments(name string, r *messaging.Rules) error {
	for _, in := range l.Instruments {
		for _, have := range r.Instruments {
			if have.Name == in.Name {
				return fmt.Errorf("%s states instrument %q, which a layer below already states", name, in.Name)
			}
		}
		r.Instruments = append(r.Instruments, messaging.Instrument{
			Name: in.Name, EffectiveFrom: in.Effective, ExpiresFrom: in.Expires,
			Provision: in.Provision, SourceURL: in.SourceURL,
			TextURL: in.TextURL, TextDigest: in.TextDigest, TextFetchedAt: in.TextFetched,
			CheckedByHandAt: in.CheckedByHand,
		})
	}
	return nil
}

func (l layerFile) addCitations(name string, r *messaging.Rules) error {
	for _, c := range l.Citations {
		if c.TerritorialTest == nil {
			return fmt.Errorf("%s: citation %q states no territorial-test; write any_sender, sender_here or sender_elsewhere",
				name, c.Code)
		}
		territory, known := territories[*c.TerritorialTest]
		if !known {
			return fmt.Errorf("%s: citation %q has territorial-test %q, which the engine cannot decide; "+
				"write any_sender, sender_here or sender_elsewhere", name, c.Code, *c.TerritorialTest)
		}
		r.Citations = append(r.Citations, messaging.Citation{
			Code: c.Code, Territory: territory, Instrument: c.Instrument, Provision: c.Provision, SourceURL: c.SourceURL,
		})
	}
	return nil
}

func (l layerFile) provenance() messaging.Provenance {
	var p messaging.Provenance
	set(&p.CreatedAt, l.Created)
	set(&p.SourceCheckedAt, l.SourceChecked)
	set(&p.ReviewedAt, l.Reviewed)
	set(&p.ReviewDue, l.ReviewDue)
	set(&p.Reviewer, l.Reviewer)
	return p
}

func (e exceptionEntry) rule() messaging.MarketingException {
	return messaging.MarketingException{
		Kind:                         messaging.ExceptionKind(e.Kind),
		RequiresSaleEvidence:         e.RequiresSaleEvidence,
		RequiresCollectionTimeOptOut: e.RequiresCollectionTimeOptOut,
		RequiresSimilarity:           e.RequiresSimilarity,
		RequiresNoObjection:          e.RequiresNoObjection,
	}
}

func (d disclosureEntry) rule() messaging.Disclosure {
	return messaging.Disclosure{
		Kind:          messaging.DisclosureKind(d.Kind),
		MarketingOnly: d.MarketingOnly,
		Means:         convert(d.Means, func(s string) messaging.ContactMeans { return messaging.ContactMeans(s) }),
	}
}

// set copies a stated key over the layer below; an absent one leaves it.
func set[T any](into *T, from *T) {
	if from != nil {
		*into = *from
	}
}

// convert maps a stated list, keeping a stated empty list empty and not nil.
func convert[From, To any](in []From, each func(From) To) []To {
	if in == nil {
		return nil
	}
	out := make([]To, 0, len(in))
	for _, v := range in {
		out = append(out, each(v))
	}
	return out
}

// earliest answers the earlier of two instants. A zero one either states no
// date, or, when zeroWins, a step that never happened.
func earliest(a, b time.Time, zeroWins bool) time.Time {
	switch {
	case a.IsZero() || b.IsZero():
		if zeroWins {
			return time.Time{}
		}
		if a.IsZero() {
			return b
		}
		return a
	case b.Before(a):
		return b
	}
	return a
}
