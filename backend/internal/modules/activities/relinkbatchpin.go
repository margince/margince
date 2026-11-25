// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// refuseBatchPin keeps a version pin off the doors it cannot mean.
//
// One version cannot speak for a thread or a named set. Applied per row, it
// would refuse every activity except the one that matched: a partial move
// nobody asked for. Only the single relink takes a pin. A caller that sends one
// here is told so, because a pin that is ignored would hide the conflict.
func refuseBatchPin(in RelinkActivityInput) error {
	if in.IfVersion == nil {
		return nil
	}
	return &BatchPinError{}
}

// BatchPinError refuses a version pin on a door that moves many activities.
//
// A MessageFault rather than a field fault: the pin arrives as an If-Match
// header, so there is no request field to name.
type BatchPinError struct{}

func (e *BatchPinError) Error() string {
	return "a thread or a named set moves many activities, and one version cannot condition them"
}

// MessageFault maps this to a 422 carrying the code, so a client can branch on
// the reason rather than parsing prose.
func (e *BatchPinError) MessageFault() (code, message string) {
	return "pin_not_supported", e.Error() +
		" — drop the If-Match and relink them one at a time to pin a version"
}
