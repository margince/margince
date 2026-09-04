// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rankAll orders a day for a test that is not asking about ownership.
//
// The production entry point takes the reader, because the lanes whose rows are
// theirs by construction can only be resolved against somebody. A test about
// ORDERING has no such reader and does not need one: the comparator never looks
// at the owner. The tests that ARE about ownership call rankAllFor with a real
// id, which is what makes the distinction visible rather than hidden behind a
// default.
func rankAll(rows []ranked) []crmcontracts.WorklistItem {
	return rankAllFor(rows, ids.UUID{})
}
