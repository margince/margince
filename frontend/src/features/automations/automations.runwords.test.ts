// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { translate } from "../../i18n";
import { de } from "../../i18n/de";
import { en, type MessageKey } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import { AUTOMATION_CATALOG } from "./automations.fixtures";
import { knownTriggerLabel } from "./automations.recipe";
import { RUN_ACTION_WORDS, UNNAMED_TARGET } from "./automations.runwords";

// The closed action set, read from the Go that declares it.
function serverActionKinds(): string[] {
  const source = readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      "../../../../backend/internal/shared/ports/workflow/workflow.go",
    ),
    "utf8",
  );
  const constants = new Map(
    [...source.matchAll(/\b(Action\w+)\s+ActionKind\s*=\s*"([a-z_]+)"/g)].map(
      ([, name, value]) => [name, value] as const,
    ),
  );
  const start = source.indexOf("func AllActionKinds()");
  const body = source.slice(start, source.indexOf("\n}\n", start));
  return [...body.matchAll(/\b(Action\w+)\b/g)].flatMap(
    ([, name]) => constants.get(name) ?? [],
  );
}

function wordedWithTarget(key: MessageKey) {
  for (const catalog of [en, de, vi]) {
    expect(catalog[key], key).toContain("{target}");
  }
}

describe("a run's words", () => {
  const kinds = serverActionKinds();

  it("reads the server's action kinds", () => {
    expect(kinds).toEqual(
      expect.arrayContaining(["create_task", "draft_email", "add_list_member"]),
    );
  });

  it.each(kinds)(
    "says what a run did and queued for %s in every language",
    (kind) => {
      const words = RUN_ACTION_WORDS[kind];
      expect(words, kind).toBeDefined();
      if (!words) return;
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
