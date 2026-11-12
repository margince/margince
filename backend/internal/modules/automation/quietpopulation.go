// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type QuietPopulation struct {
	PipelineID ids.UUID              `json:"pipeline_id"`
	ListID     ids.UUID              `json:"customer_list_id"`
	Readers    []principal.Principal `json:"-"`
}

type QuietCandidates struct {
	Records []EntityAnchor
	Total   int
}

type QuietPopulationScan interface {
	QuietCandidates(context.Context, QuietPopulation, time.Time, int) (QuietCandidates, error)
	ValidateQuietPopulation(context.Context, QuietPopulation) error
}

func quietPopulation(raw json.RawMessage) (QuietPopulation, error) {
	var population QuietPopulation
	if len(raw) == 0 {
		return population, nil
	}
	err := json.Unmarshal(raw, &population)
	return population, err
}

func (p QuietPopulation) configured() bool { return !p.PipelineID.IsZero() || !p.ListID.IsZero() }

func (s *AutomationStore) WithQuietPopulationScan(scan QuietPopulationScan) *AutomationStore {
	s.quiet = scan
	return s
}

func (h Handlers) WithQuietPopulationScan(scan QuietPopulationScan) Handlers {
	h.automations = h.automations.WithQuietPopulationScan(scan)
	return h
}

func (s *AutomationStore) quietPreview(ctx context.Context, stored Automation, in AutomationPreviewInput, readers ...principal.Principal) (AutomationPreviewResult, bool, error) {
	key, raw := stored.Key, stored.Params
	if in.Key != nil {
		key = *in.Key
	}
	daysFor, supported := activityScanHandlers[key]
	if !supported {
		return AutomationPreviewResult{}, false, nil
	}
	if in.Params != nil {
		var err error
		raw, err = json.Marshal(in.Params)
		if err != nil {
			return AutomationPreviewResult{}, true, err
		}
	}
	population, err := quietPopulation(raw)
	if err != nil || !population.configured() {
		return AutomationPreviewResult{}, err != nil, err
	}
	population.Readers = readers
	entry, ok := CatalogEntryByKey(key)
	if !ok {
		return AutomationPreviewResult{}, true, apperrors.ErrNotFound
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return AutomationPreviewResult{}, true, err
	}
	if err := entry.Validate(params); err != nil {
		return AutomationPreviewResult{}, true, err
	}
	window, err := previewWindow(in.WindowDays)
	if err != nil {
		return AutomationPreviewResult{}, true, err
	}
	if s.quiet == nil {
		return AutomationPreviewResult{}, true, errors.New("automation: quiet population scan is not configured")
	}
	days, err := daysFor(raw)
	if err != nil {
		return AutomationPreviewResult{}, true, err
	}
	candidates, err := s.quiet.QuietCandidates(ctx, population, s.now().AddDate(0, 0, -days), previewSampleLimit)
	if err != nil {
		return AutomationPreviewResult{}, true, err
	}
	result := AutomationPreviewResult{
		Kind: crmcontracts.AutomationPreviewKindPopulation, MatchesNowAvailable: true,
		MatchesNow: candidates.Total, WindowDays: window,
	}
	for _, record := range candidates.Records {
		result.Sample = append(result.Sample, record.Ref.ID)
	}
	return result, true, nil
}

func (s *TimeScanner) scanActivityInstance(ctx context.Context, h workflow.Handler, inst automationInstance, ws ids.UUID, now time.Time, daysFor clockDaysExtractor) error {
	population, err := quietPopulation(inst.params)
	if err != nil {
		return err
	}
	if !population.configured() {
		return scanInstanceCandidates(ctx, s.scan, h, inst, ws, now, s.engine.runOne, daysFor)
	}
	scan, ok := s.scan.(QuietPopulationScan)
	if !ok {
		return errors.New("automation: quiet population scan is not configured")
	}
	reader, err := ownerContext(ctx, s.engine.resolver, ws, inst.owner)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return err
	}
	days, err := daysFor(inst.params)
	if err != nil {
		return err
	}
	candidates, err := scan.QuietCandidates(reader, population, now.AddDate(0, 0, -days), clockScanBatchLimit)
	if err != nil {
		return err
	}
	return runActivityCandidates(ctx, h, inst, ws, now, s.engine.runOne, candidates.Records)
}
