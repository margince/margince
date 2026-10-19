// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import (
	"fmt"
	"strings"
	"time"
)

// MeetingDetails formats the same interval for provider descriptions and reminders.
func MeetingDetails(subject, description, location string, start, end time.Time, zone *time.Location, durationUnit string) string {
	if zone == nil {
		zone = time.UTC
	}
	lines := []string{subject, start.In(zone).Format("2006-01-02 15:04") + " – " + end.In(zone).Format("15:04") + " (" + zone.String() + ")", fmt.Sprintf("%d %s", int(end.Sub(start)/time.Minute), durationUnit)}
	if strings.TrimSpace(location) != "" {
		lines = append(lines, location)
	}
	if strings.TrimSpace(description) != "" {
		lines = append(lines, description)
	}
	return strings.Join(lines, "\n")
}

// Fragment reuses safe body components within governed human-mail composition.
func Fragment(message Message) string {
	var body strings.Builder
	for paragraph := range strings.SplitSeq(message.Text, "\n\n") {
		if strings.TrimSpace(paragraph) != "" {
			body.WriteString(renderParagraph(paragraph, message.Title, message.ActionPlaceholder))
		}
	}
	return body.String()
}

// MeetingDescription leaves event time and location to the provider's live fields.
func MeetingDescription(description string) string {
	return strings.TrimSpace(description)
}
