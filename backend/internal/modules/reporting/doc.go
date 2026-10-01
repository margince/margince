// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package reporting owns versioned sales reports, targets and scheduled editions.
// Calculations and source visibility are injected by compose; saved record-list
// preferences cannot supply the shared, immutable reporting lifecycle.
//
// Tables owned: reporting_framework, reporting_framework_revision,
// report_definition, report_definition_revision, sales_target,
// sales_target_revision, report_schedule, report_execution, report_edition,
// report_edition_contribution.
package reporting
