// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package corefeatures reads which main-menu features this installation serves.
package corefeatures

import (
	"fmt"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

var known = []crmcontracts.CoreFeature{
	crmcontracts.CoreFeatureHome, crmcontracts.CoreFeatureContacts, crmcontracts.CoreFeatureCompanies,
	crmcontracts.CoreFeatureLeads, crmcontracts.CoreFeatureDeals, crmcontracts.CoreFeatureProjects,
	crmcontracts.CoreFeatureFilters, crmcontracts.CoreFeatureAnalytics,
}

type Availability struct {
	disabled []crmcontracts.CoreFeature
}

func New(disabled []string) Availability {
	out := Availability{disabled: make([]crmcontracts.CoreFeature, 0, len(disabled))}
	for _, feature := range disabled {
		value := crmcontracts.CoreFeature(feature)
		if !slices.Contains(out.disabled, value) {
			out.disabled = append(out.disabled, value)
		}
	}
	slices.Sort(out.disabled)
	return out
}

func ValidateDisabled(disabled []string) error {
	for _, feature := range disabled {
		if !slices.Contains(known, crmcontracts.CoreFeature(feature)) {
			return fmt.Errorf("unknown core feature %q; choose one of %v", feature, known)
		}
	}
	return nil
}

func (a Availability) Enabled(feature crmcontracts.CoreFeature) bool {
	return !slices.Contains(a.disabled, feature)
}

func (a Availability) Disabled() []crmcontracts.CoreFeature {
	return append([]crmcontracts.CoreFeature{}, a.disabled...)
}

func (a Availability) RecordEnabled(recordType string) bool {
	if recordType == "commission" {
		return a.AreaEnabled("commissions")
	}
	feature, owned := RecordFeature(recordType)
	return !owned || a.Enabled(feature)
}

func RecordFeature(recordType string) (crmcontracts.CoreFeature, bool) {
	switch recordType {
	case "contact":
		return crmcontracts.CoreFeatureContacts, true
	case "company":
		return crmcontracts.CoreFeatureCompanies, true
	case "lead", "sdr_handoff", "reporting_credit":
		return crmcontracts.CoreFeatureLeads, true
	case "deal", "forecast", "forecast_projection", "pipeline", "pipeline_stage", "offer", "offer_line", "deal_room", "product", "offer_template":
		return crmcontracts.CoreFeatureDeals, true
	case "project":
		return crmcontracts.CoreFeatureProjects, true
	case "list", "tag", "saved_filter", "saved_view":
		return crmcontracts.CoreFeatureFilters, true
	case "commission", "report", "report_definition", "reporting_question", "report_edition", "report_schedule", "report_execution", "sales_target":
		return crmcontracts.CoreFeatureAnalytics, true
	default:
		return "", false
	}
}

func (a Availability) AreaEnabled(area string) bool {
	switch area {
	case "contacts", "duplicates":
		return a.Enabled(crmcontracts.CoreFeatureContacts)
	case "companies", "partners":
		return a.Enabled(crmcontracts.CoreFeatureCompanies)
	case "leads":
		return a.Enabled(crmcontracts.CoreFeatureLeads)
	case "deals", "deal_rooms", "buyer_rooms", "legal_reviews", "offers", "pipelines", "catalog":
		return a.Enabled(crmcontracts.CoreFeatureDeals)
	case "projects":
		return a.Enabled(crmcontracts.CoreFeatureProjects)
	case "lists", "tags":
		return a.Enabled(crmcontracts.CoreFeatureFilters)
	case "reports":
		return a.Enabled(crmcontracts.CoreFeatureAnalytics)
	case "forecast", "commissions":
		return a.Enabled(crmcontracts.CoreFeatureAnalytics) && a.Enabled(crmcontracts.CoreFeatureDeals)
	case "worklist":
		return a.Enabled(crmcontracts.CoreFeatureHome)
	default:
		return true
	}
}

func AreaUsesRecordScope(area string) bool {
	switch area {
	case "bulk", "sharing", "imports", "exports", "search":
		return true
	default:
		return false
	}
}
