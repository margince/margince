// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// bookingLength is the profile at the length a guest asked for, or at its
// default when the page does not offer that length. Opening the page uses it,
// so an old or mistyped length link still opens.
func bookingLength(profile crmcontracts.SchedulingProfile, requested *int) crmcontracts.SchedulingProfile {
	profile.DurationMinutes = offeredLength(bookingLengths(profile), requested)
	return profile
}

// chosenLength is the profile at the length a guest chose on the page. Times
// and bookings refuse a length the page no longer offers. A silent default
// would book a guest at a length the page did not show.
func chosenLength(profile crmcontracts.SchedulingProfile, requested *int) (crmcontracts.SchedulingProfile, error) {
	if requested != nil && !slices.Contains(bookingLengths(profile), *requested) {
		return profile, httperr.Validation("l", "length_not_offered", "This meeting length is no longer offered. Choose another length.")
	}
	return bookingLength(profile, requested), nil
}

// bookingLengths are the meeting lengths a page offers, its default first.
func bookingLengths(profile crmcontracts.SchedulingProfile) []int {
	lengths := []int{profile.DurationMinutes}
	if profile.OfferedLengths != nil {
		for _, length := range *profile.OfferedLengths {
			if !slices.Contains(lengths, length) {
				lengths = append(lengths, length)
			}
		}
	}
	return lengths
}

// offeredLength is the length a booking runs at: the requested one when
// lengths offers it, else the default that lengths names first.
func offeredLength(lengths []int, requested *int) int {
	if requested != nil && slices.Contains(lengths, *requested) {
		return *requested
	}
	return lengths[0]
}
