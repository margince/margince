/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { en } from "../i18n/en";
import type { FilterVocabulary } from "./filterdata";
import { FiltersScreen } from "./filters";
import { mountFilters, type Sent } from "./filters.testkit";
import { liveList } from "./lists.fixtures";
import { savedViewsKey } from "./savedviews.queries";

// The focused new-filter page. What it owns is the WIRING and the order the
// page grows in: which request went out for which record type, when the count
// and the rows are on screen at all, and what the line under the editor says
// while they are not.

const CONTACT_VOCAB: FilterVocabulary = {
  resource: "contact",
  fields: [
    {
      name: "full_name",
      type: "text",
      operators: ["eq", "neq", "in", "contains", "exists"],
      custom: false,
    },
  ],
};

const DEAL_VOCAB: FilterVocabulary = {
  resource: "deal",
  fields: [
    {
      name: "stage_id",
      type: "id",
      operators: ["eq", "neq", "in", "exists"],
      custom: false,
    },
  ],
};

const ADD = en["filters.addClause"];
const START_HINT = "Add a condition to see how many contacts match.";
const FINISH_HINT = "Finish the condition to see how many contacts match.";

/** Every request the screen made, so a test can assert what it asked rather than
 *  inferring it from what rendered. */
function mount(
  preview?: {
    match_count: number;
    columns?: readonly string[];
    rows?: readonly Record<string, unknown>[];
  },
  views: readonly Record<string, unknown>[] = [],
  viewsAnswered: Promise<void> = Promise.resolve(),
) {
  return mountFilters({
    preview,
    views,
    viewsAnswered,
    vocabularies: { contact: CONTACT_VOCAB, deal: DEAL_VOCAB },
  });
}

/** A stored view row, with whatever `query` blob the test is about. */
function viewRow(name: string, query: unknown) {
  return {
    id: `v-${name}`,
    owner_id: "u-1",
    resource: "contacts",
    name,
    query,
    version: 1,
  };
}

const previews = (seen: readonly Sent[]) =>
  seen.filter((sent) => sent.url.includes("/filters/preview"));

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

describe("the calm start", () => {
  it("names the page, offers two ways in, and asks nothing yet", async () => {
    const { seen, wrapper } = mount({ match_count: 4 });
    render(<FiltersScreen id="contacts" />, { wrapper });

    expect(await screen.findByRole("button", { name: ADD })).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 1, name: "New contact filter" }),
    ).toBeTruthy();
    expect(screen.getByRole("group", { name: "Record type" })).toBeTruthy();
    expect(screen.getByLabelText(en["filters.propose.label"])).toBeTruthy();
    expect(
      screen.getByRole("button", { name: en["filters.propose.submit"] }),
    ).toBeTruthy();
    expect(screen.getByText(en["filters.startOr"])).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Build it condition by condition" }),
    ).toBeTruthy();
    // The one emerald control on the page.
    expect(document.querySelectorAll(".btn-primary")).toHaveLength(1);
    expect(screen.getByText(START_HINT)).toBeTruthy();
    // No count, no rows, no save and no export before a condition exists —
    // and the empty tree is not worth a request.
    expect(screen.queryByText("Matching records")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.queryByRole("button", { name: "Save view" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Export CSV" })).toBeNull();
    expect(previews(seen)).toHaveLength(0);
  });

  it("asks for no condition while the fields to write one are read", async () => {
    const { wrapper } = mountFilters({
      vocabularyAnswered: new Promise(() => {}),
      vocabularies: { contact: CONTACT_VOCAB },
    });
    render(<FiltersScreen id="contacts" />, { wrapper });

    expect(
      await screen.findByText(en["filters.loadingVocabulary"]),
    ).toBeTruthy();
    expect(screen.queryByText(START_HINT)).toBeNull();
  });

  it("reads the vocabulary for the record type the route names", async () => {
    const { seen, wrapper } = mount();
    render(<FiltersScreen id="deals" />, { wrapper });

    await waitFor(() => {
      expect(seen.some((sent) => sent.url.includes("resource=deal"))).toBe(
        true,
      );
    });
    // And not the default: a route naming deals must not read the contact
    // vocabulary, or the picker offers fields the deal engine refuses.
    expect(seen.some((sent) => sent.url.includes("resource=contact"))).toBe(
      false,
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "New deal filter" }),
    ).toBeTruthy();
  });

  it("heads itself while the session is still being read", async () => {
    const { wrapper } = mountFilters({ meAnswered: new Promise(() => {}) });
    render(<FiltersScreen id="contacts" />, { wrapper });

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "New contact filter",
      }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: ADD })).toBeNull();
  });
});

