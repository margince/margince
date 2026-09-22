// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The three answers a browser gives a page that asks to copy, as a test can
 * install them.
 *
 * `absent` is the one worth having a name for: outside a secure context
 * `navigator.clipboard` is not a rejecting API, it is not there at all, and a
 * suite that models the failure as a rejected promise proves nothing about the
 * deployment the failure actually happens on. This is the ONE spelling of it:
 * the alternatives are an `Object.defineProperty` dance that restores by hand,
 * and `vi.stubGlobal("navigator", …)`, which replaces the whole navigator every
 * other API in the render is reading.
 *
 * `written` is what landed, so a test can assert the TEXT rather than only that
 * a button changed its label.
 *
 * **Call this AFTER `userEvent.setup()`**, which installs a working clipboard of
 * its own. Stub first and setup silently replaces it, so the absent case copies
 * happily and the test passes while proving the opposite of its name.
 *
 * A vitest case need not undo its own stub: `restoreClipboardStubs` is armed in
 * vitest.setup.ts and unwinds whatever the case left. `restore` is for the
 * caller that has no teardown — a Storybook `play`, which shares one document
 * with every other story on the page.
 */
export type ClipboardStub = Readonly<{
  written: readonly string[];
  restore: () => void;
}>;

// Stubs installed and not yet undone, in the order they were installed.
//
// Restoring on the happy path is not restoring: an assertion that fails leaves
// the stub standing, and every later case in that file then reads a clipboard
// nobody asked for — so one real failure arrives as a queue of unrelated ones
// and the first report names the wrong test.
const outstanding: (() => void)[] = [];

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
  let installed = true;
  const restore = () => {
    // Undoing twice would reinstate what stood before THIS stub, which by then
    // may be a clipboard a later stub owns.
    if (!installed) {
      return;
    }
    installed = false;
    // Deleting rather than defining `undefined` where there was nothing: a
    // property whose value is undefined and an absent property read the same
    // to this hook but not to every other reader of the navigator.
    if (original === undefined) {
      Reflect.deleteProperty(navigator, "clipboard");
      return;
    }
    Object.defineProperty(navigator, "clipboard", original);
  };
  outstanding.push(restore);
  return { written, restore };
}

/** Hand the navigator back whatever it had before the case started stubbing. */
export function restoreClipboardStubs(): void {
  // Innermost first: each stub captured what stood when IT was installed, so
  // the other order would put an already-replaced stub back on the navigator.
  for (const restore of outstanding.splice(0).reverse()) {
    restore();
  }
}

/** One write the test settles by hand, so two can be settled out of order. */
export type DeferredWrite = Readonly<{
  text: string;
  resolve: () => void;
  reject: () => void;
}>;

/**
 * A clipboard whose writes hang until the test says otherwise.
 *
 * Two presses are two independent writes, and the order they SETTLE in is not
 * the order they were made — which is the only way to prove a control ignores
 * an attempt a newer one has overtaken. `stubClipboard` cannot express it: its
 * writes settle before the click handler returns.
 */
export function stubDeferredClipboard(): Readonly<{
  writes: readonly DeferredWrite[];
  restore: () => void;
}> {
  const writes: DeferredWrite[] = [];
  const original = Object.getOwnPropertyDescriptor(navigator, "clipboard");
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: {
      writeText: (text: string) =>
        new Promise<void>((resolve, reject) => {
          writes.push({
            text,
            resolve,
            reject: () => reject(new Error("refused")),
          });
        }),
    },
  });
  return {
    writes,
    restore: () => {
      if (original === undefined) {
        Reflect.deleteProperty(navigator, "clipboard");
        return;
      }
      Object.defineProperty(navigator, "clipboard", original);
    },
  };
}
