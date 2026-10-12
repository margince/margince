// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// suggestedSourceName is the source name the upload proposes for a ZIP. It is
// built to pass sourceNameFrom, so a name the screen pre-fills is never refused.
func suggestedSourceName(fileName string) string {
	stem := fileName
	if strings.HasSuffix(strings.ToLower(stem), ".zip") {
		stem = stem[:len(stem)-len(".zip")]
	}
	name := strings.Map(func(r rune) rune {
		if sourceNamePattern.MatchString(string(r)) {
			return r
		}
		return '-'
	}, stem)
	if len(name) > maxSourceName {
		name = name[:maxSourceName]
	}
	return name
}

// importSourceSystem namespaces the source_system a run's rows carry, so ref 1
// from two old CRMs is two records. An empty name is the namespace every run
// used before source names, so a single CSV re-uploaded still finds its rows.
//
// The wire mappers refuse the reserved prefix, so a client cannot plant a row
// under a guessed import id. The identity map decides "already imported".
func importSourceSystem(sourceName string) string {
	base := provenance.ReservedSourceSystemPrefix + migration.ConnectorCSV
	if sourceName == "" {
		return base
	}
	return base + ":" + sourceName
}
