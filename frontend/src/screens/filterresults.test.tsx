/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import type { FilterPreview, VocabularyField } from "./filterdata";
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

function wrapper({ children }: { children: ReactNode }) {
  return <LocaleProvider>{children}</LocaleProvider>;
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

it("says no records match, and how to loosen the filter", () => {
  render(results([]), { wrapper });

  expect(
    screen.getByText(
      "No contacts match these conditions. Loosen one: switch a connector to “or”, or remove the most specific condition.",
    ),
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
