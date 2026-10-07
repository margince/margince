/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { en } from "../i18n/en";
import type { FilterVocabulary } from "./filterdata";
import { FiltersScreen } from "./filters";
import { GuardedFilters, mountFilters, type Sent } from "./filters.testkit";

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
const SAVE = en["filters.save"];
const START_HINT = "Add a condition to see how many contacts match.";
const FINISH_HINT = "Finish the condition to see how many contacts match.";

/** Every request the screen made, so a test can assert what it asked rather than
 *  inferring it from what rendered. */
function mount(preview?: {
  match_count: number;
  columns?: readonly string[];
  rows?: readonly Record<string, unknown>[];
}) {
  return mountFilters({
    preview,
    vocabularies: { contact: CONTACT_VOCAB, deal: DEAL_VOCAB },
  });
}

const previews = (seen: readonly Sent[]) =>
  seen.filter((sent) => sent.url.includes("/filters/preview"));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(URL, "createObjectURL");
  Reflect.deleteProperty(URL, "revokeObjectURL");
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
    expect(
      screen.getByLabelText("Describe the contacts you want"),
    ).toBeTruthy();
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
    expect(screen.queryByRole("button", { name: SAVE })).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["filters.footMore"] }),
    ).toBeNull();
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
    expect(screen.queryByText(/contacts match/)).toBeNull();
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

