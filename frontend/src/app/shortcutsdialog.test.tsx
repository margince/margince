// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DECK_COMMIT_KEY,
  DECK_UNSTAGE_KEY,
  DECK_VERDICT_KEYS,
} from "../design-system/decisiondeck.verdicts";
import {
  FIELD_COMMIT_KEY,
  isParagraphCommit,
} from "../design-system/fieldeditdialog";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { PRINTABLE_KEYS } from "../testing/keys";
import { isSidebarChord, modifierCap, sidebarHotkeyCaps } from "./hotkeys";
import { paletteHotkeyCaps, usePaletteHotkey } from "./palette";
import { capOf, ShortcutsDialog } from "./shortcutsdialog";

afterEach(cleanup);

function renderDialog() {
  render(
    <LocaleProvider initial="en">
      <ShortcutsDialog open onClose={vi.fn()} returnFocusTo={() => null} />
    </LocaleProvider>,
  );
  return screen.getByRole("dialog", { name: "Keyboard shortcuts" });
}

/** The caps the dialog draws beside a label. */
function capsBeside(dialog: HTMLElement, label: string): string[] {
  const term = within(dialog).getByText(label, { selector: "dt" });
  const value = term.nextElementSibling;
  return [...(value?.querySelectorAll("kbd") ?? [])].map(
    (cap) => cap.textContent ?? "",
  );
}

/** The one letter a platform chord answers to, as its cap. */
function chordCaps(claims: (key: string) => boolean): string[] {
  return [
    ...new Set(PRINTABLE_KEYS.filter(claims).map((key) => key.toUpperCase())),
  ];
}

describe("the keyboard shortcuts dialog", () => {
  it("lists every chord from the definitions its handlers read", () => {
    const dialog = renderDialog();
    const platform = navigator.platform;
    expect(capsBeside(dialog, "Open search")).toEqual(
      paletteHotkeyCaps(platform),
    );
    expect(capsBeside(dialog, "Show or hide sidebar")).toEqual(
      sidebarHotkeyCaps(platform),
    );
    expect(capsBeside(dialog, "Close dialog or menu")).toEqual(["Esc"]);
  });

  it("lists every key the decision deck binds, in the Brief's words", () => {
    const dialog = renderDialog();
    const deck = within(dialog)
      .getByRole("heading", { name: en["brief.panel.decisions"] })
      .closest("section");
    if (!(deck instanceof HTMLElement)) {
      throw new Error("the deck group did not render");
    }
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

// A handler that answers a key the caps do not name, or caps naming a key no
// handler answers, fails here, whichever side moved.
describe("the chords the handlers bind", () => {
  const platform = navigator.platform;

  it("binds the sidebar on the letter its caps name", () => {
    const claims = (key: string) =>
      isSidebarChord({
        key,
        metaKey: true,
        ctrlKey: false,
        altKey: false,
        shiftKey: false,
      });
    expect(chordCaps(claims)).toEqual([sidebarHotkeyCaps(platform).at(-1)]);
  });

  it("binds search on the letter its caps name", () => {
    const setOpen = vi.fn();
    renderHook(() => usePaletteHotkey(false, setOpen));
    const claims = (key: string) => {
      setOpen.mockClear();
      fireEvent.keyDown(window, { key, ctrlKey: true });
      return setOpen.mock.calls.length > 0;
    };
    expect(chordCaps(claims)).toEqual([paletteHotkeyCaps(platform).at(-1)]);
  });

  it("saves a paragraph on the key its caps name, and only with a modifier", () => {
    const keys = [...PRINTABLE_KEYS, FIELD_COMMIT_KEY, "Tab", "Escape"];
    const chord = (key: string, metaKey: boolean, ctrlKey: boolean) =>
      isParagraphCommit({ key, metaKey, ctrlKey });
    expect(keys.filter((key) => chord(key, true, false))).toEqual([
      FIELD_COMMIT_KEY,
    ]);
    expect(keys.filter((key) => chord(key, false, true))).toEqual([
      FIELD_COMMIT_KEY,
    ]);
    expect(keys.filter((key) => chord(key, false, false))).toEqual([]);
    expect(capsBeside(renderDialog(), "Save multi-line field")).toEqual([
      modifierCap(platform),
      capOf(FIELD_COMMIT_KEY),
    ]);
  });
});
