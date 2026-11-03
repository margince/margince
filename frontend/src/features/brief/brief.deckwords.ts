// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { DeckVerdict } from "../../design-system/decisiondeck";
import type { MessageKey } from "../../i18n/en";

/**
 * The words the Brief's deck answers its keys with. The deck's labels and the
 * shortcuts dialog both read them, so the dialog names a key the way the card does.
 */
export const DECK_KEY_WORDS = {
  accept: "brief.approval.approve",
  edit: "trust.edit",
  reject: "decision.reject",
  skip: "brief.deck.later",
  unstage: "brief.deck.unstage",
  commit: "brief.deck.commit",
} as const satisfies Record<DeckVerdict | "unstage" | "commit", MessageKey>;
