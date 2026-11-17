// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import (
	"errors"
	"fmt"
	"slices"
)

// PurposeClass names a class of consent purpose, in the core catalog's
// vocabulary. Closed: the core gates a send by class.
type PurposeClass string

// The purpose classes.
const (
	PurposeClassMarketing              PurposeClass = "marketing"
	PurposeClassPhoneOutreach          PurposeClass = "phone_outreach"
	PurposeClassBusinessCorrespondence PurposeClass = "business_correspondence"
	PurposeClassTransactional          PurposeClass = "transactional"
)

// PurposeClasses lists every class, so the core can hold its own catalog to it.
func PurposeClasses() []PurposeClass {
	return []PurposeClass{
		PurposeClassMarketing, PurposeClassPhoneOutreach,
		PurposeClassBusinessCorrespondence, PurposeClassTransactional,
	}
}

// validateRequestFirstFor refuses a class the core does not know, and classes
// named for a rule the pack does not declare.
func (r Rules) validateRequestFirstFor() error {
	if len(r.ConsentRequestFirstFor) > 0 && !r.ConsentConfirmationNeedsRequest {
		return errors.New("purpose classes needing a request are named, but no consent mail needs one")
	}
	for _, class := range r.ConsentRequestFirstFor {
		if !slices.Contains(PurposeClasses(), class) {
			return fmt.Errorf("purpose class %q is not one the core knows", string(class))
		}
	}
	return nil
}
