// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../../i18n";
import { de } from "../../i18n/de";
import { en, type MessageKey } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import { AUTOMATION_CATALOG } from "./automations.fixtures";
import { knownTriggerLabel } from "./automations.recipe";
import { RUN_ACTION_WORDS, UNNAMED_TARGET } from "./automations.runwords";

function wordedWithTarget(key: MessageKey) {
  for (const catalog of [en, de, vi]) {
    expect(catalog[key], key).toContain("{target}");
  }
}

describe("a run's words", () => {
  it.each(Object.entries(RUN_ACTION_WORDS))(
    "says what a run did and queued for %s in every language",
    (_, words) => {
      wordedWithTarget(words.did);
      wordedWithTarget(words.queued);
    },
  );

  it.each([...new Set(AUTOMATION_CATALOG.map((entry) => entry.trigger))])(
    "words the trigger %s",
    (trigger) => {
      for (const locale of ["en", "de", "vi"] as const) {
        expect(
          knownTriggerLabel(trigger, (key, params) =>
            translate(locale, key, params),
          ),
        ).toBeDefined();
      }
    },
  );

  it("names an unseen target by its kind in every language", () => {
    for (const key of Object.values(UNNAMED_TARGET)) {
      for (const catalog of [en, de, vi]) expect(catalog[key]).not.toBe("");
    }
  });
});
