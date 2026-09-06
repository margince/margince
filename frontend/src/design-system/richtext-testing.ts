// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { fireEvent, screen } from "@testing-library/react";

/**
 * The ONE way a test writes into a Margince `RichText`.
 *
 * `userEvent.type` drives an `<input>` or a `<textarea>`; this control is a
 * `contentEditable` div, and jsdom's implementation of one does not produce the
 * text a typed key would. So a suite that wants words in the editor has to write
 * the markup and announce it, which is two steps and one piece of this
 * component's internals. Every suite doing that by hand encodes those internals
 * in as many files as there are suites — the composer's did, in two — and they
 * break together the day the editor's shape changes. Import this instead:
 *
 * ```ts
 * writeMessage("Body", "On my way.");
 * ```
 *
 * `label` is the editor's accessible name. The toolbar above it carries the same
 * name, so this asks for the TEXTBOX by role and never by label text, which
 * matches both.
 *
 * The text is written as ONE paragraph. A test that needs several passes them
 * separated by a blank line, exactly as the editor's own plain-text reading
 * takes them.
 */
export function writeMessage(label: string, text: string): HTMLElement {
  const editor = messageBox(label);
  editor.innerHTML = text
    .split(/\n{2,}/)
    .map((block) => `<p>${block}</p>`)
    .join("");
  fireEvent.input(editor);
  return editor;
}

/** The editor itself, for a test asserting on what it holds. */
export function messageBox(label: string): HTMLElement {
  return screen.getByRole("textbox", { name: label });
}

/** What the editor is showing, as a reader would read it back. */
export function messageText(label: string): string {
  return messageBox(label).textContent ?? "";
}
