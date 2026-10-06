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
      // The trail names the list the page belongs to, not a record that
      // failed to load.
      expect(screen.queryByText("Name could not load")).not.toBeInTheDocument();
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
