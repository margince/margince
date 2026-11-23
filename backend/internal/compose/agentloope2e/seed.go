// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentloope2e

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// maxDaysAgo bounds a seeded activity to a year, so a typo cannot place a touch
// in the next century.
const maxDaysAgo = 365

// Seed is the workspace a scenario runs on. The integration lane builds it
// through the product's own API, so every derived column is the one production
// maintains.
type Seed struct {
	Deals []SeedDeal `yaml:"deals"`
}

// SeedDeal is one open deal. Its name is the logical subject the checks use.
type SeedDeal struct {
	Name string `yaml:"name"`
	// OtherSeat gives the deal to a second member. A run acts for one member,
	// so it is neither on that member's queue nor on its slipping list.
	OtherSeat  bool           `yaml:"other_seat"`
	Activities []SeedActivity `yaml:"activities"`
}

// SeedActivity is one inbound email on a deal. DaysAgo is a whole number of
// days before the seeding instant, so the facts hold at any hour of the day.
type SeedActivity struct {
	DaysAgo int    `yaml:"days_ago"`
	Subject string `yaml:"subject"`
	Body    string `yaml:"body"`
}

// Deal answers the seeded deal of that name.
func (s Seed) Deal(name string) (SeedDeal, bool) {
	i := slices.IndexFunc(s.Deals, func(d SeedDeal) bool { return d.Name == name })
	if i < 0 {
		return SeedDeal{}, false
	}
	return s.Deals[i], true
}

func (s Scenario) validateSeed() error {
	if len(s.Seed.Deals) == 0 {
		return errors.New("the scenario seeds no deal, so the agent has no workspace to act on")
	}
	seen := map[string]bool{}
	planted := false
	for _, deal := range s.Seed.Deals {
		switch {
		case !safeName.MatchString(deal.Name):
			return fmt.Errorf("the seeded deal name %q is not a plain logical name", deal.Name)
		case seen[deal.Name]:
			return fmt.Errorf("two seeded deals are named %s", deal.Name)
		}
		seen[deal.Name] = true
		for _, activity := range deal.Activities {
			if activity.DaysAgo < 0 || activity.DaysAgo > maxDaysAgo {
				return fmt.Errorf("deal %s has an activity %d days ago, outside 0 to %d", deal.Name, activity.DaysAgo, maxDaysAgo)
			}
			if strings.TrimSpace(activity.Subject) == "" || strings.TrimSpace(activity.Body) == "" {
				return fmt.Errorf("deal %s has an activity with no subject or body", deal.Name)
			}
			planted = planted || (s.Canary != "" && strings.Contains(activity.Body, s.Canary))
		}
	}
	if s.Canary != "" && !planted {
		return fmt.Errorf("the canary %q is in no seeded activity body, so nothing plants it", s.Canary)
	}
	return s.validateSubjects()
}

// validateSubjects holds every subject a check names to the seed. A worklist
// deal must be the acting member's own, since no other is on their queue or list.
func (s Scenario) validateSubjects() error {
	for _, subject := range append(slices.Clone(s.Worklist), s.Forbidden...) {
		if _, ok := s.Seed.Deal(subject); !ok {
			return fmt.Errorf("the subject %q is no seeded deal", subject)
		}
	}
	for _, subject := range s.Worklist {
		if deal, _ := s.Seed.Deal(subject); deal.OtherSeat {
			return fmt.Errorf("the worklist names %s, which belongs to another seat and is never on this member's list", subject)
		}
	}
	return nil
}
