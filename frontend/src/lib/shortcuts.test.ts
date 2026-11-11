// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PRINTABLE_KEYS } from "../testing/keys";
import { KEYMAP, SEQUENCE_WINDOW_MS, type ShortcutId } from "./keymap";
import {
  createShortcutDispatcher,
  setSingleKeyShortcutsOn,
  singleKeyShortcutsOn,
  useShortcut,
  useShortcutAria,
  useShortcutDispatcher,
  useShortcutHandler,
} from "./shortcuts";
import { readStored, STORAGE_KEYS } from "./storage";

let ran: ShortcutId[] = [];
let singleKeys = true;
let dispatcher: ReturnType<typeof createShortcutDispatcher>;

/** Every binding has a handler that records its id. */
const everyHandler = (id: ShortcutId) => () => {
  ran.push(id);
};

beforeEach(() => {
  ran = [];
  singleKeys = true;
  dispatcher = createShortcutDispatcher({
    singleKeysOn: () => singleKeys,
    handlerFor: everyHandler,
  });
  globalThis.addEventListener("keydown", dispatcher.onKeyDown);
});

afterEach(() => {
  globalThis.removeEventListener("keydown", dispatcher.onKeyDown);
  cleanup();
  vi.useRealTimers();
  document.body.replaceChildren();
  act(() => setSingleKeyShortcutsOn(true));
});

function press(init: KeyboardEventInit, target: EventTarget = document.body) {
  const event = new KeyboardEvent("keydown", {
    bubbles: true,
    cancelable: true,
    ...init,
  });
  target.dispatchEvent(event);
  return event;
}

const letter = (key: string, init: KeyboardEventInit = {}) =>
  press({ key, code: `Key${key.toUpperCase()}`, ...init });

describe("a single key", () => {
  it("runs its binding and keeps the key from typing anywhere", () => {
    const event = letter("c");
    expect(ran).toEqual(["create"]);
    expect(event.defaultPrevented).toBe(true);
  });

  it("matches ? and / as typed, Shift and all", () => {
    press({ key: "?", code: "Slash", shiftKey: true });
    press({ key: "/", code: "Digit7", shiftKey: true });
    expect(ran).toEqual(["shortcuts", "search.slash"]);
  });

  it("refuses a letter typed with Shift", () => {
    letter("C", { shiftKey: true });
    expect(ran).toEqual([]);
  });

  it("refuses a key typed into a field", () => {
    const input = document.createElement("input");
    document.body.append(input);
    press({ key: "c", code: "KeyC" }, input);
    expect(ran).toEqual([]);
  });

  it("does not run M on AZERTY's comma, which sits at M's place", () => {
    press({ key: ",", code: "KeyM" });
    expect(ran).toEqual([]);
  });

  it("still runs once a row's checkbox holds focus", () => {
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    document.body.append(checkbox);
    press({ key: "c", code: "KeyC" }, checkbox);
    expect(ran).toEqual(["create"]);
  });

  it("reaches a binding from a Cyrillic layout by the key's place", () => {
    press({ key: "с", code: "KeyC" });
    expect(ran).toEqual(["create"]);
  });

  it("does nothing, and lets the key through, with no handler mounted", () => {
    const bare = createShortcutDispatcher({
      singleKeysOn: () => true,
      handlerFor: () => undefined,
    });
    const event = new KeyboardEvent("keydown", { key: "c", cancelable: true });
    bare.onKeyDown(event);
    expect(event.defaultPrevented).toBe(false);
  });
});

describe("a go-to sequence", () => {
  it("runs on G, then the letter", () => {
    const lead = letter("g");
    letter("c");
    expect(ran).toEqual(["goto.contacts"]);
    expect(lead.defaultPrevented).toBe(true);
  });

  it("reaches the letter from a Greek layout", () => {
    press({ key: "γ", code: "KeyG" });
    press({ key: "ψ", code: "KeyC" });
    expect(ran).toEqual(["goto.contacts"]);
  });

  it("lets the second key go once the window has passed", () => {
    vi.useFakeTimers();
    letter("g");
    vi.advanceTimersByTime(SEQUENCE_WINDOW_MS);
    letter("c");
    expect(ran).toEqual(["create"]);
  });

  it("still runs just inside the window", () => {
    vi.useFakeTimers();
    letter("g");
    vi.advanceTimersByTime(SEQUENCE_WINDOW_MS - 1);
    letter("d");
    expect(ran).toEqual(["goto.deals"]);
  });

  it("ends on a letter that names nothing after G", () => {
    letter("g");
    letter("x");
    letter("c");
    expect(ran).toEqual(["create"]);
  });
});

describe("a chord", () => {
  it("runs with either platform modifier", () => {
    letter("k", { metaKey: true });
    letter("k", { ctrlKey: true });
    expect(ran).toEqual(["search", "search"]);
  });

  // Ctrl+Alt is AltGr on Windows, and Shift turns ⌘B into bold in an editor.
  it("refuses another modifier held with it", () => {
    letter("k", { ctrlKey: true, altKey: true });
    letter("b", { metaKey: true, shiftKey: true });
    letter("k", { metaKey: true, ctrlKey: true });
    expect(ran).toEqual([]);
  });

  it("works in a field, unless the field keeps it for itself", () => {
    const input = document.createElement("input");
    document.body.append(input);
    press({ key: "k", code: "KeyK", metaKey: true }, input);
    press({ key: "b", code: "KeyB", metaKey: true }, input);
    expect(ran).toEqual(["search"]);
  });

  it("stays on while single keys are off", () => {
    singleKeys = false;
    letter("k", { metaKey: true });
    letter("c");
    letter("g");
    letter("c");
    press({ key: "?", shiftKey: true });
    expect(ran).toEqual(["search"]);
  });
});

