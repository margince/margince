// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// CheckKindFields is the one rule for which fields an activity's kind may carry.
// The wire mapper and the import ask it, so a request and a spreadsheet row are
// refused for the same field with the same error.
//
// The order is the order a caller fixes things in: the kind, what only a
// meeting carries, the transport, then the duration. The wire mapper asks the
// same four checks one at a time, where its own earlier errors win.
func CheckKindFields(kind string, durationSeconds *int, meetingStatus *string, channelProvider string) error {
	if err := checkKind(kind); err != nil {
		return err
	}
	if err := checkMeetingStatusKind(kind, meetingStatus); err != nil {
		return err
	}
	if err := refuseKindProviderMismatch(kind, channelProvider); err != nil {
		return err
	}
	if err := checkDurationKind(kind, durationSeconds); err != nil {
		return err
	}
	return validateActivityDuration(durationSeconds)
}

func checkKind(kind string) error {
	if kind == "" {
		return &RequiredFieldError{Field: fieldKind}
	}
	// A kind outside the contract's list is a picklist mistake, refused here
	// rather than by activity_kind_fkey, which a transport reads as a missing record.
	if !crmcontracts.CreateActivityRequestKind(kind).Valid() {
		return &InvalidKindError{Kind: kind}
	}
	return nil
}

func checkMeetingStatusKind(kind string, meetingStatus *string) error {
	if meetingStatus != nil && kind != KindMeeting {
		return &MeetingStatusKindError{Kind: kind}
	}
	return nil
}

// checkDurationKind is the kind half of the duration rule. The contract
// promises field_not_valid_for_kind for this, and only the two kinds that
// occupy a span of time have a duration to state.
func checkDurationKind(kind string, durationSeconds *int) error {
	if durationSeconds != nil && kind != KindMeeting && kind != string(crmcontracts.ActivityKindCall) {
		return &KindFieldError{Field: fieldDurationSeconds, Only: "a meeting or call"}
	}
	return nil
}

// meetingStatusOf is the wire's meeting status as the plain string the rule reads.
func meetingStatusOf(status *crmcontracts.CreateActivityRequestMeetingStatus) *string {
	if status == nil {
		return nil
	}
	s := string(*status)
	return &s
}

// providerOf is the wire's transport, empty when the request names none.
func providerOf(provider *string) string {
	if provider == nil {
		return ""
	}
	return *provider
}

// MaxLinks is how many records one activity may be filed under.
const MaxLinks = maxActivityLinks

// CheckLinkTypes is the rule for which records an activity of a kind may be
// filed under, held against the record types alone. An import asks it before
// any record is looked up, so a row is refused for its cell and not at commit.
func CheckLinkTypes(kind string, entityTypes []string) error {
	if len(entityTypes) > maxActivityLinks {
		return &TooManyLinksError{Count: len(entityTypes)}
	}
	for _, entityType := range entityTypes {
		if linkColumn(entityType) == "" {
			return &InvalidLinkTypeError{EntityType: entityType}
		}
		if err := refuseACompanyMeeting(kind, ActivityLinkInput{EntityType: entityType}); err != nil {
			return err
		}
	}
	return nil
}
