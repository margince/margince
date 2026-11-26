// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import (
	"slices"
	"sort"
	"strings"
)

// TypeSet is the event types one consumer group's handler acts on. The
// subscriber acks every other type before a dedupe mark is written. The handler
// tests its input against the same value. The zero TypeSet holds every type.
// Held by: TestEveryDeclaringHandlerStartsWithItsGroupsTypeTest (backend/gates/consumergrouptypes_test.go)
type TypeSet struct {
	types map[string]struct{}
}

// NewTypeSet returns the set of the named types.
func NewTypeSet(types ...string) TypeSet {
	set := TypeSet{types: make(map[string]struct{}, len(types))}
	for _, t := range types {
		set.types[t] = struct{}{}
	}
	return set
}

// Declared reports whether the set names its types, rather than taking all.
func (s TypeSet) Declared() bool {
	return s.types != nil
}

// Has reports whether eventType is in the set; the zero set holds every type.
func (s TypeSet) Has(eventType string) bool {
	if s.types == nil {
		return true
	}
	_, ok := s.types[eventType]
	return ok
}

// List returns the declared types, sorted; nil for the zero set.
func (s TypeSet) List() []string {
	if s.types == nil {
		return nil
	}
	out := make([]string, 0, len(s.types))
	for t := range s.types {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// The type sets of the groups whose handler acts on a fixed list. Each
// handler's own type test reads its set here.
var (
	AIBudgetResumeTypes        = NewTypeSet("ai_budget.updated")
	ApprovalNotifyTypes        = NewTypeSet("approval.requested")
	ApprovalNoticeRetractTypes = NewTypeSet("approval.decided")
	AudienceRescopeTypes       = NewTypeSet("activity.updated")
	VCardIngestTypes           = NewTypeSet("activity.captured")
	ContactDataTypes           = NewTypeSet("contact.created")
	OvernightAgentTypes        = NewTypeSet("approval.decided")
	CommissionsTypes           = NewTypeSet("deal.stage_changed")
	PartnerActivationTypes     = NewTypeSet("deal.stage_changed")
	AIActivityTypes            = NewTypeSet("ai_task.state_changed")
	ScheduledSendStopTypes     = NewTypeSet("consent.suppressed", "consent.changed")
	CommitmentSettleTypes      = NewTypeSet("activity.updated", "conversation_claim.changed")
	NoticeCaseOpenTypes        = NewTypeSet("contact.created", "activity.captured")
	CompanyAutoEnrichTypes     = NewTypeSet("company.created", "company.updated", "company.merged")
	ContactAutoEnrichTypes     = NewTypeSet("contact.created", "contact.updated", "contact.restored", "contact.merged")
	CohortPromoteTypes         = NewTypeSet("contact.created", "contact.updated", "contact.restored", "contact.merged")
	CaptureEnrichTypes         = NewTypeSet("activity.captured", "activity.updated", "contact.created")
	StandingLineTypes          = NewTypeSet("activity.captured", "activity.updated")
	StageEvidenceTypes         = NewTypeSet("contract.status_changed", "activity.captured", "activity.updated", "offer.sent", "deal.stage_changed")
	// ActivityNarrowingTypes leave fewer members able to read an activity:
	// archived, restricted, or its audience changed.
	ActivityNarrowingTypes = NewTypeSet("activity.updated", "activity.archived", "retention.restricted")
	// ContextGraphTypes change the content an embedding is built from, on the
	// streams the embedding index reads.
	ContextGraphTypes = typesOn(contextGraphEntities, ".created", ".updated", ".captured", ".promoted", ".merged")
)

// contextGraphEntities are the streams of the records the embedding index holds.
var contextGraphEntities = []string{contactStreamEntity, companyStreamEntity, dealStreamEntity, activityStreamEntity, leadStreamEntity}

// typesOn collects the catalog types that ride one of the entities' streams and
// end in one of the suffixes.
func typesOn(entities []string, suffixes ...string) TypeSet {
	var out []string
	for eventType, spec := range catalog {
		if !slices.Contains(entities, spec.stream) {
			continue
		}
		for _, suffix := range suffixes {
			if strings.HasSuffix(eventType, suffix) {
				out = append(out, eventType)
				break
			}
		}
	}
	return NewTypeSet(out...)
}
