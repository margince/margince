/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { CompanyRail } from "./companyrail";

// The Source row under Registration says who wrote the record, in the kind of
// words the design uses. The roster walk is bounded and leaves out archived
// members. A name that cannot be resolved falls back to "Typed by a user",
// because "Typed by 3f2b8c…" is the same non-answer with a reader-hostile
// spelling.

type Company = components["schemas"]["Company"];

// Typed, not asserted: a fixture cast into the contract type can drop a
// required field and still compile.
const COMPANY: Company = {
  writable: true,
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "u-owner",
  captured_by: "human:u-author",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stub(roster: ReadonlyArray<{ id: string; display_name: string }>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      let body: unknown = {
        data: roster,
        page: { has_more: false, next_cursor: null },
      };
      if (pathname.endsWith("/me")) {
        body = { user: { id: "u-reader", display_name: "The Reader" } };
      } else if (
        pathname.endsWith("/profile-fields") ||
        pathname.endsWith("/tags")
      ) {
        body = { data: [] };
      }
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

function renderRail(company: Company) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyRail company={company} loading={false} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The Source row's value, read the way a reader does: the whole text of the
// row's value cell.
function sourceText(): string {
  const term = screen.getByText("Source");
  return (
    term.nextElementSibling?.textContent?.replace(/\s+/g, " ").trim() ?? ""
  );
}

describe("who wrote this record", () => {
  it("names the author the roster can resolve", async () => {
    stub([
      { id: "u-author", display_name: "Sofia Meier" },
      { id: "u-owner", display_name: "Mira Voss" },
    ]);
    renderRail(COMPANY);

    await waitFor(() => expect(sourceText()).toBe("Typed by Sofia Meier"));
    expect(screen.queryByText("Typed by a person")).toBeNull();
  });

  it("names the hand that typed it, not a uuid, when the roster cannot resolve them", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderRail(COMPANY);

    await waitFor(() => expect(sourceText()).toBe("Typed by a person"));
    expect(document.body.textContent).not.toContain("u-author");
  });
});

// An import runs as one administrator, so `captured_by` names that seat on
// every row it wrote: true, and useless as a statement about authorship. The
// author field is the one that knows, and the row has to prefer it.
describe("a record imported from somewhere else", () => {
  it("names who wrote it there, not the seat that ran the import", async () => {
    stub([{ id: "u-author", display_name: "Sofia Meier" }]);
    renderRail({
      ...COMPANY,
      author: { display_name: "Mutaz Suleiman", via: "HubSpot" },
    });

    await waitFor(() =>
      expect(sourceText()).toBe("Logged in HubSpot by Mutaz Suleiman"),
    );
    expect(document.body.textContent).not.toContain("Sofia Meier");
    expect(screen.queryByText(/Typed by/)).toBeNull();
  });

  it("labels a file import as imported, not typed by the reader", async () => {
    stub([{ id: "u-reader", display_name: "The Reader" }]);
    renderRail({
      ...COMPANY,
      captured_by: "human:u-reader",
      source: "mirror:csv:pipedrive-export:company:co-12",
    });

    await waitFor(() =>
      expect(sourceText()).toBe("Imported from pipedrive-export"),
    );
    expect(screen.queryByText(/Typed by/)).toBeNull();
  });
});
