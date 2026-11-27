// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatwire

import "time"

// HistoryProfile records the installation limits used to judge catch-up support.
type HistoryProfile struct {
	Distribution     string    `json:"distribution"`
	HistoryPerMinute int       `json:"history_per_minute"`
	RepliesPerMinute int       `json:"replies_per_minute"`
	HistoryPageSize  int       `json:"history_page_size"`
	RepliesPageSize  int       `json:"replies_page_size"`
	Source           string    `json:"source"`
	RecordedAt       time.Time `json:"recorded_at"`
}

// SlackHistoryProfile derives catch-up limits from the reviewed installation distribution.
func SlackHistoryProfile(distribution string, at time.Time) (*HistoryProfile, error) {
	profile := &HistoryProfile{Distribution: distribution, HistoryPerMinute: 50, RepliesPerMinute: 50, HistoryPageSize: 200, RepliesPageSize: 100, Source: "admin_declared", RecordedAt: at}
	switch distribution {
	case "internal", "marketplace":
	case "non_marketplace":
		profile.HistoryPerMinute = 1
		profile.RepliesPerMinute = 1
		profile.HistoryPageSize = 15
		profile.RepliesPageSize = 15
	default:
		return nil, ErrRequestRejected
	}
	return profile, nil
}

// CanonicalSupported rejects installation profiles that cannot support complete canonical catch-up.
func (p *HistoryProfile) CanonicalSupported() bool {
	return p != nil && (p.Distribution == "internal" || p.Distribution == "marketplace") && p.HistoryPerMinute > 0 && p.HistoryPerMinute <= 50 && p.RepliesPerMinute > 0 && p.RepliesPerMinute <= 50 && p.HistoryPageSize > 0 && p.HistoryPageSize <= 200 && p.RepliesPageSize > 0 && p.RepliesPageSize <= 100 && p.Source == "admin_declared" && !p.RecordedAt.IsZero()
}
