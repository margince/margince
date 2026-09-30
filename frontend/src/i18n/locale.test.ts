/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it } from "vitest";
import { STORAGE_KEYS, writeStored } from "../app/storage";
import { preferredLocale } from "./locale";

function browserAsks(languages: readonly string[]): void {
  Object.defineProperty(navigator, "languages", {
    configurable: true,
    value: languages,
  });
}

afterEach(() => {
  localStorage.clear();
  Reflect.deleteProperty(navigator, "languages");
});

describe("the locale a page speaks before the account says", () => {
  it("is the reader's own pick over the browser's language", () => {
    browserAsks(["vi-VN"]);
    writeStored(STORAGE_KEYS.locale, "de");
    expect(preferredLocale()).toBe("de");
  });

  it("is the browser's language when the stored pick is one no longer shipped", () => {
    browserAsks(["fr-FR", "vi-VN"]);
    writeStored(STORAGE_KEYS.locale, "fr");
    expect(preferredLocale()).toBe("vi");
  });

  it("is English when neither names a language Margince speaks", () => {
    browserAsks(["ja-JP"]);
    expect(preferredLocale()).toBe("en");
  });
});
