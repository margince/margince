// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import "github.com/margince/margince/backend/internal/compose/integration"

func openCommitmentMeaning(due, words string) *commitmentMeaning {
	return (*commitmentMeaning)(integration.OpenCommitmentMeaning(due, words))
}
