// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// GetMyWorkingHours implements GET /me/working-hours.
func (h Handlers) GetMyWorkingHours(w http.ResponseWriter, r *http.Request) {
	hours, chosen, err := h.svc.MyWorkingHours(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, workingHoursResponse(hours, chosen))
}

// SaveMyWorkingHours implements PUT /me/working-hours.
//
// Human-only, and self-scoped inside the service: the caller's own row is the
// only one it writes, taken from the authenticated principal rather than from
// anything on the request. There is no id to pass and no admin form of it.
func (h Handlers) SaveMyWorkingHours(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.WorkingHours
	if !httperr.Decode(w, r, &body) {
		return
	}
	start, err := minutePastMidnight(body.StartTime)
	if err != nil {
		httperr.Write(w, r, &WorkingHoursError{Field: fieldWorkStart, Message: err.Error()})
		return
	}
	end, err := minutePastMidnight(body.EndTime)
	if err != nil {
		httperr.Write(w, r, &WorkingHoursError{Field: fieldWorkEnd, Message: err.Error()})
		return
	}
	saved, err := h.svc.SaveMyWorkingHours(r.Context(), WorkingHours{
		StartMinute: start, EndMinute: end, Days: body.Days, Timezone: body.Timezone,
	})
	if err != nil {
		// No branch for WorkingHoursError: it implements apperrors.FieldFault,
		// so the 422 naming the control comes from the error itself on every
		// surface rather than from a transport that has to remember.
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, workingHoursResponse(saved, true))
}

func workingHoursResponse(hours WorkingHours, chosen bool) crmcontracts.MyWorkingHoursResponse {
	return crmcontracts.MyWorkingHoursResponse{
		Chosen: chosen,
		WorkingHours: crmcontracts.WorkingHours{
			StartTime: minuteOfDay(hours.StartMinute),
			EndTime:   minuteOfDay(hours.EndMinute),
			Days:      hours.Days,
			Timezone:  hours.Timezone,
		},
	}
}

// minutePastMidnight reads the wire's `HH:MM` as the minute the store keeps.
//
// `24:00` is accepted for the end of the day and is why this is not
// time.Parse: a working day that runs until midnight is one somebody keeps, and
// 23:59 is a different answer.
func minutePastMidnight(written string) (int, error) {
	hour, minute, found := strings.Cut(written, ":")
	if !found {
		return 0, fmt.Errorf("a time is written HH:MM")
	}
	hours, hoursErr := strconv.Atoi(hour)
	minutes, minutesErr := strconv.Atoi(minute)
	if hoursErr != nil || minutesErr != nil || len(hour) != 2 || len(minute) != 2 {
		return 0, fmt.Errorf("a time is written HH:MM")
	}
	total := hours*60 + minutes
	if hours < 0 || minutes < 0 || minutes > 59 || total > minutesInADay {
		return 0, fmt.Errorf("a time is between 00:00 and 24:00")
	}
	return total, nil
}
