/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { RecordOriginFacts } from "../../design-system/recordfacts";
import { LocaleProvider } from "../../i18n";
import { CompanyIdentityFacts } from "./companyheaderfacts";

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

function renderOrigin() {
  renderInApp(<RecordOriginFacts record={COMPANY} />);
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
    renderOrigin();

    await waitFor(() => expect(provenanceText()).toBe("Typed by Sofia Meier"));
    expect(screen.queryByText("Typed by a person")).toBeNull();
  });

  it("names the hand that typed it, not a uuid, when the roster cannot resolve them", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderOrigin();

    expect(await screen.findByText("Typed by a person")).toBeTruthy();
    // The id must not reach the page in any form, neither whole nor
    // truncated, which is what the generic record reference would render.
    expect(provenanceText()).toBe("Typed by a person");
    expect(document.body.textContent).not.toContain("u-author");
  });
});

// Every fact keeps its label, printed or for a screen reader only, so the
// line under the name says what each value is.
describe("the facts line", () => {
  it("labels what the account is and who holds it", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderInApp(<CompanyIdentityFacts company={COMPANY} />);

    for (const label of ["Domain", "Industry", "Size", "Owner"]) {
      expect(await screen.findByText(label)).toBeTruthy();
    }
    await waitFor(() => expect(screen.getByText("Mira Voss")).toBeTruthy());
    expect(screen.getByText("brandt.example").tagName).toBe("A");
    expect(screen.queryByText("Created")).toBeNull();
  });

  it("leaves when the row was written to the origin facts", () => {
    stub([]);
    renderOrigin();
    expect(screen.getByText("Created")).toBeTruthy();
  });
});

// A record somebody wrote in the system it was imported from.
//
// An import runs as ONE administrator, so `captured_by` names that seat on
// every row it wrote — true, and useless as a statement about authorship. The
// author field is the one that knows, and the tag has to prefer it: the timeline
// on the same page already reads "Logged in HubSpot by Mutaz Suleiman" while
// this strip said "Typed by you", and the two sat side by side disagreeing.
describe("a record imported from somewhere else", () => {
  it("names who wrote it there, not the seat that ran the import", async () => {
    stub([{ id: "u-author", display_name: "Sofia Meier" }]);
    renderInApp(
      <RecordOriginFacts
        record={{
          ...COMPANY,
          author: { display_name: "Mutaz Suleiman", via: "HubSpot" },
        }}
      />,
    );

    await waitFor(() =>
      expect(provenanceText()).toBe("Logged in HubSpot by Mutaz Suleiman"),
    );
    // The importing seat must not surface at all. Naming it would credit the
    // record to whoever happened to run the import, which is the defect.
    expect(document.body.textContent).not.toContain("Sofia Meier");
    expect(screen.queryByText(/Typed by/)).toBeNull();
  });
});

// A company an import wrote has the importing administrator's seat in
// captured_by. Without its own arm the strip tells that administrator they
// typed the whole file.
describe("a company an import wrote", () => {
  it("an imported company is labelled imported, not typed by you", async () => {
    stub([{ id: "u-reader", display_name: "The Reader" }]);
    renderInApp(
      <RecordOriginFacts
        record={{
          ...COMPANY,
          captured_by: "human:u-reader",
          source: "mirror:csv:pipedrive-export:company:co-12",
        }}
      />,
    );

    await waitFor(() =>
      expect(provenanceText()).toBe("Imported from pipedrive-export"),
    );
    expect(screen.queryByText(/Typed by/)).toBeNull();
  });
});
