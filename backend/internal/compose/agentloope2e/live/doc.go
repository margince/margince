// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package live is the paid half of the agent_loop lane. It turns what a real
// model did on a seeded workspace into verdict files, and tells a report which
// files still describe the product. It opens no database and calls no model: the
// integration harness supplies each run's end state, and the judge is reached
// through agentloope2e.Judge. That keeps everything here testable for free.
package live
