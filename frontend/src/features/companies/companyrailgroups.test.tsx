/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { listsMe, shortlist } from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
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

// captured_by names only the agent's passport. The create audit row behind
// `created_by` is what knows the colleague and the tool.
describe("a record an agent created", () => {
  const AGENT = "agent:0b9f3c2a-6d7e-4c1b-9a8f-1e2d3c4b5a69";

  it("names the colleague the agent worked for and the tool", async () => {
    stub([]);
    renderRail({
      ...COMPANY,
      captured_by: AGENT,
      created_by: {
        actor_type: "agent",
        actor_id: AGENT,
        on_behalf_of_name: "Anna Weber",
        agent_client: "Claude",
      },
    });

    await waitFor(() =>
      expect(sourceText()).toBe("Made by Claude for Anna Weber"),
    );
  });

  it("says only that an agent made it when the create row is not known", async () => {
    stub([]);
    renderRail({ ...COMPANY, captured_by: AGENT });

    await waitFor(() => expect(sourceText()).toBe("Automated by an agent"));
  });
});

describe("the Lists group", () => {
  function serve(me: () => Response, onLists: readonly unknown[] = []) {
    installFetchStub({
      "GET /me": me,
      [`GET /records/company/${COMPANY.id}/lists`]: () =>
        jsonResponse({ data: onLists }),
    });
    render(
      <StoryProviders>
        <CompanyRail company={COMPANY} loading={false} />
      </StoryProviders>,
    );
  }

  const group = async () =>
    (
      await screen.findByRole("heading", { name: en["lists.record.title"] })
    ).closest("section") as HTMLElement;

  it("offers Add to Shortlist and every list while the company is on none", async () => {
    serve(listsMe(true));
    const lists = within(await group());
    expect(
      await lists.findByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeInTheDocument();
    expect(
      lists.getByRole("link", { name: en["lists.record.allLists"] }),
    ).toHaveAttribute("href", "/#/filters/lists");
    expect(lists.queryByRole("combobox")).toBeNull();
  });

  it("keeps the lists it is on beside the verbs", async () => {
    serve(listsMe(true), [shortlist]);
    const lists = within(await group());
    expect(await lists.findByText(shortlist.name)).toBeInTheDocument();
    expect(
      lists.getByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeInTheDocument();
  });
});
