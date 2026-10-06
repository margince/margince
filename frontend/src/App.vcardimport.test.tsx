// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  memoryStorage,
  renderApp,
  sessionOnlyFetch,
} from "./testing/appharness";

// `#/contacts/import` is asserted through the real router, shell and trail:
// the screens alone cannot see `import` being read as a contact id.

const SETTLE_MS = 10_000;

function requestedUrls(fetchSpy: ReturnType<typeof vi.fn>): string[] {
  return fetchSpy.mock.calls.map(([input]) =>
    String(input instanceof Request ? input.url : input),
  );
}

let fetchSpy: ReturnType<typeof vi.fn>;

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  Object.defineProperty(globalThis.navigator, "languages", {
    value: ["en-US"],
    configurable: true,
  });
  fetchSpy = vi.fn(sessionOnlyFetch());
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
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
          { level: 1, name: "Import vCards" },
          { timeout: SETTLE_MS },
        ),
      ).toBeInTheDocument();
      expect(screen.getByTestId("vcard-import-file")).toBeInTheDocument();
      expect(
        requestedUrls(fetchSpy).filter((url) =>
          /\/v1\/contacts\/import\b/.test(url),
        ),
      ).toEqual([]);
      // The trail leads back to the list and ends in this page, and only the
      // trail claims to be the page: the Contacts row is its ancestor.
      expect(screen.queryByText("Name could not load")).not.toBeInTheDocument();
      const trail = screen.getByRole("navigation", { name: "Breadcrumb" });
      expect(
        within(trail).getByRole("link", { name: "Contacts" }),
      ).toHaveAttribute("href", "#/contacts");
      expect(trail.querySelector('[aria-current="page"]')).toHaveTextContent(
        "Import vCards",
      );
      expect(document.querySelectorAll('[aria-current="page"]')).toHaveLength(
        1,
      );
      expect(
        screen.getByRole("link", { name: "Contacts", current: true }),
      ).toHaveAttribute("href", "#/contacts");
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
        { level: 1, name: "Import vCards" },
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
        screen.queryByRole("heading", { name: "Import vCards" }),
      ).not.toBeInTheDocument();
    },
    SETTLE_MS * 3,
  );
});
