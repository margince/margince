// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// An episode retains the precision actually purchased. The first of a month
// may be a storage bound, but must never be described as a known start day.
type employmentEvidence struct {
	CompanyName   string `json:"company_name"`
	CompanyDomain string `json:"company_domain"`
	Role          string `json:"job_title"`
	Started       string `json:"started_at"`
	Ended         string `json:"ended_at"`
	Status        string `json:"employment_status"`
	claimID       ids.UUID
	runID         ids.UUID
	provider      string
	retrieved     time.Time
	key           string
	trustedDomain bool
	superseded    bool
	invalidDates  bool
}

func employmentEpisodes(kind string, raw []byte) ([]employmentEvidence, error) {
	var episodes []employmentEvidence
	if kind == employmentCurrentClaim {
		var current employmentEvidence
		if err := json.Unmarshal(raw, &current); err != nil {
			return nil, fmt.Errorf("reading current employment evidence: %w", err)
		}
		current.Status = employmentCurrent
		episodes = append(episodes, current)
	} else if err := json.Unmarshal(raw, &episodes); err != nil {
		return nil, fmt.Errorf("reading employment history evidence: %w", err)
	}
	out := make([]employmentEvidence, 0, len(episodes))
	for _, e := range episodes {
		e.trustedDomain = e.CompanyDomain != ""
		e.CompanyName = strings.TrimSpace(e.CompanyName)
		if e.CompanyName == "" {
			continue
		}
		e.Role = strings.TrimSpace(e.Role)
		started, _ := preciseEmploymentDate(e.Started)
		ended, _ := preciseEmploymentDate(e.Ended)
		e.invalidDates = (e.Started != "" && started == "") || (e.Ended != "" && ended == "")
		if !e.invalidDates {
			e.Started, e.Ended = started, ended
		}
		if e.Status != employmentCurrent && e.Status != employmentFormer {
			e.Status = employmentUnknown
		}
		if e.CompanyDomain == "" {
			e.CompanyDomain = e.CompanyName
		}
		if domain, err := values.ParseDomain(e.CompanyDomain); err == nil {
			e.CompanyDomain = domain.String()
		} else {
			e.CompanyDomain = ""
		}
		identity := e.CompanyDomain
		if identity == "" {
			identity = strings.ToLower(e.CompanyName)
		}
		digest := sha256.Sum256([]byte(strings.Join([]string{identity, strings.ToLower(e.Role), e.Started, e.Ended}, "\x00")))
		e.key = hex.EncodeToString(digest[:])
		out = append(out, e)
	}
	return out, nil
}

func preciseEmploymentDate(raw string) (string, *time.Time) {
	for _, layout := range []string{time.DateOnly, "2006-01", time.RFC3339} {
		date, err := time.Parse(layout, raw)
		if err != nil {
			continue
		}
		if layout == "2006-01" {
			return date.Format(layout), &date
		}
		return date.Format(time.DateOnly), &date
	}
	return "", nil
}

func employmentPrecision(date string) *string {
	if date == "" {
		return nil
	}
	precision := "day"
	if len(date) == 7 {
		precision = "month"
	}
	return &precision
}

// Date evidence can prove departure; an entry's presence in a history array
// cannot. A month-only departure becomes former only once that month is over.
func (e employmentEvidence) effectiveStatus(today time.Time) string {
	if e.Ended != "" && !e.invalidDates {
		_, end := preciseEmploymentDate(e.Ended)
		if len(e.Ended) == 7 {
			later := end.AddDate(0, 1, 0)
			end = &later
		}
		if !end.After(today) {
			return employmentFormer
		}
	}
	return e.Status
}

func (e employmentEvidence) validDateRange() bool {
	_, start := preciseEmploymentDate(e.Started)
	_, end := preciseEmploymentDate(e.Ended)
	if start == nil || end == nil {
		return !e.invalidDates
	}
	if len(e.Ended) == 7 {
		return start.Before(end.AddDate(0, 1, 0))
	}
	return !start.After(*end)
}

func (e employmentEvidence) companyIdentity() string {
	if e.CompanyDomain != "" {
		return e.CompanyDomain
	}
	return strings.ToLower(strings.TrimSpace(e.CompanyName))
}

func (e employmentEvidence) requiresReview(today time.Time) bool {
	if e.invalidDates || !e.validDateRange() {
		return true
	}
	if e.Status != employmentFormer || e.Ended == "" {
		return false
	}
	if len(e.Ended) == 7 {
		return e.Ended > today.Format("2006-01")
	}
	_, end := preciseEmploymentDate(e.Ended)
	return end != nil && end.After(today)
}