describe("growing the filter", () => {
  it("shows results only from the first complete condition", async () => {
    const { wrapper } = mount({ match_count: 12 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    // The row alone: a clause with an empty value is nothing to count yet.
    expect(screen.getByLabelText("Value")).toBeTruthy();
    expect(screen.getByText(FINISH_HINT)).toBeTruthy();
    expect(screen.queryByText("Matching records")).toBeNull();

    await user.type(screen.getByLabelText("Value"), "ann");

    expect(await screen.findByText("12 contacts match")).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 2, name: "Matching records" }),
    ).toBeTruthy();
    expect(screen.getByText(en["filters.resultsCaption"])).toBeTruthy();
    expect(screen.queryByText(FINISH_HINT)).toBeNull();
    // The description folds away above the rows the reader is now building.
    expect(screen.getByText(en["filters.describeChanges"])).toBeTruthy();
  });

  it("keeps the last answer while a second condition is written, and forgets it once the filter empties", async () => {
    const { wrapper, seen } = mount({ match_count: 12 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await screen.findByText("12 contacts match");

    await user.click(screen.getByRole("button", { name: ADD }));
    expect(screen.getByText("12 contacts match")).toBeTruthy();
    expect(screen.getByText(en["filters.hint.update"])).toBeTruthy();

    for (const remove of screen.getAllByRole("button", {
      name: "Remove Name condition",
    })) {
      await user.click(remove);
    }
    expect(await screen.findByText(START_HINT)).toBeTruthy();
    expect(screen.queryByText("Matching records")).toBeNull();

    // The preview cache still holds the answer for "ann", and an unfinished
    // condition must not bring it back.
    const asked = previews(seen).length;
    await user.click(screen.getByRole("button", { name: ADD }));
    expect(screen.getByText(FINISH_HINT)).toBeTruthy();
    expect(screen.queryByText("Matching records")).toBeNull();
    expect(screen.queryByText("12 contacts match")).toBeNull();
    expect(previews(seen)).toHaveLength(asked);
  });

  it("names the count after the record type, not after a placeholder", async () => {
    const { wrapper } = mount({ match_count: 3 });
    const user = userEvent.setup();
    render(<FiltersScreen id="deals" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "s1");

    // "3 deals match", not "3 contacts match" and not "3 match" — the object is
    // part of the sentence, which is why the copy is keyed per object.
    expect(await screen.findByText("3 deals match")).toBeTruthy();
  });

  // Which columns get chosen is proved directly against `previewColumnNames`;
  // what this asserts is the wiring — that the rows behind the count arrive.
  it("shows the rows behind the count", async () => {
    const { wrapper } = mount({
      match_count: 1,
      columns: ["id", "full_name", "city"],
      rows: [{ id: "p1", full_name: "Ann Lee", city: "Berlin" }],
    });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");

    expect(await screen.findByText("Ann Lee")).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: /^Name/ })).toBeTruthy();
  });

  it("says nothing about the count while the first one is on its way", async () => {
    let answer = () => {};
    const { wrapper } = mountFilters({
      preview: { match_count: 2 },
      previewAnswered: new Promise<void>((resolve) => {
        answer = resolve;
      }),
      vocabularies: { contact: CONTACT_VOCAB },
    });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");

    // The results are waiting, and nothing claims there is no count or asks
    // the reader to add what they just finished.
    expect(
      screen.getByRole("heading", { name: "Matching records" }),
    ).toBeTruthy();
    expect(screen.queryByText(/match/)).toBeNull();
    expect(screen.queryByText(FINISH_HINT)).toBeNull();
    answer();
    expect(await screen.findByText("2 contacts match")).toBeTruthy();
  });
});

describe("keeping the reader's place", () => {
  // The control pressed is often gone after the press: the calm start goes
  // when it is used and comes back when the last row goes. Focus left there
  // falls to <body>, and a keyboard reader starts again from the top.
  it("moves focus into the row the calm start added, and back when it goes", async () => {
    const { wrapper } = mount();
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    (await screen.findByRole("button", { name: ADD })).focus();
    await user.keyboard("{Enter}");
    expect(document.activeElement).toBe(
      screen.getByRole("combobox", { name: "Field" }),
    );

    screen.getByRole("button", { name: "Remove Name condition" }).focus();
    await user.keyboard("{Enter}");
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: ADD }),
    );
  });

  it("moves focus into each row the reader adds after the first", async () => {
    const { wrapper } = mount();
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    screen.getByRole("button", { name: ADD }).focus();
    await user.keyboard("{Enter}");

    expect(document.activeElement).toBe(
      screen.getAllByRole("combobox", { name: "Field" })[1],
    );
  });
});

