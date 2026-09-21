/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanyIdentityFacts, CompanySubtitle } from "./companyheaderfacts";

// Who wrote the record, beside when it was written, and the rest of the facts
// strip CompanyIdentityLine used to draw as one running sentence. The tag has
// always been able to name the author (`ProvenanceTag` takes a `renderUser`)
// and the header has always had the roster in hand, because the owner control
// reads it too. Nobody connected the two, so a record every colleague could
// see reported its author as "a contact".
//
// The fallback is the half worth pinning: the roster walk is bounded and the
// list it walks excludes archived members, so a name that cannot be resolved
// must go back to "Typed by a person" rather than forward to the raw uuid.
// "Typed by 3f2b8c…" is not more information than "Typed by a person", it is
// the same non-answer with a reader-hostile spelling.

type Company = components["schemas"]["Company"];

// Typed, not asserted. A fixture cast into the contract type can drop a
// required field and still compile, so the test would go on passing after
// the wire shape moved under it, which is the one thing a fixture must not do.
const COMPANY: Company = {
  writable: true,
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  industry: "Automotive",
  size_band: "51-200",
  domains: [
    {
      id: "d-1",
      domain: "brandt.example",
      is_primary: true,
      source: "manual",
      captured_by: "human:u-author",
    },
  ],
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

// `roster` is what /users answers with, as one complete page, the walk stops
// on a null cursor. An empty one is the honest shape of an author the roster
// does not carry, not a broken stub.
function stub(roster: ReadonlyArray<{ id: string; display_name: string }>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      const body = pathname.endsWith("/me")
        ? { user: { id: "u-reader", display_name: "The Reader" } }
        : { data: roster, page: { has_more: false, next_cursor: null } };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

function renderInApp(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function renderFacts(company: Company = COMPANY) {
  renderInApp(<CompanyIdentityFacts company={company} />);
}

// The tag is one badge carrying "Typed by" and the name as sibling text
// nodes, so the reading a human gets is the badge's whole text, asserting on
// the name alone would pass on markup that never says what the name is
// doing there.
function provenanceText(): string {
  const tag = document.querySelector(".record-fact .badge");
  if (!tag) {
    throw new Error("the facts strip rendered no provenance badge");
  }
  if (tag.classList.contains("badge-ai")) {
    throw new Error("a human-captured record carries the agent tone");
  }
  return tag.textContent?.replace(/\s+/g, " ").trim() ?? "";
}

describe("who wrote this record", () => {
  it("names the author the roster can resolve", async () => {
    stub([
      { id: "u-author", display_name: "Sofia Meier" },
      { id: "u-owner", display_name: "Mira Voss" },
    ]);
    renderFacts();

    await waitFor(() => expect(provenanceText()).toBe("Typed by Sofia Meier"));
    expect(screen.queryByText("Typed by a person")).toBeNull();
  });

  it("names the hand that typed it, not a uuid, when the roster cannot resolve them", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderFacts();

    expect(await screen.findByText("Typed by a person")).toBeTruthy();
    // The id must not reach the page in any form, neither whole nor
    // truncated, which is what the generic record reference would render.
    expect(provenanceText()).toBe("Typed by a person");
    expect(document.body.textContent).not.toContain("u-author");
  });

  // An import runs as ONE administrator, so `captured_by` names that one seat
  // on every row it wrote — and where the reader IS that administrator, `self`
  // is true of a decade of other people's work. The author is the only field
  // on the row that knows who wrote it, so the strip has to read it.
  it("names the author an import carried, not the reader who ran the import", async () => {
    stub([{ id: "u-reader", display_name: "The Reader" }]);
    renderFacts({
      ...COMPANY,
      captured_by: "human:u-reader",
      author: { display_name: "Mutaz Suleiman", via: "hubspot" },
    });

    await waitFor(() =>
      expect(provenanceText()).toBe("Logged in hubspot by Mutaz Suleiman"),
    );
    // Both readings the row would take with the author dropped: "you" once the
    // session lands, and the generic hand before it does.
    expect(screen.queryByText("Typed by you")).toBeNull();
    expect(screen.queryByText("Typed by a person")).toBeNull();
  });
});

// The strip re-houses what CompanyIdentityLine drew as one sentence, as named
// cells, each found by the eyebrow label a reader scans the row by.
describe("the facts strip", () => {
  it("names every cell by its eyebrow label", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderFacts();

    for (const label of ["Domain", "Industry", "Size", "Owner", "Created"]) {
      expect(await screen.findByText(label)).toBeTruthy();
    }
    await waitFor(() => expect(screen.getByText("Mira Voss")).toBeTruthy());
    expect(screen.getByText("brandt.example")).toBeTruthy();
  });
});

// The name line's own subtitle: what the account is and the one way in every
// reader already knows, beside the name rather than mid-sentence under
// everything else the header carries.
describe("CompanySubtitle", () => {
  it("joins industry and domain on one inline line", () => {
    const { container } = renderInApp(<CompanySubtitle company={COMPANY} />);

    // Industry and the domain link sit as siblings on one line, so the line's
    // own text (rather than either half alone) is what a reader scans.
    const line = container.querySelector(".record-sub-inline");
    expect(line?.textContent).toBe("Automotive · brandt.example");
    expect(screen.getByText("brandt.example").tagName).toBe("A");
  });
});
