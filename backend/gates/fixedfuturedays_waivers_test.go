// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import "github.com/margince/margince/backend/internal/shared/gatekit"

// The reasons a fixed day after fixedDateHorizon is safe in an integration
// test. Each waiver below names one of them.
const (
	dayUnderPinnedClock = "the test pins the product's clock near this day, so the writer compares the day with that " +
		"instant and never with today"
	dayIsCalendarFact = "the day is a calendar fact under test, such as a zone shift or a quarter boundary, and no " +
		"writer compares it with today"
	dayIsOpaquePayload = "the day is payload the test hashes, echoes, counts or stores raw; no writer compares it " +
		"with today"
	dayStoredEitherSide = "the writer takes this day in the past as readily as in the future, so the test reads " +
		"the same after the day passes"
	dayBoundsARead = "the day bounds a read or filter over rows seeded at fixed days, and nothing compares it with " +
		"today"
	dayOnAClosedRow = "the row is already decided or closed, and its expiry or due day acts only on an open row, so " +
		"the day is never compared with today"
)

// fixedFutureDayWaivers ratifies each fixed day an integration test may spell
// after fixedDateHorizon, keyed by file and day.
var fixedFutureDayWaivers = gatekit.Waive(map[string]string{
	"internal/compose/analyticsmcpquestions_forecast_integration_test.go 2038-02-01": dayUnderPinnedClock,
	"internal/compose/approvalpreviewbooking_integration_test.go 2027-01-12":         dayIsOpaquePayload,
	"internal/compose/approvalpreviewbookingconsent_integration_test.go 2027-01-12":  dayIsOpaquePayload,
	"internal/compose/approvalpreviewinvitation_integration_test.go 2027-01-12":      dayIsOpaquePayload,
	"internal/compose/approvalpreviewmail_integration_test.go 2027-01-12":            dayIsOpaquePayload,
	"internal/compose/approvalpreviewprojectcreate_integration_test.go 2030-01-02":   dayIsOpaquePayload,
	"internal/compose/approvalpreviewprojectcreate_integration_test.go 2030-06-30":   dayIsOpaquePayload,
	"internal/compose/approvalstageconsent_integration_test.go 2027-01-12":           dayIsOpaquePayload,
	"internal/compose/commitmentadmission_integration_test.go 2027-01-01":            dayUnderPinnedClock,
	"internal/compose/companyrollupperiod_integration_test.go 2026-12-20":            dayUnderPinnedClock,
	"internal/compose/historyputback_integration_test.go 2026-12-01":                 dayStoredEitherSide,
	"internal/compose/historyputback_integration_test.go 2026-12-09": "a task due day the writer takes on either side of today, and a contact " +
		"title that only spells an instant as text",
	"internal/compose/homerepairverdict_context_integration_test.go 2030-01-01":                 dayStoredEitherSide,
	"internal/compose/homerepairverdict_integration_test.go 2030-01-01":                         dayUnderPinnedClock,
	"internal/compose/integration/analyticsreadroutes_integration_test.go 2036-02-01":           dayUnderPinnedClock,
	"internal/compose/integration/analyticsreadroutes_integration_test.go 2036-02-10":           dayUnderPinnedClock,
	"internal/compose/integration/analyticsreadroutes_integration_test.go 2036-02-14":           dayBoundsARead,
	"internal/compose/integration/analyticsreadroutes_integration_test.go 2036-04-01":           dayIsCalendarFact,
	"internal/compose/integration/collections/segmentvocabulary_integration_test.go 2026-12-31": dayBoundsARead,
	"internal/compose/integration/collections/segmentvocabulary_integration_test.go 2027-06-01": dayStoredEitherSide,
	"internal/compose/integration/forecastsnapshotlens_integration_test.go 2038-02-01":          dayUnderPinnedClock,
	"internal/compose/integration/forecastsnapshotlens_integration_test.go 2038-02-14":          dayBoundsARead,
	"internal/compose/integration/forecastsnapshotlens_integration_test.go 2038-02-20":          dayIsOpaquePayload,
	"internal/compose/integration/forecastsnapshotrefs_integration_test.go 2036-02-01":          dayUnderPinnedClock,
	"internal/compose/integration/forecastsnapshotrefs_integration_test.go 2036-02-10":          dayUnderPinnedClock,
	"internal/compose/integration/forecastsnapshotrefs_integration_test.go 2036-02-14":          dayBoundsARead,
	"internal/compose/integration/forecastsnapshotrefs_integration_test.go 2037-02-01":          dayUnderPinnedClock,
	"internal/compose/integration/forecastsnapshotrefs_integration_test.go 2037-02-14":          dayBoundsARead,
	"internal/compose/integration/listimportorigin_integration_test.go 2026-12-20":              dayUnderPinnedClock,
	"internal/compose/integration/nosavechanges_http_integration_test.go 2026-12-31":            dayStoredEitherSide,
	"internal/compose/integration/projectdateorder_integration_test.go 2026-12-01":              dayStoredEitherSide,
	"internal/compose/integration/queryplan_integration_test.go 2026-12-01":                     dayIsOpaquePayload,
	"internal/compose/integration/renewal_reminder_integration_test.go 2027-07-20":              dayUnderPinnedClock,
	"internal/compose/integration/renewalpreview_integration_test.go 2026-12-06":                dayUnderPinnedClock,
	"internal/compose/integration/renewalpreview_integration_test.go 2026-12-20":                dayUnderPinnedClock,
	"internal/compose/integration/renewalpreview_integration_test.go 2027-01-05":                dayUnderPinnedClock,
	"internal/compose/integration/renewalpreview_integration_test.go 2027-03-01":                dayUnderPinnedClock,
	"internal/compose/integration/searchstanding_integration_test.go 2026-12-12":                dayIsOpaquePayload,
	"internal/compose/integration/searchstanding_integration_test.go 2027-01-30":                dayIsOpaquePayload,
	"internal/compose/privacyreview/unlinkedapproval_integration_test.go 2030-01-01":            dayOnAClosedRow,
	"internal/compose/report_closemonth_integration_test.go 2026-11-30":                         dayBoundsARead,
	"internal/compose/report_closemonth_integration_test.go 2026-12-01":                         dayBoundsARead,
	"internal/compose/reporting_calendar_integration_test.go 2027-01-01":                        dayBoundsARead,
	"internal/compose/reporting_history_integration_test.go 2026-12-31":                         dayUnderPinnedClock,
	"internal/compose/reporting_meetingprecision_integration_test.go 2027-01-31":                dayUnderPinnedClock,
	"internal/compose/reporting_meetingprecision_integration_test.go 2027-02-03":                dayUnderPinnedClock,
	"internal/compose/reporting_name_integration_test.go 2026-12-31":                            dayUnderPinnedClock,
	"internal/compose/reporting_review_integration_test.go 2026-12-03":                          dayUnderPinnedClock,
	"internal/compose/requestproofbackfill_integration_test.go 2029-01-03":                      dayUnderPinnedClock,
	"internal/compose/requestproofexpiry_integration_test.go 2026-12-31":                        dayUnderPinnedClock,
	"internal/compose/requestproofexpiry_integration_test.go 2027-01-01":                        dayUnderPinnedClock,
	"internal/compose/requestproofexpiry_integration_test.go 2029-01-03":                        dayUnderPinnedClock,
	"internal/compose/requestproofexpiry_integration_test.go 2029-12-31":                        dayUnderPinnedClock,
	"internal/compose/requestproofexpiry_integration_test.go 2030-01-02":                        dayUnderPinnedClock,
	"internal/compose/requestproofexpiry_integration_test.go 2030-01-20":                        dayUnderPinnedClock,
	"internal/compose/requestproofexpiry_integration_test.go 2030-02-01":                        dayUnderPinnedClock,
	"internal/compose/requesttask/requesttask_integration_test.go 2026-12-03":                   dayStoredEitherSide,
	"internal/modules/activities/activityintent_integration_test.go 2027-10-31":                 dayIsCalendarFact,
	"internal/modules/activities/scheduling_intent_integration_test.go 2027-03-28":              dayIsCalendarFact,
	"internal/modules/activities/scheduling_intent_integration_test.go 2027-06-01":              dayIsCalendarFact,
	"internal/modules/capture/registrywatchrebind_integration_test.go 2027-01-01":               dayIsOpaquePayload,
	"internal/modules/consent/rightscase_integration_test.go 2026-12-15":                        dayIsCalendarFact,
	"internal/modules/consent/rightscase_integration_test.go 2027-01-15":                        dayIsCalendarFact,
	"internal/modules/customfields/candidates_integration_test.go 2026-12-20":                   dayBoundsARead,
	"internal/modules/customfields/candidates_integration_test.go 2026-12-24":                   dayIsCalendarFact,
	"internal/modules/customfields/candidates_integration_test.go 2026-12-31":                   dayBoundsARead,
	"internal/modules/customfields/candidates_integration_test.go 2027-01-01":                   dayBoundsARead,
	"internal/modules/customfields/candidates_integration_test.go 2027-01-10":                   dayIsCalendarFact,
	"internal/modules/customfields/candidates_integration_test.go 2027-01-15":                   dayBoundsARead,
	"internal/modules/customfields/candidates_integration_test.go 2027-03-01":                   dayIsCalendarFact,
	"internal/modules/forecasting/callhistory_integration_test.go 2031-08-12":                   dayStoredEitherSide,
	"internal/modules/forecasting/callhistory_integration_test.go 2032-09-12":                   dayStoredEitherSide,
	"internal/modules/forecasting/callhistory_integration_test.go 2032-12-12":                   dayStoredEitherSide,
	"internal/modules/forecasting/callhistory_integration_test.go 2033-11-12":                   dayStoredEitherSide,
	"internal/modules/forecasting/callhistory_integration_test.go 2034-08-12":                   dayStoredEitherSide,
	"internal/modules/forecasting/callhistory_integration_test.go 2035-05-12":                   dayStoredEitherSide,
	"internal/modules/forecasting/snapshot_integration_test.go 2027-03-08":                      dayUnderPinnedClock,
	"internal/modules/forecasting/snapshotrefs_integration_test.go 2042-02-01":                  dayUnderPinnedClock,
	"internal/modules/forecasting/snapshotrefs_integration_test.go 2043-02-01":                  dayUnderPinnedClock,
	"internal/modules/forecasting/snapshotrefs_integration_test.go 2044-02-01":                  dayUnderPinnedClock,
	"internal/modules/forecasting/snapshotrefs_integration_test.go 2046-02-01":                  dayUnderPinnedClock,
	"internal/modules/forecasting/snapshotrefs_integration_test.go 2046-02-14":                  dayUnderPinnedClock,
	"internal/modules/privacy/auditredaction_integration_test.go 2030-01-01":                    dayIsOpaquePayload,
	"internal/modules/privacy/linkedapprovalreview_integration_test.go 2030-01-01":              dayOnAClosedRow,
	"internal/modules/privacy/linkedhistoricalcases_integration_test.go 2030-01-01":             dayOnAClosedRow,
	"internal/modules/privacy/linkedproof_integration_test.go 2030-01-01":                       dayOnAClosedRow,
})
