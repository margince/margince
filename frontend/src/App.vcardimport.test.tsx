// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  memoryStorage,
  renderApp,
  sessionOnlyFetch,
  teardownApp,
} from "./testing/appharness";
import { meFixture, READS_RECORDS } from "./testing/mefixture";
// The screens these cases open, loaded while the file is collected. A screen's
// first import costs seconds; inside a case it races the case's own waiters.
import "./features/contacts/contacts";
import { en as englishCopy } from "./i18n/en";

// The import writes contacts, so the seat holds the two grants it asks.
const IMPORTER = meFixture({
  allow: { ...READS_RECORDS, contact: ["read", "create", "update"] },
});

// `#/contacts/import` is asserted through the real router, shell and trail:
// the screens alone cannot see `import` being read as a contact id.

const SETTLE_MS = 10_000;

function requestedUrls(fetchSpy: ReturnType<typeof vi.fn>): string[] {
  return fetchSpy.mock.calls.map(([input]) =>
    String(input instanceof Request ? input.url : input),
  );
}

let fetchSpy: ReturnType<typeof vi.fn>;

// Defined on the instance below, so the browser's own answer is put back after.
const browserLanguages = Object.getOwnPropertyDescriptor(
  globalThis.navigator,
  "languages",
);

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  Object.defineProperty(globalThis.navigator, "languages", {
    value: ["en-US"],
    configurable: true,
  });
  const session = sessionOnlyFetch();
  fetchSpy = vi.fn(async (input: Request | string | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (!url.endsWith("/v1/me")) {
      return session(input);
    }
    return new Response(JSON.stringify(IMPORTER), {
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(async () => {
  cleanup();
  await teardownApp();
  vi.unstubAllGlobals();
  window.location.hash = "";
  if (browserLanguages) {
    Object.defineProperty(globalThis.navigator, "languages", browserLanguages);
  } else {
    Reflect.deleteProperty(globalThis.navigator, "languages");
  }
});

describe("the vCard import address", () => {
  it(
    "opens the import page on a deep link without reading a contact",
    async () => {
      window.location.hash = "#/contacts/import";
      renderApp();

      expect(
        await screen.findByRole(
          "heading",
          { level: 1, name: englishCopy["vcardImport.title"] },
          { timeout: SETTLE_MS },
        ),
      ).toBeInTheDocument();
      expect(screen.getByTestId("vcard-import-file")).toBeInTheDocument();
      expect(
        requestedUrls(fetchSpy).filter((url) =>
          /\/v1\/contacts\/import\b/.test(url),
        ),
      ).toEqual([]);
      // The page stands below the Contacts list: the bar leads back to it, and
      // the Contacts row is its ancestor rather than the page.
      expect(screen.queryByText("Name could not load")).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Back" })).toBeInTheDocument();
      expect(document.querySelectorAll('[aria-current="page"]')).toHaveLength(
        0,
      );
      expect(
        screen.getByRole("link", { name: "Contacts", current: true }),
      ).toHaveAttribute("href", "/#/contacts");
    },
    SETTLE_MS * 2,
  );

  it(
    "returns to the contact list on the browser's Back",
    async () => {
      window.location.hash = "#/contacts";
      renderApp();
      await waitFor(
        () =>
          expect(
            screen.getByRole("heading", { level: 1, name: "Contacts" }),
          ).toBeInTheDocument(),
        { timeout: SETTLE_MS },
      );

      window.location.hash = "#/contacts/import";
      await screen.findByRole(
        "heading",
        { level: 1, name: englishCopy["vcardImport.title"] },
        { timeout: SETTLE_MS },
      );

      globalThis.history.back();
      await waitFor(
        () =>
          expect(
            screen.getByRole("heading", { level: 1, name: "Contacts" }),
          ).toBeInTheDocument(),
        { timeout: SETTLE_MS },
      );
      expect(window.location.hash.split("?")[0]).toBe("#/contacts");
      expect(
        screen.queryByRole("heading", {
          name: englishCopy["vcardImport.title"],
        }),
      ).not.toBeInTheDocument();
    },
    SETTLE_MS * 3,
  );
});
