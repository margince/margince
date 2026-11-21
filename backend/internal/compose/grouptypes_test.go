// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// unusedRuns is a provider run service that panics when called, so a handler
// that passes its nil check and goes on to work is seen.
type unusedRuns struct{ provider.RunService }

// declaredTypeHandlers are the handlers of every group that declares its types.
// They are built with no dependencies, so a handler that goes past its type
// test reaches a nil store or pool and fails.
func declaredTypeHandlers() map[string]func(context.Context, events.Envelope) error {
	return map[string]func(context.Context, events.Envelope) error{
		"cg:ai-budget-resume":        AIBudgetResumeTrigger(nil),
		"cg:approval-notify":         (&ApprovalNotify{}).HandleEvent,
		"cg:approval-notice-retract": (&ApprovalNoticeRetract{}).HandleEvent,
		"cg:audience-rescope":        (&AudienceRescopeGen{}).HandleEvent,
		"cg:vcard-ingest":            (&VCardIngestTrigger{}).HandleEvent,
		"cg:contact-data":            (&ContactDataEnrich{runs: unusedRuns{}}).HandleEvent,
		"cg:overnight-agent":         (&RunnerService{}).HandleEvent,
		"cg:commissions":             (&CommissionGen{}).HandleEvent,
		"cg:partner-activation":      (&PartnerActivation{}).HandleEvent,
		"cg:ai-activity":             (&aiactivity.Consumer{}).HandleEvent,
		"cg:scheduled-send-stop":     (&ScheduledSendStop{}).HandleEvent,
		"cg:commitment-settle":       (&CommitmentSettleTrigger{}).HandleEvent,
		"cg:notice-case-open":        (&NoticeCaseOpen{}).HandleEvent,
		"cg:company-auto-enrich":     (&CompanyAutoEnrichTrigger{}).HandleEvent,
		"cg:contact-auto-enrich":     (&ContactAutoEnrich{}).HandleEvent,
		"cg:cohort-promote":          (&CohortPromoteGen{}).HandleEvent,
		"cg:capture-enrich":          (&CaptureEnrichTrigger{}).HandleEvent,
		"cg:stage-evidence":          (&StageEvidenceTrigger{}).HandleEvent,
		"cg:voice-copy-recheck":      (&VoiceCopyRecheckGen{}).HandleEvent,
		"cg:context-graph":           (&search.EmbedGen{}).HandleEvent,
	}
}

// The subscriber acks every type a group does not declare before its handler
// runs. A handler that acts on a type its group leaves out loses that event
// without an error. So each handler is fed every other type on its streams and
// must ignore it.
func TestEveryHandlerIgnoresTheTypesItsGroupLeavesOut(t *testing.T) {
	handlers := declaredTypeHandlers()
	for _, group := range events.Groups() {
		handler, listed := handlers[group.Name]
		if !group.Types.Declared() {
			if listed {
				t.Errorf("%s is listed here but declares no types; drop it from declaredTypeHandlers", group.Name)
			}
			continue
		}
		delete(handlers, group.Name)
		if !listed {
			t.Errorf("%s declares its types but has no handler in declaredTypeHandlers, so nothing checks them", group.Name)
			continue
		}
		for _, eventType := range events.Types() {
			stream, err := events.StreamFor(eventType)
			if err != nil {
				t.Fatalf("routing %s: %v", eventType, err)
			}
			if group.Types.Has(eventType) || !slices.Contains(group.Streams, stream) {
				continue
			}
			for _, entity := range entityTypesFor(eventType) {
				if err := ignores(handler, eventType, entity); err != nil {
					t.Errorf("%s acts on %s (entity %s), which its group does not declare: %v", group.Name, eventType, entity, err)
				}
			}
		}
	}
	for name := range handlers {
		t.Errorf("declaredTypeHandlers names %s, which is not a consumer group", name)
	}
}

// entityTypesFor is the type's own entity and every record type a family
// stream event can name.
func entityTypesFor(eventType string) []string {
	out := []string{"contact", "company", "deal", "activity", "lead", "approval"}
	if entity, _, err := events.SplitType(eventType); err == nil && !slices.Contains(out, entity) {
		out = append(out, entity)
	}
	return out
}

// ignores runs handler on one event and reports whether it did anything.
func ignores(handler func(context.Context, events.Envelope) error, eventType, entity string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panicked: %v", r)
		}
	}()
	return handler(context.Background(), events.Envelope{
		EventID: ids.NewV7(), Type: eventType, OccurredAt: time.Now(),
		Actor:   events.Actor{Type: "human", ID: "human:" + ids.NewV7().String()},
		Entity:  events.EntityRef{Type: entity, ID: ids.NewV7()},
		Payload: json.RawMessage(`{}`),
	})
}
