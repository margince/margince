// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// handOnAccess asks, as the rule's new owner, whether each record its stored
// params name is one that owner may use. The params themselves are not judged:
// a param that names no record, or no longer parses, names nothing to check.
// A record the new owner cannot use refuses with ErrPermissionDenied, and one
// that is gone with ErrNotFound, never as a param error.
func (s *AutomationStore) handOnAccess(ctx context.Context, entry CatalogEntry, params map[string]any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	switch {
	case isQuietRule(entry.Key):
		population, err := quietPopulation(raw)
		if err != nil || !population.configured() {
			return nil
		}
		if s.quiet == nil {
			return errors.New("automation: quiet population scan is not configured")
		}
		return accessRefusal(s.quiet.ValidateQuietPopulation(ctx, population))
	case entry.Key == stageChangeCreateTaskName || entry.Key == stageChangeNotifyName:
		return s.stageAccess(ctx, params)
	case entry.Key == inquiryRecipeName:
		policy, err := ParseInquiryPolicy(raw)
		if err != nil {
			return nil
		}
		if s.inquiries == nil {
			return errors.New("automation: website inquiry handling is not configured")
		}
		return accessRefusal(s.inquiries.Validate(ctx, policy))
	case entry.ValidateRefs != nil:
		return s.listAccess(ctx, raw)
	}
	return nil
}

func isQuietRule(key string) bool {
	_, quiet := activityScanHandlers[key]
	return quiet
}

func (s *AutomationStore) stageAccess(ctx context.Context, params map[string]any) error {
	scope, err := decodeStageParams(params)
	if err != nil || len(scope.StageIDs) == 0 {
		return nil
	}
	if s.stages == nil {
		return errors.New("automation: stage lookup is not configured")
	}
	for _, stage := range scope.StageIDs {
		if _, _, err := s.stages.StageSemantic(ctx, stage); err != nil {
			return err
		}
	}
	return nil
}

// listRef is a list a list rule names, and whether the rule writes to it.
type listRef struct {
	id        ids.UUID
	shortlist bool
}

func (s *AutomationStore) listAccess(ctx context.Context, raw []byte) error {
	rule, err := decodeListRuleParams(raw)
	if err != nil {
		return nil
	}
	if s.lists == nil {
		return errors.New("automation: lists are not configured")
	}
	named := []listRef{{id: rule.ListID}}
	if rule.ShortlistID != nil {
		named = append(named, listRef{id: *rule.ShortlistID, shortlist: true})
	}
	for _, ref := range named {
		found, err := s.lists.Find(ctx, ref.id)
		if err != nil {
			return err
		}
		if found.Archived {
			return apperrors.ErrNotFound
		}
		if ref.shortlist {
			if err := s.lists.CheckShortlist(ctx, ref.id, found.EntityType); err != nil {
				return accessRefusal(err)
			}
		}
	}
	return nil
}

// accessRefusal keeps an access error as it is. A reference check answers a
// record that is no longer usable with a param error, which becomes a refusal.
func accessRefusal(err error) error {
	if _, isParam := errors.AsType[*ParamError](err); isParam {
		return apperrors.ErrPermissionDenied
	}
	return err
}
