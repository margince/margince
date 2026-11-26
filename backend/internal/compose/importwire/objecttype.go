// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// ObjectOf types a CSV sent without a type by the mapping suggested for it as
// each type. With no winner it is read as a lead, and says nothing chose it.
func ObjectOf(suggestions map[string]map[string]string) (string, crmcontracts.ImportSourceProfileObjectChosenBy) {
	if object := migration.ObjectOfMappings(suggestions); object != "" {
		return object, crmcontracts.ImportSourceProfileObjectChosenByColumns
	}
	return migration.ObjectLead, crmcontracts.ImportSourceProfileObjectChosenByNone
}
