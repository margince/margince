// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The three answers a browser gives a page that asks to copy, as a test can
 * install them.
 *
 * `absent` is the one worth having a name for: outside a secure context
 * `navigator.clipboard` is not a rejecting API, it is not there at all, and a
 * suite that models the failure as a rejected promise proves nothing about the
 * deployment the failure actually happens on. The tree previously spelled this
 * two ways — an `Object.defineProperty` dance in one file, a whole
 * `vi.stubGlobal("navigator", …)` in another, which replaces the navigator
 * every other API in the render is reading.
 *
 * `written` is what landed, so a test can assert the TEXT rather than only that
 * a button changed its label.
 *
 * **Call this AFTER `userEvent.setup()`**, which installs a working clipboard of
 * its own. Stub first and setup silently replaces it, so the absent case copies
 * happily and the test passes while proving the opposite of its name.
 */
export type ClipboardStub = Readonly<{
  written: readonly string[];
  restore: () => void;
}>;

export function stubClipboard(
  behaviour: "accepts" | "refuses" | "absent",
): ClipboardStub {
  const original = Object.getOwnPropertyDescriptor(navigator, "clipboard");
  const written: string[] = [];
  const value =
    behaviour === "absent"
      ? undefined
      : {
          writeText: async (text: string) => {
            if (behaviour === "refuses") {
              throw new Error("the clipboard refused the write");
            }
            written.push(text);
          },
        };
  Object.defineProperty(navigator, "clipboard", { value, configurable: true });
  return {
    written,
    restore: () => {
      // Deleting rather than defining `undefined` where there was nothing: a
      // property whose value is undefined and an absent property read the same
      // to this hook but not to every other reader of the navigator.
      if (original === undefined) {
        Reflect.deleteProperty(navigator, "clipboard");
        return;
      }
      Object.defineProperty(navigator, "clipboard", original);
    },
  };
}
