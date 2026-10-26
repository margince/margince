/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { FilterPreview, VocabularyField } from "../../lib/filterdata";
import { FilterResults, previewColumnNames } from "./filterresults";

// The preview answers every non-generated column of the table, so what this
// component decides is which of them a reader sees first. That choice is the
// logic here, and it is provable without rendering — so most of these tests ask
// the function directly, and the render tests cover what only the DOM can show.

describe("previewColumnNames", () => {
  it("leads with the identity column, then the fields the filter named", () => {
    const chosen = previewColumnNames(
      ["id", "full_name", "city", "cf_tier", "created_at"],
      ["cf_tier", "city"],
    );

    // The filter's own order, not the projection's: somebody who filtered on
    // tier first is checking tier first.
    expect(chosen).toEqual(["full_name", "cf_tier", "city", "created_at"]);
  });

  it("spends no column on the id once something else names the row", () => {
    // The projection always carries the primary key, and a UUID beside the name
    // it duplicates is a column spent on nothing a reader can use.
    const chosen = previewColumnNames(["id", "full_name", "city"], []);

    expect(chosen).toEqual(["full_name", "city"]);
  });

  it("prefers a readable name over the id", () => {
    // A UUID identifies a row to the database and to nobody else.
    expect(previewColumnNames(["id", "full_name"], [])[0]).toBe("full_name");
  });

  it("falls back to the id when the record type has no name column", () => {
    expect(previewColumnNames(["id", "amount_minor"], [])[0]).toBe("id");
  });

  it("ignores a filtered field the projection does not carry", () => {
    // A tag leaf reads a join rather than a column, so naming it must not put a
    // `tag` header over cells that will always be empty.
    const chosen = previewColumnNames(["id", "name", "city"], ["tag", "city"]);

    expect(chosen).toEqual(["name", "city"]);
    expect(chosen).not.toContain("tag");
  });

  it("names each column once when the filter names a field twice", () => {
    // A range is two clauses over one field. `fieldsNamed` already dedupes, so
    // this is the belt to that brace: a caller passing a repeat cannot produce
    // two identical column keys, which React would reject as duplicate keys.
    const chosen = previewColumnNames(
      ["id", "name", "cf_score"],
      ["cf_score", "cf_score"],
    );

    expect(chosen).toEqual(["name", "cf_score"]);
  });

  it("stops at six, so the table opens as a table and not a scrollbar", () => {
    const available = [
      "id",
      "name",
      "a",
      "b",
      "c",
      "d",
      "e",
      "f",
      "g",
      "h",
      "i",
    ];

    expect(previewColumnNames(available, [])).toHaveLength(6);
    // And the cap holds when it is the FILTER that names too many, which is the
    // path that would otherwise push the identity column past the ceiling.
    expect(
      previewColumnNames(available, ["a", "b", "c", "d", "e", "f", "g"]),
    ).toEqual(["name", "a", "b", "c", "d", "e"]);
  });

  it("answers nothing when the projection is empty", () => {
    // The shape before the first response arrives. A column list derived from a
    // filter must not invent columns the response never offered.
    expect(previewColumnNames([], ["city"])).toEqual([]);
  });
});

const TIER_FIELD: VocabularyField = {
  name: "cf_loyalty_tier",
  type: "picklist",
  operators: ["eq", "neq", "in", "exists"],
  custom: true,
};

function preview(rows: readonly Record<string, unknown>[]): FilterPreview {
  return {
    resource: "contact",
    match_count: rows.length,
    columns: ["id", "full_name", "cf_loyalty_tier"],
    rows: rows as FilterPreview["rows"],
    truncated: false,
  };
}

