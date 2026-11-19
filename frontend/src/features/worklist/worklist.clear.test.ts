// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { en } from "../../i18n/en";
import { clearSentence } from "./worklist.clear";

const t = ((key: keyof typeof en) => en[key]) as Parameters<
  typeof clearSentence
>[3];

it("names the kind of work that is clear, not all of it", () => {
  expect(clearSentence(false, "urgent", null, t)).toBe(
    en["worklist.clearOfUrgent"],
  );
  expect(clearSentence(false, "all", null, t)).toBe(en["worklist.clear"]);
});

it("says decisions are waiting instead of that nothing is", () => {
  expect(clearSentence(false, "all", null, t, false, "today", true)).toBe(
    en["worklist.clearDecisionsWaiting"],
  );
  expect(clearSentence(false, "all", null, t, false, "today", false)).toBe(
    en["worklist.clear"],
  );
  expect(clearSentence(false, "all", null, t, false, "decisions", true)).toBe(
    en["worklist.clear.decisions"],
  );
});
