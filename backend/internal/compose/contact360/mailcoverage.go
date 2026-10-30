// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import "github.com/margince/margince/backend/internal/compose/mailcoverage"

// WithMailCoverage keeps connection observations outside the record transaction.
func (s *Service) WithMailCoverage(read mailcoverage.Reader) *Service {
	s.mailCoverage = read
	return s
}
