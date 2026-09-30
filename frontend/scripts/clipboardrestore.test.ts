/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// That a stubbed clipboard does not outlive the case that installed it.
//
// `stubClipboard` replaces `navigator.clipboard` outright, so a case that fails
// an assertion before undoing it hands every later case in the file a clipboard
// nobody asked for: the real failure is followed by a run of unrelated ones and
// is no longer the interesting line in the report. vitest.setup.ts arms
// `restoreClipboardStubs` for every suite, and this is what fails if that stops.
//
// What makes the guard necessary is that the failure is SILENT. A hook that was
// never registered is reported by nothing, and the stub it would have undone
// breaks a LATER file only sometimes — so the guard is behavioural rather than
// a check that some hook exists: one case stubs and never restores, and the
// next asks what it was left holding.

import { expect, it } from "vitest";
import {
  stubClipboard,
  stubDeferredClipboard,
} from "../src/design-system/clipboard-testing";

const pristine = navigator.clipboard;

it("installs a stub and deliberately never takes it back", () => {
  stubClipboard("accepts");

  expect(navigator.clipboard).not.toBe(pristine);
});

it("starts on the clipboard the case before it did not hand back", () => {
  expect(navigator.clipboard).toBe(pristine);
});

// Every stub installs through one path, and this is what fails if a new one is
// ever written that forgets to: the deferred stub was added later and did not
// register, so the leak came back the moment a second installer existed.
it("installs a deferred stub and deliberately never takes it back", () => {
  stubDeferredClipboard();

  expect(navigator.clipboard).not.toBe(pristine);
});

it("starts on the clipboard the deferred case did not hand back", () => {
  expect(navigator.clipboard).toBe(pristine);
});
