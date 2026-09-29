/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderOfflinePage } from "./page";
import { presentOfflinePage } from "./present";

beforeEach(() => {
  const built = new DOMParser().parseFromString(
    renderOfflinePage("", "/assets/offline.js"),
    "text/html",
  );
  document.documentElement.lang = built.documentElement.lang;
  document.title = built.title;
  document.body.innerHTML = built.body.innerHTML;
});

function shown(): string[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>("main[lang]:not([hidden])"),
    (block) => block.lang,
  );
}

describe("the offline page", () => {
  it("speaks the language the reader picked, over the browser's", () => {
    presentOfflinePage(document, "de", ["vi-VN", "en-US"]);
    expect(shown()).toEqual(["de"]);
    expect(document.documentElement.lang).toBe("de");
    expect(document.title).toBe("Keine Verbindung zu Margince");
  });

  it("follows the browser's first language it speaks when nobody picked", () => {
    presentOfflinePage(document, null, ["fr-FR", "VI-vn", "de"]);
    expect(shown()).toEqual(["vi"]);
  });

  it("ignores a stored pick it does not speak", () => {
    presentOfflinePage(document, "fr", ["de-AT"]);
    expect(shown()).toEqual(["de"]);
  });

  it("falls back to English", () => {
    presentOfflinePage(document, null, ["ja"]);
    expect(shown()).toEqual(["en"]);
    expect(document.title).toBe("No connection to Margince");
  });

  it("reloads the address the reader asked for on retry", () => {
    const reload = vi
      .spyOn(document.location, "reload")
      .mockImplementation(() => undefined);
    presentOfflinePage(document, null, ["en"]);
    const retry = document.querySelector<HTMLElement>(
      "main:not([hidden]) [data-retry]",
    );
    retry?.click();
    expect(reload).toHaveBeenCalledTimes(1);
  });
});
