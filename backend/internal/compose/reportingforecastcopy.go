// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

func reportingForecastReason(frame crmcontracts.ReportingContext, caveat forecasting.LandingCaveat) string {
	if caveat == "" {
		return reporting.ExplanationCopy(frame).Say(reportingForecastWords.Unpriced)
	}
	if phrase, ok := reportingForecastCaveats[caveat]; ok {
		return reporting.ExplanationCopy(frame).Say(phrase)
	}
	return string(caveat)
}
