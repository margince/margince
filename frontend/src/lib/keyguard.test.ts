// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { afterEach, describe, expect, it } from "vitest";
import { characterKeyRefused, isTextEntry, layerIsOpen } from "./keyguard";

afterEach(() => {
  document.body.replaceChildren();
});

function place<T extends HTMLElement>(
  element: T,
  parent: HTMLElement = document.body,
): T {
  parent.append(element);
  return element;
}

/** `c` pressed on `target`, with whatever the case holds besides. */
function press(
  target: EventTarget,
  init: KeyboardEventInit = {},
): KeyboardEvent {
  let seen: KeyboardEvent | null = null;
  const listen = (event: Event) => {
    if (event instanceof KeyboardEvent) {
      seen = event;
    }
  };
  globalThis.addEventListener("keydown", listen);
  target.dispatchEvent(
    new KeyboardEvent("keydown", {
      key: "c",
      code: "KeyC",
      bubbles: true,
      cancelable: true,
      ...init,
    }),
  );
  globalThis.removeEventListener("keydown", listen);
  if (seen === null) {
    throw new Error("the press never reached the window");
  }
  return seen;
}

describe("a text field", () => {
  it.each([
    ["an input", () => document.createElement("input")],
    ["a text area", () => document.createElement("textarea")],
  ])("takes the key in %s", (_, make) => {
    expect(isTextEntry(place(make()))).toBe(true);
  });

  it("takes the key in an editable region", () => {
    const editor = place(document.createElement("div"));
    editor.contentEditable = "true";
    expect(isTextEntry(editor)).toBe(true);
  });

  it("leaves it to the page on a button or a link", () => {
    expect(isTextEntry(place(document.createElement("button")))).toBe(false);
    expect(isTextEntry(place(document.createElement("a")))).toBe(false);
    expect(isTextEntry(null)).toBe(false);
  });
});

describe("a character key on the page", () => {
  it("acts when nothing holds it", () => {
    expect(characterKeyRefused(press(document.body))).toBe(false);
  });

  it.each<[string, KeyboardEventInit]>([
    ["with ⌘ held", { metaKey: true }],
    ["with Ctrl held", { ctrlKey: true }],
    ["with Alt held", { altKey: true }],
    ["with Ctrl and Alt held, which is AltGr", { ctrlKey: true, altKey: true }],
    ["while the key is held down", { repeat: true }],
    ["while an input method composes", { isComposing: true }],
  ])("refuses %s", (_, init) => {
    expect(characterKeyRefused(press(document.body, init))).toBe(true);
  });

  it("refuses a key an input method reports as 229", () => {
    const event = new KeyboardEvent("keydown", { key: "c", keyCode: 229 });
    Object.defineProperty(event, "keyCode", { value: 229 });
    expect(characterKeyRefused(event)).toBe(true);
  });

  it("refuses a key another handler already took", () => {
    const button = place(document.createElement("button"));
    button.addEventListener("keydown", (event) => event.preventDefault());
    expect(characterKeyRefused(press(button))).toBe(true);
  });

  it("refuses a key typed into a field", () => {
    expect(
      characterKeyRefused(press(place(document.createElement("input")))),
    ).toBe(true);
  });
});

describe("a layer over the page", () => {
  it("is a modal dialog that is up", () => {
    const dialog = place(document.createElement("div"));
    dialog.setAttribute("role", "dialog");
    dialog.setAttribute("aria-modal", "true");
    expect(layerIsOpen(document.body)).toBe(true);
    expect(characterKeyRefused(press(document.body))).toBe(true);
  });

  it("is not a dialog already leaving behind its inert surface", () => {
    const leaving = place(document.createElement("div"));
    leaving.inert = true;
    const dialog = place(document.createElement("div"), leaving);
    dialog.setAttribute("role", "dialog");
    dialog.setAttribute("aria-modal", "true");
    expect(layerIsOpen(document.body)).toBe(false);
  });

  it("is a menu the key is pressed in", () => {
    const menu = place(document.createElement("div"));
    menu.setAttribute("role", "menu");
    const item = place(document.createElement("button"), menu);
    expect(layerIsOpen(item)).toBe(true);
  });

  it("is a popup its trigger says is open", () => {
    const trigger = place(document.createElement("button"));
    trigger.setAttribute("aria-haspopup", "menu");
    trigger.setAttribute("aria-expanded", "true");
    expect(layerIsOpen(document.body)).toBe(true);
    trigger.setAttribute("aria-expanded", "false");
    expect(layerIsOpen(document.body)).toBe(false);
  });

  it("is a panel portalled out of its trigger", () => {
    const trigger = place(document.createElement("button"));
    trigger.setAttribute("aria-expanded", "true");
    trigger.setAttribute("aria-controls", "panel");
    place(document.createElement("section")).id = "panel";
    expect(layerIsOpen(document.body)).toBe(true);
  });

  // A disclosure's region stays in the page; an open one must not silence the app.
  it("is not a disclosure's region beside its toggle", () => {
    const details = place(document.createElement("div"));
    const toggle = place(document.createElement("button"), details);
    toggle.setAttribute("aria-expanded", "true");
    toggle.setAttribute("aria-controls", "region");
    place(document.createElement("div"), details).id = "region";
    expect(layerIsOpen(document.body)).toBe(false);
  });
});
