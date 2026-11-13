// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vitest";
import { ACTION_WORDS } from "../src/features/automations/automations.actionwords";
import { de } from "../src/i18n/de";
import { en } from "../src/i18n/en";
import { vi } from "../src/i18n/vi";
import { srcDir } from "./tree";

it("gives every registered automation action words in every language", () => {
  const source = readFileSync(
    join(
      srcDir,
      "../../backend/internal/modules/automation/catalog_actions.go",
    ),
    "utf8",
  );
  const values = new Map(
    Array.from(
      source.matchAll(/(ActionType\w+)\s+ActionType\s*=\s*"([^"]+)"/g),
      (match): [string, string] => [match[1], match[2]],
    ),
  );
  const registry = Array.from(
    source.matchAll(/^\s*(ActionType\w+):\s*\{/gm),
    (match) => match[1],
  );
  expect(registry.length).toBeGreaterThan(0);
  expect(new Set(registry)).toEqual(new Set(values.keys()));
  for (const name of registry) {
    const action = values.get(name);
    expect(action, `${name} has no readable action value`).toBeTruthy();
    const word = action ? ACTION_WORDS[action] : undefined;
    expect(word, `${name} has no result wording`).toBeTruthy();
    for (const language of [en, de, vi]) {
      expect(
        word && language[word],
        `${name} has no translated result wording`,
      ).toBeTruthy();
    }
  }
});