describe("asking for more rows", () => {
  const MANY = Array.from({ length: 214 }, (_, index) => ({
    id: `p${index}`,
    full_name: `Contact ${index}`,
  }));

  it("asks for up to 100 and shows them on one page", async () => {
    const { seen, wrapper } = mount({
      match_count: 214,
      columns: ["id", "full_name"],
      rows: MANY,
    });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await user.click(
      await screen.findByRole("button", { name: en["filters.showMore"] }),
    );

    expect(await screen.findByText("Contact 99")).toBeTruthy();
    const last = previews(seen).at(-1)?.body as { limit?: number };
    expect(last.limit).toBe(100);
    // One page holding every row that came: the dial and the request agree.
    expect(screen.queryByRole("button", { name: "Page 2" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["filters.showMore"] }),
    ).toBeNull();
  });

  it("re-asks with the size the table's own dial chose", async () => {
    const { seen, wrapper } = mount({
      match_count: 214,
      columns: ["id", "full_name"],
      rows: MANY,
    });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await screen.findByText("Contact 24");
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Rows per page" }),
      "50 per page",
    );

    expect(await screen.findByText("Contact 49")).toBeTruthy();
    const last = previews(seen).at(-1)?.body as { limit?: number };
    expect(last.limit).toBe(50);
  });
});

describe("switching the record type", () => {
  it("goes straight to the other type while there is nothing to lose", async () => {
    const { wrapper } = mount();
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: "Companies" }));

    expect(window.location.hash).toBe("#/filters/companies");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("asks before clearing conditions that name the other type's fields", async () => {
    const { wrapper } = mount();
    const user = userEvent.setup();
    const { rerender } = render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.click(screen.getByRole("button", { name: "Companies" }));

    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain("Switch to companies?");
    expect(dialog.textContent).toContain(en["filters.switch.body.contacts"]);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    // The condition is still there, and the address did not move.
    expect(screen.getByLabelText("Value")).toBeTruthy();
    expect(window.location.hash).toBe("");

    await user.click(screen.getByRole("button", { name: "Companies" }));
    await user.click(
      await screen.findByRole("button", { name: "Switch and clear" }),
    );
    expect(window.location.hash).toBe("#/filters/companies");
    // The address names the new type, and the page it opens starts calm.
    rerender(<FiltersScreen id="companies" />);
    expect(await screen.findByRole("button", { name: ADD })).toBeTruthy();
    expect(screen.queryByLabelText("Value")).toBeNull();
  });
});

describe("saving and exporting", () => {
  it("offers no save until the filter is one the engine would accept", async () => {
    const { wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    // A clause with an empty value is refused per-leaf as filter_value_invalid,
    // so saving it would store a view nobody can open.
    expect(screen.queryByRole("button", { name: "Save view" })).toBeNull();

    await user.type(screen.getByLabelText("Value"), "ann");

    expect(
      await screen.findByRole("button", { name: "Save view" }),
    ).toBeTruthy();
  });

  it("saves the tree under the key the server validates as a filter", async () => {
    const { written, wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await user.click(await screen.findByRole("button", { name: "Save view" }));
    await user.type(screen.getByLabelText("Name"), "Anns");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(written).toHaveLength(1);
    });
    // `contacts`, not `contact` — the two endpoint families spell the same object
    // differently, and sending the filter vocabulary's word here would 422.
    // And the tree goes under `filter`, carrying no editor ids.
    expect(written[0]?.body).toEqual({
      resource: "contacts",
      name: "Anns",
      query: {
        filter: { and: [{ field: "full_name", op: "eq", value: "ann" }] },
      },
    });
  });

  it("draws the footer band only once there is something to keep", async () => {
    const { wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });
    const editor = () =>
      screen
        .getByRole("heading", { name: "Find contacts where…" })
        .closest(".panel");

    await user.click(await screen.findByRole("button", { name: ADD }));
    // A band drawn for an unfinished filter would rule an empty strip under
    // the editor: everything in it sends the tree, and this one is refused.
    expect(editor()?.querySelector(".panel-foot")).toBeNull();

    await user.type(screen.getByLabelText("Value"), "ann");

    expect(editor()?.querySelector(".panel-foot")).not.toBeNull();
  });

  it("exports the filter on screen, under the name the server gave it", async () => {
    const createObjectURL = vi.fn(() => "blob:test");
    const revokeObjectURL = vi.fn();
    Object.defineProperties(URL, {
      createObjectURL: { configurable: true, value: createObjectURL },
      revokeObjectURL: { configurable: true, value: revokeObjectURL },
    });
    const anchors: HTMLAnchorElement[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      anchors.push(this);
    });
    const { written, wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await user.click(await screen.findByRole("button", { name: "Export CSV" }));

    await waitFor(() => {
      expect(written).toHaveLength(1);
    });
    // The tree on screen, not a saved view's id: what gets exported is what the
    // count just said, through the one filter engine.
    expect(written[0]?.body).toEqual({
      object: "contact",
      filter: { and: [{ field: "full_name", op: "eq", value: "ann" }] },
      format: "csv",
    });
    expect(anchors[0]?.download).toBe("contacts-slice.csv");
  });

  it("says so when an export is refused, instead of leaving the reader waiting", async () => {
    const { wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await screen.findByText("2 contacts match");
    // Only the export's answer changes, so the failure path is the real one —
    // a Problem body the screen has to read, not a thrown string.
    vi.stubGlobal("fetch", refusing("/exports", REFUSED_EXPORT, 403));
    await user.click(screen.getByRole("button", { name: "Export JSON" }));

    // The SERVER's reason, not "request failed": a refused bulk read can be
    // refused for something a reader can act on.
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Bulk record read is human-only.",
    );
  });
});