// The table reads the companies its rows name, so it needs a query client
// even when no column is a reference.
function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider>{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

function results(rows: readonly Record<string, unknown>[]) {
  return (
    <FilterResults
      preview={preview(rows)}
      fields={[TIER_FIELD]}
      named={["cf_loyalty_tier"]}
      unit="contacts"
      widthsKey="filter-preview-contacts"
      pending={false}
    />
  );
}

afterEach(cleanup);

it("heads a custom column the way the field picker named it", () => {
  render(
    results([{ id: "p1", full_name: "Ann Lee", cf_loyalty_tier: "gold" }]),
    { wrapper },
  );

  // `cf_loyalty_tier` is the storage name; the picker a reader chose the field
  // from reads it as "loyalty tier", and the column it selects has to agree — one
  // field, one name, wherever it appears on this screen.
  expect(
    screen.getByRole("columnheader", { name: /loyalty tier/ }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("columnheader", { name: /cf_loyalty_tier/ }),
  ).toBeNull();
});

it("shows a dash where a row has no value", () => {
  render(results([{ id: "p1", full_name: "Ann Lee", cf_loyalty_tier: null }]), {
    wrapper,
  });

  // Not blank: an empty cell cannot be told apart from a column that failed to
  // render, and "nothing here" is what an `exists` clause is checked against.
  expect(screen.getByText("—")).toBeTruthy();
});

it("words an industry code in its column", () => {
  render(
    <FilterResults
      preview={{
        resource: "company",
        match_count: 1,
        columns: ["id", "display_name", "industry"],
        rows: [{ id: "c1", display_name: "Acme", industry: "healthcare" }],
        truncated: false,
      }}
      fields={[]}
      named={["industry"]}
      unit="companies"
      widthsKey="filter-preview-companies"
      pending={false}
    />,
    { wrapper },
  );

  expect(screen.getByText("Healthcare")).toBeTruthy();
  expect(screen.queryByText("healthcare")).toBeNull();
});

it("says what the caller worded for an empty answer", () => {
  // Only the caller knows which connector the filter ran on, so the advice
  // is its own; the table only has to show it.
  render(
    <FilterResults
      preview={preview([])}
      fields={[TIER_FIELD]}
      named={["cf_loyalty_tier"]}
      unit="contacts"
      widthsKey="filter-preview-contacts"
      emptyNote="No contacts match. Remove the most specific condition."
      pending={false}
    />,
    { wrapper },
  );

  expect(
    screen.getByText("No contacts match. Remove the most specific condition."),
  ).toBeTruthy();
});

describe("a first page of many", () => {
  const PAGE = Array.from({ length: 25 }, (_, index) => ({
    id: `p${index}`,
    full_name: `Contact ${index}`,
    cf_loyalty_tier: "gold",
  }));

  function firstPage(onPerPage: (next: number) => void) {
    return (
      <FilterResults
        preview={{ ...preview(PAGE), match_count: 214, truncated: true }}
        fields={[TIER_FIELD]}
        named={["cf_loyalty_tier"]}
        unit="contacts"
        widthsKey="filter-preview-contacts"
        pending={false}
        total={214}
        perPage={25}
        onPerPage={onPerPage}
      />
    );
  }

  it("counts what matches on the server, on one page of what was sent", () => {
    render(
      firstPage(() => {}),
      { wrapper },
    );

    expect(screen.getByText(/1 to 25 of 214 contacts/)).toBeTruthy();
    // No page past the rows in hand: the next page is asked for, not walked.
    expect(screen.queryByRole("button", { name: "Page 2" })).toBeNull();
  });

  it("hands the size dial's choice back as the next request's page", async () => {
    const asked: number[] = [];
    const user = userEvent.setup();
    render(
      firstPage((next) => asked.push(next)),
      { wrapper },
    );

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Rows per page" }),
      "50 per page",
    );

    expect(asked).toEqual([50]);
  });
});

describe("a column that points at another record", () => {
  const DEAL_FIELDS: VocabularyField[] = [
    {
      name: "owner_id",
      type: "id",
      operators: ["eq"],
      references: "app_user",
      custom: false,
    },
    {
      name: "stage_id",
      type: "id",
      operators: ["eq"],
      references: "stage",
      custom: false,
    },
    {
      name: "company_id",
      type: "id",
      operators: ["eq"],
      references: "company",
      custom: false,
    },
    {
      name: "project_id",
      type: "id",
      operators: ["eq"],
      references: "project",
      custom: false,
    },
    {
      name: "owner_team_id",
      type: "id",
      operators: ["eq"],
      references: "team",
      custom: false,
    },
  ];
  const NAMED = DEAL_FIELDS.map((field) => field.name);

  function jsonResponse(body: unknown, status = 200) {
    return new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Names only u-1, s-1, c-1, p-1 and t-1. Every other id is one the workspace
  // no longer holds, the way a removed seat or a deleted stage is.
  function stubWorkspace({ companiesFail = false } = {}) {
    const fetchMock = vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      const path = url.pathname;
      if (path.endsWith("/companies") && companiesFail) {
        return jsonResponse({ title: "Server error" }, 500);
      }
      if (path.endsWith("/projects/p-1")) {
        return jsonResponse({ id: "p-1", name: "Rollout" });
      }
      if (path.endsWith("/teams")) {
        return jsonResponse({
          data: [{ id: "t-1", name: "Field Sales" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (path.endsWith("/companies")) {
        const asked = url.searchParams.getAll("id");
        return jsonResponse({
          data: asked.includes("c-1")
            ? [{ id: "c-1", display_name: "Brandt GmbH" }]
            : [],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (path.endsWith("/users/names")) {
        return jsonResponse({
          data: [{ id: "u-1", display_name: "Priya Shah" }],
        });
      }
      if (path.endsWith("/stages")) {
        return jsonResponse({ data: [{ id: "s-1", name: "Qualified" }] });
      }
      return jsonResponse({}, 404);
    });
    vi.stubGlobal("fetch", fetchMock);
    return fetchMock;
  }

  function pathsAsked(fetchMock: ReturnType<typeof stubWorkspace>) {
    return fetchMock.mock.calls.map(
      ([request]) => new URL(request.url).pathname,
    );
  }

  function renderDeals(rows: readonly Record<string, unknown>[]) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <FilterResults
            preview={{
              resource: "deal",
              match_count: rows.length,
              columns: ["id", "name", ...NAMED],
              rows: rows as FilterPreview["rows"],
              truncated: false,
            }}
            fields={DEAL_FIELDS}
            named={NAMED}
            unit="deals"
            widthsKey="filter-preview-deals"
            pending={false}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
  }

  afterEach(() => vi.unstubAllGlobals());

  it("names the owner, the stage and the company instead of their ids", async () => {
    stubWorkspace();
    renderDeals([
      {
        id: "d-1",
        name: "Brandt renewal",
        owner_id: "u-1",
        stage_id: "s-1",
        company_id: "c-1",
      },
    ]);

    expect(await screen.findByText("Priya Shah")).toBeTruthy();
    expect(await screen.findByText("Qualified")).toBeTruthy();
    expect(await screen.findByText("Brandt GmbH")).toBeTruthy();
    expect(screen.queryByText("u-1")).toBeNull();
    expect(screen.queryByText("s-1")).toBeNull();
    expect(screen.queryByText("c-1")).toBeNull();
  });

  it("reads every company on the page in one request", async () => {
    const fetchMock = stubWorkspace();
    renderDeals(
      ["c-1", "c-2", "c-3", "c-1"].map((company, index) => ({
        id: `d-${index}`,
        name: `Deal ${index}`,
        owner_id: "u-1",
        stage_id: "s-1",
        company_id: company,
      })),
    );

    expect(await screen.findAllByText("Brandt GmbH")).toHaveLength(2);
    const companyReads = pathsAsked(fetchMock).filter((path) =>
      path.includes("/companies"),
    );
    expect(companyReads).toHaveLength(1);
    expect(companyReads[0]).toMatch(/\/companies$/);
  });

  it("says a reference is gone rather than printing an id nobody can read", async () => {
    stubWorkspace();
    renderDeals([
      {
        id: "d-2",
        name: "Orphaned deal",
        owner_id: "u-removed",
        stage_id: "s-removed",
        company_id: "c-removed",
      },
    ]);

    await waitFor(() =>
      expect(screen.getAllByText(en["filters.resultRefGone"])).toHaveLength(3),
    );
    expect(screen.queryByText("u-removed")).toBeNull();
    expect(screen.queryByText("s-removed")).toBeNull();
    expect(screen.queryByText("c-removed")).toBeNull();
  });

  it("names a project and a team, and says when either is gone", async () => {
    stubWorkspace();
    renderDeals([
      { id: "d-1", name: "Live", project_id: "p-1", owner_team_id: "t-1" },
      {
        id: "d-2",
        name: "Orphaned",
        project_id: "p-removed",
        owner_team_id: "t-removed",
      },
    ]);

    expect(await screen.findByText("Rollout")).toBeTruthy();
    expect(await screen.findByText("Field Sales")).toBeTruthy();
    await waitFor(() =>
      expect(screen.getAllByText(en["filters.resultRefGone"])).toHaveLength(2),
    );
    expect(screen.queryByText("p-removed")).toBeNull();
    expect(screen.queryByText("t-removed")).toBeNull();
  });

  it("says a company's name did not load when the read fails", async () => {
    // A failed read answered nothing about the company, so it may not claim
    // the company is gone.
    stubWorkspace({ companiesFail: true });
    renderDeals([{ id: "d-1", name: "Brandt renewal", company_id: "c-1" }]);

    expect(await screen.findByText(en["ref.nameLoadFailed"])).toBeTruthy();
    expect(screen.queryByText(en["filters.resultRefGone"])).toBeNull();
  });
});
