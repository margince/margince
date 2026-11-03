// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { SummaryWords } from "./approvalsummary";

/** Summary words for a test: each field by its own name. */
export const PLAIN_SUMMARY_WORDS: SummaryWords = {
  field: (name) => name,
  more: (count) => `${count} more not shown`,
};
