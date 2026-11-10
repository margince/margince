// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DECK_COMMIT_KEY,
  DECK_UNSTAGE_KEY,
  DECK_VERDICT_KEYS,
} from "../design-system/decisiondeck.verdicts";
import { isParagraphCommit } from "../design-system/fieldeditdialog";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { bindingCaps, capOf, KEYMAP, shortcutCaps } from "../lib/keymap";
import { setSingleKeyShortcutsOn } from "../lib/shortcuts";
import { PRINTABLE_KEYS } from "../testing/keys";
import { ShortcutsDialog } from "./shortcutsdialog";

afterEach(() => {
  cleanup();
  act(() => setSingleKeyShortcutsOn(true));
});

function renderDialog() {
  render(
    <LocaleProvider initial="en">
      <ShortcutsDialog open onClose={vi.fn()} returnFocusTo={() => null} />
    </LocaleProvider>,
  );
  return screen.getByRole("dialog", { name: "Keyboard shortcuts" });
}

function groupNamed(dialog: HTMLElement, title: string): HTMLElement {
  const group = within(dialog)
    .getByRole("heading", { name: title })
    .closest("section");
  if (!(group instanceof HTMLElement)) {
    throw new Error(`the ${title} group did not render`);
  }
  return group;
}

/** The caps the dialog draws beside a label. */
function capsBeside(scope: HTMLElement, label: string): string[] {
  const term = within(scope).getByText(label, { selector: "dt" });
  const value = term.nextElementSibling;
  return [...(value?.querySelectorAll("kbd") ?? [])].map(
    (cap) => cap.textContent ?? "",
  );
}

const GROUP_TITLES = {
  anywhere: en["shell.shortcuts.anywhere"],
  goto: en["shell.shortcuts.goto"],
  record: en["shell.shortcuts.record"],
  editing: en["shell.shortcuts.editing"],
} as const;

describe("the keyboard shortcuts dialog", () => {
  // Read from the keymap the dispatcher reads, so a binding added there and not
  // drawn here fails, whichever group it joins.
  it("lists every binding of the keymap under its group, with its caps", () => {
    const dialog = renderDialog();
    for (const entry of KEYMAP) {
      const group = groupNamed(dialog, GROUP_TITLES[entry.group]);
      const caps = capsBeside(group, en[entry.labelKey]).join(" ");
      expect(caps).toContain(bindingCaps(entry.keys).join(" "));
    }
  });

  it("draws search as one row with both ways to open it", () => {
    const dialog = renderDialog();
    const row = within(dialog).getByText("Open search", { selector: "dt" });
    expect(row.nextElementSibling).toHaveTextContent(
      `${shortcutCaps("search").join("")}or/`,
    );
  });

  it("reads a go-to sequence as one key, then the next", () => {
    const dialog = renderDialog();
    const goTo = groupNamed(dialog, "Go to");
    expect(within(goTo).getByText("Press G, then the letter.")).toBeVisible();
    const contacts = within(goTo).getByText("Contacts", { selector: "dt" });
    expect(contacts.nextElementSibling).toHaveTextContent("GthenC");
  });

  it("says so when single-key shortcuts are off", () => {
    act(() => setSingleKeyShortcutsOn(false));
    const dialog = renderDialog();
    expect(
      within(dialog).getByText("Single-key shortcuts are off"),
    ).toBeVisible();
  });

  it("says nothing about them while they are on", () => {
    const dialog = renderDialog();
    expect(
      within(dialog).queryByText("Single-key shortcuts are off"),
    ).toBeNull();
  });

  it("lists every key the decision deck binds, in the Brief's words", () => {
    const deck = groupNamed(renderDialog(), en["brief.panel.decisions"]);
    const listed = [...deck.querySelectorAll("kbd")].map(
      (cap) => cap.textContent,
    );
    expect(listed).toEqual([
      ...DECK_VERDICT_KEYS.map((bound) => capOf(bound.key)),
      capOf(DECK_UNSTAGE_KEY),
      capOf(DECK_COMMIT_KEY),
    ]);
    expect(capsBeside(deck, "Approve")).toEqual(["→"]);
    expect(capsBeside(deck, "Later")).toEqual(["↓"]);
    expect(capsBeside(deck, "Undo last")).toEqual(["U"]);
    expect(capsBeside(deck, "Send staged decisions")).toEqual(["Enter"]);
  });
});

// The keymap lists the paragraph save, and the field binds it. A field that
// answered another key fails here.
describe("the paragraph save the field binds", () => {
  it("answers the key the keymap lists, and only with a modifier", () => {
    const paragraph = KEYMAP.find(
      (entry) => entry.id === "editing.paragraphSave",
    );
    if (paragraph?.keys.kind !== "chord") {
      throw new Error("the paragraph save is listed as a chord");
    }
    const keys = [...PRINTABLE_KEYS, paragraph.keys.key, "Tab", "Escape"];
    const chord = (key: string, metaKey: boolean, ctrlKey: boolean) =>
      isParagraphCommit({
        key,
        code: "",
        metaKey,
        ctrlKey,
        altKey: false,
        shiftKey: false,
      });
    expect(keys.filter((key) => chord(key, true, false))).toEqual([
      paragraph.keys.key,
    ]);
    expect(keys.filter((key) => chord(key, false, true))).toEqual([
      paragraph.keys.key,
    ]);
    expect(keys.filter((key) => chord(key, false, false))).toEqual([]);
  });
});
