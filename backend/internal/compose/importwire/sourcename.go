// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	"fmt"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// SourceNamePattern is the contract's pattern for `source_name`.
var SourceNamePattern = regexp.MustCompile(`^[A-Za-z0-9 ._-]*$`)

// SourceNameMax is the contract's `maxLength` for `source_name`.
const SourceNameMax = 80

// SourceName trims a requested source name and holds it to the contract.
func SourceName(raw *string) (string, error) {
	if raw == nil {
		return "", nil
	}
	name := strings.TrimSpace(*raw)
	if len(name) > SourceNameMax || !SourceNamePattern.MatchString(name) {
		return "", httperr.Validation("source_name", "invalid", fmt.Sprintf(
			"A source name holds at most %d characters: the letters A to Z without accents, the digits 0 to 9, "+
				"spaces, dots (.), underscores (_) and hyphens (-).", SourceNameMax)).WithMax(SourceNameMax)
	}
	return name, nil
}

// SuggestedSourceName is the source name the upload proposes for a ZIP. It is
// built to pass SourceName, so a name the screen pre-fills is never refused.
func SuggestedSourceName(fileName string) string {
	stem := fileName
	if strings.HasSuffix(strings.ToLower(stem), ".zip") {
		stem = stem[:len(stem)-len(".zip")]
	}
	name := strings.Map(func(r rune) rune {
		if SourceNamePattern.MatchString(string(r)) {
			return r
		}
		return '-'
	}, stem)
	if len(name) > SourceNameMax {
		name = name[:SourceNameMax]
	}
	return name
}

// SourceSystem namespaces the source_system a run's rows carry, so ref 1 from
// two old CRMs is two records. An empty name keeps the namespace runs used
// before source names. The wire mappers refuse the reserved prefix, so a
// client cannot plant a row under a guessed import id.
//
// An own import (ownSeat set) gets its seat's namespace, so its refs never
// meet a workspace import's bindings or another seat's.
func SourceSystem(sourceName string, ownSeat *ids.UserID) string {
	base := provenance.ReservedSourceSystemPrefix + migration.ConnectorCSV
	if ownSeat != nil {
		base += ":" + provenance.OwnImportSeatSegment + ownSeat.String()
	}
	if sourceName == "" {
		return base
	}
	return base + ":" + sourceName
}
