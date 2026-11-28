// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
)

// HistoryProfileReader resolves the currently sealed installation profile for link operations.
type HistoryProfileReader func(context.Context, string) (*chatwire.HistoryProfile, error)

// WithHistoryProfiles injects sealed installation-profile reads into channel operations.
func (s *Store) WithHistoryProfiles(read HistoryProfileReader) *Store {
	s.historyProfiles = read
	return s
}

// WireHistoryProfile exposes reviewed limits without returning sealed installation credentials.
func WireHistoryProfile(profile *chatwire.HistoryProfile) *crmcontracts.SlackHistoryProfile {
	if profile == nil {
		return nil
	}
	return &crmcontracts.SlackHistoryProfile{Distribution: crmcontracts.SlackHistoryProfileDistribution(profile.Distribution), HistoryPerMinute: profile.HistoryPerMinute, RepliesPerMinute: profile.RepliesPerMinute, HistoryPageSize: profile.HistoryPageSize, RepliesPageSize: profile.RepliesPageSize, Source: crmcontracts.SlackHistoryProfileSource(profile.Source), RecordedAt: profile.RecordedAt, CanonicalSupported: profile.CanonicalSupported()}
}
