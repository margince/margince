// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
)

func newContactsStore(db *database.DB) *contacts.Store {
	return contacts.NewStore(db).WithConsumerMail(capture.MatcherTx)
}