describe("keeping and exporting", () => {
  const editor = () =>
    screen
      .getByRole("heading", { name: "Find contacts where…" })
      .closest(".panel");

  it("draws the footer band only once the filter is one the engine would accept", async () => {
    const { wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    // A clause with an empty value is refused per-leaf as filter_value_invalid,
    // so every verb in the band would send a tree the engine refuses.
    expect(editor()?.querySelector(".panel-foot")).toBeNull();
    expect(screen.queryByRole("button", { name: SAVE })).toBeNull();

    await user.type(screen.getByLabelText("Value"), "ann");

    const foot = await screen.findByText(en["filters.unsavedFilter"]);
    expect(foot.closest(".panel-foot")).not.toBeNull();
    expect(
      screen.getByRole("button", { name: en["filters.footMore"] }),
    ).toBeTruthy();
    // From here the emerald belongs to Save, and to nothing else.
    const primary = document.querySelectorAll(".btn-primary");
    expect(primary).toHaveLength(1);
    expect(primary[0]?.textContent).toBe(SAVE);
  });

  /** A page whose export waits for `answer`, with the file it hands over caught. */
  function mountHeldExport() {
    Object.defineProperties(URL, {
      createObjectURL: { configurable: true, value: () => "blob:test" },
      revokeObjectURL: { configurable: true, value: () => undefined },
    });
    const anchors: HTMLAnchorElement[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      anchors.push(this);
    });
    let answer = () => {};
    const { written, wrapper } = mountFilters({
      preview: { match_count: 2 },
      vocabularies: { contact: CONTACT_VOCAB },
      exportAnswered: new Promise<void>((resolve) => {
        answer = resolve;
      }),
    });
    render(<FiltersScreen id="contacts" />, { wrapper });
    return { anchors, written, answer: () => answer() };
  }

  /** One complete condition, then More › Export CSV. */
  async function exportCsv(user: ReturnType<typeof userEvent.setup>) {
    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await user.click(
      await screen.findByRole("button", { name: en["filters.footMore"] }),
    );
    await user.click(screen.getByRole("button", { name: "Export CSV" }));
  }

  it("exports the filter on screen from More, under the name the server gave it", async () => {
    const { anchors, written, answer } = mountHeldExport();
    const user = userEvent.setup();
    await exportCsv(user);

    // The menu closed on the press, so the band is what says a file is coming.
    const exporting = await screen.findByText(en["filters.exporting"]);
    expect(exporting.closest(".panel-foot")).not.toBeNull();
    expect(exporting.getAttribute("role")).toBe("status");
    answer();
    await waitFor(() =>
      expect(screen.queryByText(en["filters.exporting"])).toBeNull(),
    );
    expect(written).toHaveLength(1);
    // The tree on screen, not a saved view's id: what gets exported is what the
    // count just said, through the one filter engine.
    expect(written[0]?.body).toEqual({
      object: "contact",
      filter: { and: [{ field: "full_name", op: "eq", value: "ann" }] },
      format: "csv",
    });
    expect(anchors[0]?.download).toBe("contacts-slice.csv");
  });

  it("stops saying a file is coming once the filter on screen is not the one exported", async () => {
    const { anchors, written, answer } = mountHeldExport();
    const user = userEvent.setup();
    await exportCsv(user);
    expect(await screen.findByText(en["filters.exporting"])).toBeTruthy();

    await user.type(screen.getByLabelText("Value"), "a");
    expect(screen.getByText(en["filters.unsavedFilter"])).toBeTruthy();
    expect(screen.queryByText(en["filters.exporting"])).toBeNull();

    // The file still arrives, and it is the filter that asked for it.
    answer();
    await waitFor(() => expect(anchors).toHaveLength(1));
    expect(written[0]?.body).toMatchObject({
      filter: { and: [{ value: "ann" }] },
    });
  });

  it("says so beside the footer when an export is refused", async () => {
    const { wrapper } = mount({ match_count: 2 });
    const user = userEvent.setup();
    render(<FiltersScreen id="contacts" />, { wrapper });

    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
    await screen.findByText("2 contacts match");
    // Only the export's answer changes, so the failure path is the real one —
    // a Problem body the screen has to read, not a thrown string.
    const served = globalThis.fetch;
    vi.stubGlobal("fetch", refusing("/exports", REFUSED_EXPORT, 403));
    await user.click(
      screen.getByRole("button", { name: en["filters.footMore"] }),
    );
    await user.click(screen.getByRole("button", { name: "Export JSON" }));

    // The SERVER's reason, not "request failed": a refused bulk read can be
    // refused for something a reader can act on. The menu that asked has
    // closed, so the band is where it lands.
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Bulk record read is human-only.");
    expect(alert.closest(".panel-foot")).not.toBeNull();

    // The refusal was about that filter: one rebuilt after the footer went
    // away is a filter nobody has tried to export.
    vi.stubGlobal("fetch", served);
    await user.clear(screen.getByLabelText("Value"));
    expect(screen.queryByText(en["filters.unsavedFilter"])).toBeNull();
    await user.type(screen.getByLabelText("Value"), "ann");
    expect(await screen.findByText(en["filters.unsavedFilter"])).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

const GUARDED_PAGES: Readonly<Record<string, "contacts" | "companies">> = {
  "#/filters/contacts": "contacts",
  "#/filters/companies": "companies",
};

/** A new filter's address draws the page; every other is somewhere else. */
const newFilterAt = (address: string) => {
  const tab = GUARDED_PAGES[address];
  return tab ? <FiltersScreen key={address} id={tab} /> : null;
};

describe("leaving a filter", () => {
  const ASKS = en["unsaved.title"];

  function guarded(listsOn = false) {
    window.location.hash = "#/filters/contacts";
    const { wrapper } = mountFilters({
      listsOn,
      preview: { match_count: 2 },
      vocabularies: { contact: CONTACT_VOCAB },
    });
    render(<GuardedFilters page={newFilterAt} />, { wrapper });
  }

  async function complete(user: ReturnType<typeof userEvent.setup>) {
    await user.click(await screen.findByRole("button", { name: ADD }));
    await user.type(screen.getByLabelText("Value"), "ann");
  }

  it("never asks about a half-built condition", async () => {
    guarded();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: ADD }));
    window.location.hash = "#/home";

    expect(await screen.findByText("Arrived at #/home")).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
  });

  it("asks before a complete filter nobody kept is left behind", async () => {
    guarded();
    const user = userEvent.setup();
    await complete(user);
    window.location.hash = "#/home";

    expect(await screen.findByRole("dialog", { name: ASKS })).toBeTruthy();
    expect(screen.getByDisplayValue("ann")).toBeTruthy();
  });

  it("goes to the view it just saved without asking", async () => {
    guarded();
    const user = userEvent.setup();
    await complete(user);
    await user.click(await screen.findByRole("button", { name: SAVE }));
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Anns");
    await user.click(screen.getByRole("button", { name: "Save view" }));

    expect(
      await screen.findByText("Arrived at #/filters/contacts/v-new"),
    ).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
  });

  it("goes to the Live List it just created without asking", async () => {
    guarded(true);
    const user = userEvent.setup();
    await complete(user);
    await user.click(await screen.findByRole("button", { name: SAVE }));
    await user.click(screen.getByRole("radio", { name: /^Live List/ }));
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Anns");
    await user.click(
      screen.getByRole("button", { name: en["filters.saveListConfirm"] }),
    );

    expect(await screen.findByText("Arrived at #/lists/new-list")).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
  });

  it("asks once when switching the record type clears a complete filter", async () => {
    guarded();
    const user = userEvent.setup();
    await complete(user);
    await user.click(screen.getByRole("button", { name: "Companies" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "Switch and clear" }));

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "New company filter",
      }),
    ).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
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
