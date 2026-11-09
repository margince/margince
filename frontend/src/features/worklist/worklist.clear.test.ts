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
