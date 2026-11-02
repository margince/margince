// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PRINTABLE_KEYS } from "../testing/keys";
import type { DecisionDeckItem } from "./decisiondeck";
import {
  DECK_COMMIT_KEY,
  DECK_UNSTAGE_KEY,
  DECK_VERDICT_KEYS,
  deckKeyHandler,
  keyVerdict,
} from "./decisiondeck.verdicts";

afterEach(cleanup);

const LIVE: DecisionDeckItem = {
  kind: "single",
  id: "id-1",
  approval: {
    id: "id-1",
    kind: "held_draft",
    status: "pending",
    proposed_by: "agent:mailroom",
    created_at: "2026-08-24T07:41:00.000Z",
    summary: "Reply.",
  },
};

const NAMED_KEYS = [
  "ArrowUp",
  "ArrowDown",
  "ArrowLeft",
  "ArrowRight",
  "Enter",
  "Escape",
  "Tab",
  "Backspace",
  "Delete",
  "Home",
  "End",
  "PageUp",
  "PageDown",
  "F1",
];

function claimedKeys(): string[] {
  const { container } = render(
    <fieldset
      tabIndex={-1}
      onKeyDown={deckKeyHandler({
        live: LIVE,
        onStage: vi.fn(),
        onUnstage: vi.fn(),
        onCommit: vi.fn(),
      })}
    />,
  );
  const surface = container.querySelector("fieldset");
  if (!surface) {
    throw new Error("the surface did not render");
  }
  return [...NAMED_KEYS, ...PRINTABLE_KEYS].filter(
    (key) => !fireEvent.keyDown(surface, { key }),
  );
}

describe("the deck's keys", () => {
  // The shortcuts dialog lists the deck from these exports. A key the handler
  // answers that they do not name is a shortcut nobody is told about.
  it("binds exactly the keys its exported table names", () => {
    const listed = [
      ...DECK_VERDICT_KEYS.map((bound) => bound.key),
      DECK_COMMIT_KEY,
      DECK_UNSTAGE_KEY,
      DECK_UNSTAGE_KEY.toUpperCase(),
    ];
    expect(claimedKeys().sort()).toEqual(listed.sort());
  });

  it("reads every verdict from the table", () => {
    for (const bound of DECK_VERDICT_KEYS) {
      expect(keyVerdict(bound.key)).toBe(bound.verdict);
    }
    expect(new Set(DECK_VERDICT_KEYS.map((bound) => bound.verdict)).size).toBe(
      4,
    );
  });
});