const REFUSED_EXPORT = {
  title: "Export refused",
  status: 403,
  detail: "Bulk record read is human-only.",
};

/**
 * A stub answering one route with a problem body and everything else
 * normally, so the vocabulary still arrives and the clause on screen is still
 * complete: what the screen says about the refusal is the whole assertion.
 */
function refusing(route: string, body: unknown, status: number) {
  const json = (payload: unknown) =>
    new Response(JSON.stringify(payload), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes(route)) {
      return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/problem+json" },
      });
    }
    if (url.includes("/filters/vocabulary")) {
      return json(CONTACT_VOCAB);
    }
    if (url.endsWith("/v1/me")) {
      return json(meFixture({}));
    }
    return json({ data: [], page: { next_cursor: null, has_more: false } });
  });
}

/**
 * Add a second clause, which is a new query key and therefore a new request.
 * The refusal has to be the answer ON SCREEN, and react-query keeps the
 * previous key's success: re-asking the same one would read a cache hit.
 */
async function addSecondClause(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: ADD }));
  await user.type(screen.getAllByLabelText("Value")[1], "lee");
}

describe("a refused count", () => {
  it("reads a refused preview as a failure, not as an unwritten filter", async () => {
    const { wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await screen.findByText("2 contacts match");

    // A bodiless 502 — a proxy between the client and the app — is the failure
    // with the least to say, so what the screen says instead is all its own.
    vi.stubGlobal("fetch", refusing("/filters/preview", {}, 502));
    await addSecondClause(user);

    // Not a hint to finish a condition: two complete conditions are on
    // screen, and blaming the reader for the server's refusal hides it.
    expect(await screen.findByText("Count unavailable")).toBeTruthy();
    expect(screen.queryByText(en["filters.hint.update"])).toBeNull();
    expect(
      screen.getByRole("heading", { name: "Matching records" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
    expect((await screen.findByRole("alert")).textContent).toContain(
      "The request failed. No cause reported.",
    );
  });

  // `/filters/preview` is a POST, and the seat ceiling refuses every mutating
  // method for a read seat BEFORE any role is consulted. So a read-seat member
  // reads the vocabulary, builds a clause, and can never get a count.
  it("names the seat when a read seat is refused a preview", async () => {
    const { wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await screen.findByText("2 contacts match");

    vi.stubGlobal(
      "fetch",
      refusing(
        "/filters/preview",
        {
          title: "Forbidden",
          status: 403,
          code: "seat_tier_insufficient",
          detail: "seat tier insufficient",
        },
        403,
      ),
    );
    await addSecondClause(user);

    // The server's own detail is the bare sentinel: it names a concept no
    // reader has met, so the catalog copy replaces it.
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("This seat is read-only");
    expect(alert.textContent).toContain(
      "Ask an administrator to upgrade the seat.",
    );
    expect(alert.textContent).not.toContain("seat tier insufficient");
    expect(screen.getByText("Count unavailable")).toBeTruthy();
  });
});

describe("an opened saved view", () => {
  it("opens the view the address names, already loaded, under its name", async () => {
    const { wrapper } = mount({ match_count: 4 }, [
      viewRow("Other", {
        filter: { and: [{ field: "full_name", op: "contains", value: "bob" }] },
      }),
      viewRow("Berliners", {
        filter: { and: [{ field: "full_name", op: "contains", value: "ann" }] },
      }),
    ]);
    render(<FiltersScreen id="contacts" view="v-Berliners" />, { wrapper });

    expect(await screen.findByDisplayValue("ann")).toBeTruthy();
    expect(screen.queryByDisplayValue("bob")).toBeNull();
    expect(
      screen.getByRole("heading", { level: 1, name: "Berliners" }),
    ).toBeTruthy();
    expect(await screen.findByText("4 contacts match")).toBeTruthy();
  });

  it("holds the editor until the addressed view is read, so no edit is overwritten", async () => {
    let answer = () => {};
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const { wrapper, seen } = mount(
      { match_count: 4 },
      [
        viewRow("Berliners", {
          filter: {
            and: [{ field: "full_name", op: "contains", value: "ann" }],
          },
        }),
      ],
      answered,
    );
    render(<FiltersScreen id="contacts" view="v-Berliners" />, { wrapper });

    // The session has answered and the views are being read.
    await waitFor(() =>
      expect(seen.some((sent) => sent.url.includes("/views"))).toBe(true),
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Saved view" }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: ADD })).toBeNull();
    answer();
    expect(await screen.findByDisplayValue("ann")).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 1, name: "Berliners" }),
    ).toBeTruthy();
  });

  it("reads the views again when the cached ones predate the addressed view", async () => {
    const berliners = viewRow("Berliners", {
      filter: { and: [{ field: "full_name", op: "contains", value: "ann" }] },
    });
    const { wrapper, client } = mount({ match_count: 4 }, [berliners]);
    client.setQueryData(savedViewsKey("contacts"), []);
    render(<FiltersScreen id="contacts" view="v-Berliners" />, { wrapper });

    expect(await screen.findByDisplayValue("ann")).toBeTruthy();
  });

  it("keeps what the reader built while the views are read again", async () => {
    let reread = () => {};
    const { wrapper, client, seen } = mountFilters({
      views: [],
      viewsReread: new Promise<void>((resolve) => {
        reread = resolve;
      }),
      vocabularies: { contact: CONTACT_VOCAB },
    });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" view="v-Gone" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    // What a save on this page does to every views read, and what a refetch
    // on focus does to this one: the page is open while the answer is out.
    const again = client.invalidateQueries({
      queryKey: savedViewsKey("contacts"),
    });
    await waitFor(() =>
      expect(seen.filter((sent) => sent.url.includes("/views"))).toHaveLength(
        2,
      ),
    );
    reread();
    await act(() => again);

    expect(screen.getByDisplayValue("ann")).toBeTruthy();
  });

  it("opens an empty editor when the addressed view is gone", async () => {
    const { wrapper } = mount({ match_count: 4 }, []);
    render(<FiltersScreen id="contacts" view="v-Gone" />, { wrapper });

    expect(await screen.findByRole("button", { name: ADD })).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 1, name: "Saved view" }),
    ).toBeTruthy();
    expect(screen.queryByDisplayValue("ann")).toBeNull();
  });
});

describe("a Live List's filter", () => {
  it("heads itself while the list is still being read", async () => {
    let answer = () => {};
    const { wrapper } = mountFilters({
      listsOn: true,
      lists: [liveList],
      listAnswered: new Promise<void>((resolve) => {
        answer = resolve;
      }),
    });
    render(<FiltersScreen id="list" view={liveList.id} />, { wrapper });

    expect(
      await screen.findByRole("heading", { level: 1, name: "Edit filter" }),
    ).toBeTruthy();
    answer();
    expect(
      await screen.findByRole("heading", { level: 1, name: liveList.name }),
    ).toBeTruthy();
  });
});

// The shell's page head stands down on a focused page (app/pagemeta.ts,
// headsItself; shell.test holds that side), so the page prints its one h1.
it("names itself once, with the record type beside its name", async () => {
  const { wrapper } = mount({ match_count: 4 });
  render(<FiltersScreen id="contacts" />, { wrapper });

  await screen.findByRole("button", { name: ADD });
  expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  // The record type stays: everything on the page reads from it.
  expect(
    screen.getByRole("button", { name: en["filters.tab.contacts"] }),
  ).toBeTruthy();
});