// Each dispatched binding runs on its own keys and on nothing else, so a key
// the keymap does not list cannot reach a handler.
describe("every binding the dispatcher answers", () => {
  const dispatched = KEYMAP.filter(
    (entry) => entry.answeredBy === "dispatcher",
  );

  it("runs on its keys", () => {
    for (const entry of dispatched) {
      ran = [];
      const keys = entry.keys;
      if (keys.kind === "chord") {
        press({
          key: keys.key,
          code: `Key${keys.key.toUpperCase()}`,
          ctrlKey: true,
        });
      } else if (keys.kind === "sequence") {
        letter(keys.keys[0]);
        letter(keys.keys[1]);
      } else if (keys.kind === "single") {
        press({ key: keys.key, code: "" });
      }
      expect(ran).toEqual([entry.id]);
    }
  });

  it("runs on no printable key its keymap entry does not name", () => {
    const singles = new Set(
      dispatched.flatMap((entry) =>
        entry.keys.kind === "single" ? [entry.keys.key] : [],
      ),
    );
    const answered = PRINTABLE_KEYS.filter((key) => {
      ran = [];
      press({ key, code: "" });
      press({ key: "Escape", code: "Escape" });
      return ran.length > 0;
    });
    expect(answered).toEqual(
      PRINTABLE_KEYS.filter((key) => singles.has(key.toLowerCase())),
    );
  });
});

describe("the handler a binding runs", () => {
  const mounted = createShortcutDispatcher({ singleKeysOn: () => true });
  const pressed = (key: string) =>
    mounted.onKeyDown(
      new KeyboardEvent("keydown", {
        key,
        code: `Key${key.toUpperCase()}`,
        cancelable: true,
      }),
    );

  it("is the one mounted last, and the earlier one again once it leaves", () => {
    const first = vi.fn();
    const second = vi.fn();
    renderHook(() => useShortcutHandler("record.addTask", first));
    const later = renderHook(() =>
      useShortcutHandler("record.addTask", second),
    );
    pressed("t");
    later.unmount();
    pressed("t");
    expect(second).toHaveBeenCalledOnce();
    expect(first).toHaveBeenCalledOnce();
  });

  it("is none while its component says it is not enabled", () => {
    const written = vi.fn();
    renderHook(() => useShortcutHandler("record.writeEmail", written, false));
    pressed("m");
    expect(written).not.toHaveBeenCalled();
  });

  it("calls the latest render's handler", () => {
    const calls: string[] = [];
    const view = renderHook(
      ({ word }) =>
        useShortcutHandler("record.logActivity", () => calls.push(word)),
      { initialProps: { word: "first" } },
    );
    view.rerender({ word: "second" });
    pressed("n");
    expect(calls).toEqual(["second"]);
  });
});

describe("a shortcut a control answers", () => {
  const mounted = createShortcutDispatcher({ singleKeysOn: () => true });
  const pressN = () =>
    mounted.onKeyDown(
      new KeyboardEvent("keydown", {
        key: "n",
        code: "KeyN",
        cancelable: true,
      }),
    );

  it("runs, and names its key, only while enabled", () => {
    const logged = vi.fn();
    const control = renderHook(
      ({ enabled }) => useShortcut("record.logActivity", logged, enabled),
      { initialProps: { enabled: false } },
    );
    pressN();
    expect(logged).not.toHaveBeenCalled();
    expect(control.result.current).toBeUndefined();

    control.rerender({ enabled: true });
    pressN();
    expect(logged).toHaveBeenCalledOnce();
    expect(control.result.current).toBe("n");
  });
});

describe("the single-key preference", () => {
  it("is on until the reader turns it off, and this browser keeps it", () => {
    expect(singleKeyShortcutsOn()).toBe(true);
    act(() => setSingleKeyShortcutsOn(false));
    expect(singleKeyShortcutsOn()).toBe(false);
    expect(readStored(STORAGE_KEYS.singleKeyShortcuts)).toBe("off");
  });

  it("drops a single key's aria-keyshortcuts while off, and keeps a chord's", () => {
    const create = renderHook(() => useShortcutAria("create"));
    const search = renderHook(() => useShortcutAria("search"));
    expect(create.result.current).toBe("c");
    act(() => setSingleKeyShortcutsOn(false));
    create.rerender();
    search.rerender();
    expect(create.result.current).toBeUndefined();
    expect(search.result.current).toBe("Meta+K Control+K");
  });
});

describe("the app's dispatcher", () => {
  // A G pressed before the shell unmounts must not finish a sequence after it mounts again.
  it("forgets a pending G once nothing listens", () => {
    globalThis.removeEventListener("keydown", dispatcher.onKeyDown);
    const handled: string[] = [];
    renderHook(() => {
      useShortcutHandler("goto.contacts", () => handled.push("goto"));
      useShortcutHandler("create", () => handled.push("create"));
    });
    const shell = renderHook(() => useShortcutDispatcher());
    letter("g");
    shell.unmount();
    renderHook(() => useShortcutDispatcher());
    letter("c");
    expect(handled).toEqual(["create"]);
  });
});
