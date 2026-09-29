/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Locale } from "../i18n/locale";
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

const presented: (() => void)[] = [];

afterEach(() => {
  for (const remove of presented.splice(0)) {
    remove();
  }
  vi.restoreAllMocks();
});

function present(locale: Locale): void {
  presented.push(presentOfflinePage(document, locale));
}

function shown(): string[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>("main[lang]:not([hidden])"),
    (block) => block.lang,
  );
}

describe("the offline page", () => {
  it("shows the block in the reader's language, and only that one", () => {
    present("de");
    expect(shown()).toEqual(["de"]);
    expect(document.documentElement.lang).toBe("de");
    expect(document.title).toBe("Keine Verbindung zu Margince");
  });

  it("shows English to an English reader", () => {
    present("en");
    expect(shown()).toEqual(["en"]);
    expect(document.title).toBe("No connection to Margince");
  });

  it("reloads the address the reader asked for on retry", () => {
    const reload = vi
      .spyOn(document.location, "reload")
      .mockImplementation(() => undefined);
    present("vi");
    document
      .querySelector<HTMLElement>("main:not([hidden]) [data-retry]")
      ?.click();
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("reloads by itself once the device is back online", () => {
    const reload = vi
      .spyOn(document.location, "reload")
      .mockImplementation(() => undefined);
    present("en");
    expect(reload).not.toHaveBeenCalled();
    window.dispatchEvent(new Event("online"));
    window.dispatchEvent(new Event("online"));
    expect(reload).toHaveBeenCalledTimes(1);
  });
});
