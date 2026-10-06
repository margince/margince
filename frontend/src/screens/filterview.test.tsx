/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import type { FilterVocabulary } from "./filterdata";
import { FiltersScreen } from "./filters";
import {
  type FiltersServer,
  firstRowField,
  GuardedFilters,
  mountFilters,
  type Sent,
} from "./filters.testkit";

// `#/filters/<tab>/<viewId>`: a saved view opened to be read, edited in place
// and saved back over itself, held to the version it was read at.

const CONTACT_VOCAB: FilterVocabulary = {
  resource: "contact",
  fields: [
    {
      name: "city",
      type: "text",
      operators: ["eq", "neq", "contains"],
      custom: false,
    },
  ],
};

const BERLIN = { and: [{ field: "city", op: "eq", value: "Berlin" }] };

/** A stored view with a key beside its filter that a save must keep. */
const VIEW = {
  id: "v1",
  owner_id: "u-1",
  resource: "contacts",
  name: "Berlin contacts",
  shared_scope: "private",
  query: { filter: BERLIN, columns: ["full_name"] },
  version: 3,
};

const ASKS = en["unsaved.title"];
const EDIT = en["filters.editConditions"];
const MORE = "More for Berlin contacts";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

/** An opened view's address draws the page; every other is somewhere else. */
const viewAt = (address: string) => {
  const at = /^#\/filters\/(\w+)\/([\w-]+)$/.exec(address);
  return at ? <FiltersScreen key={address} id={at[1]} view={at[2]} /> : null;
};

/**
 * `cached` is a copy of v1 the client kept from an earlier visit, marked to be
 * read again as a write elsewhere leaves it.
 */
function open(server: FiltersServer = {}, cached?: object) {
  window.location.hash = "#/filters/contacts/v1";
  const mounted = mountFilters({
    views: [VIEW],
    preview: { match_count: 4 },
    vocabularies: { contact: CONTACT_VOCAB },
    ...server,
  });
  if (cached !== undefined) {
    const key = ["views", "one", "v1"];
    mounted.client.setQueryData(key, cached);
    mounted.client.getQueryCache().find({ queryKey: key })?.invalidate();
  }
  render(<GuardedFilters page={viewAt} />, { wrapper: mounted.wrapper });
  return mounted;
}

type User = ReturnType<typeof userEvent.setup>;

/** Edit conditions if the rows are folded, then the one value changed. */
async function changeCity(user: User, city: string) {
  if (screen.queryByLabelText("Value") === null) {
    await user.click(await screen.findByRole("button", { name: EDIT }));
  }
  const value = screen.getByLabelText("Value");
  await user.clear(value);
  await user.type(value, city);
}

const patches = (written: readonly Sent[]) =>
  written.filter((sent) => sent.method === "PATCH");

const saveChanges = (user: User) =>
  user.click(screen.getByRole("button", { name: en["filters.saveChanges"] }));

/** A promise a case resolves when it is done looking at the wait. */
function held() {
  let release = () => {};
  const answered = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { answered, release };
}

const HAMBURG_PROPOSED = {
  resource: "contact",
  filter: { field: "city", op: "eq", value: "Hamburg" },
  unsupported: [],
};

describe("opening a saved view", () => {
  it.each([
    ["the view", "viewsAnswered"],
    ["the fields", "vocabularyAnswered"],
  ] as const)(
    "waits under its fallback name while %s are read",
    async (_, held) => {
      open({ [held]: new Promise<void>(() => undefined) });

      expect(
        await screen.findByRole("heading", { level: 1, name: "Saved view" }),
      ).toBeInTheDocument();
      expect(screen.getByText(en["common.loading"])).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: en["filters.addClause"] }),
      ).toBeNull();
      expect(document.querySelector(".filter-clause")).toBeNull();
    },
  );

  it("reads first: its name, what it is, the filter as a sentence and what it selects", async () => {
    open();

    expect(
      await screen.findByRole("heading", { level: 1, name: "Berlin contacts" }),
    ).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(
      screen.getByText("Saved view · Contacts · Only me"),
    ).toBeInTheDocument();
    expect(screen.getByText("City is Berlin")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: en["filters.resultsTitle"] }),
    ).toBeInTheDocument();
    expect(await screen.findByText("4 contacts match")).toBeInTheDocument();
    // No rows to edit, and nothing on the page asks to be kept.
    expect(document.querySelector(".filter-clause")).toBeNull();
    expect(document.querySelectorAll(".btn-primary")).toHaveLength(0);
  });

  it.each([
    ["is not there", { views: [] }],
    [
      "was deleted",
      { views: [{ ...VIEW, archived_at: "2026-09-01T00:00:00Z" }] },
    ],
    [
      "holds no filter",
      { views: [{ ...VIEW, query: { columns: ["full_name"] } }] },
    ],
    ["is over projects", { views: [{ ...VIEW, resource: "projects" }] }],
  ])("says so, with the way back, when the view %s", async (_, server) => {
    open(server);
    const user = userEvent.setup();

    expect(
      await screen.findByText(en["filters.view.gone"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 1, name: "Saved view" }),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: en["filters.backToLibrary"] }),
    );
    expect(await screen.findByText("Arrived at #/filters")).toBeInTheDocument();
  });

  it("says why a read failed and reads again, rather than call the view gone", async () => {
    const { seen } = open({ viewsFail: true });
    const user = userEvent.setup();
    const reads = () =>
      seen.filter((sent) => sent.url.endsWith("/views/v1")).length;

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 1, name: "Saved view" }),
    ).toBeInTheDocument();
    expect(screen.queryByText(en["filters.view.gone"])).toBeNull();
    const before = reads();
    await user.click(
      screen.getByRole("button", { name: en["filters.view.reload"] }),
    );
    await vi.waitFor(() => expect(reads()).toBe(before + 1));
  });

  it("opens on what the server holds now, not a copy kept from before a rename elsewhere", async () => {
    const { written } = open(
      { views: [{ ...VIEW, name: "Berliners", version: 4 }] },
      VIEW,
    );
    const user = userEvent.setup();

    expect(
      await screen.findByRole("heading", { level: 1, name: "Berliners" }),
    ).toBeInTheDocument();
    await changeCity(user, "Hamburg");
    await saveChanges(user);
    expect(
      await screen.findByText(en["filters.changesSaved"]),
    ).toBeInTheDocument();
    expect(patches(written).map((sent) => sent.ifMatch)).toEqual(["4"]);
  });

  it("finds a view deleted elsewhere gone, though a copy of it was kept", async () => {
    open({ views: [] }, VIEW);

    expect(
      await screen.findByText(en["filters.view.gone"]),
    ).toBeInTheDocument();
  });

  it("puts the address right when it names another record type, leaving no entry for Back", async () => {
    open({
      views: [{ ...VIEW, resource: "companies" }],
      vocabularies: {
        contact: CONTACT_VOCAB,
        company: { resource: "company", fields: CONTACT_VOCAB.fields },
      },
    });
    const entries = window.history.length;

    expect(
      await screen.findByRole("heading", { level: 1, name: "Berlin contacts" }),
    ).toBeInTheDocument();
    expect(window.location.hash).toBe("#/filters/companies/v1");
    expect(window.history.length).toBe(entries);
    expect(
      screen.getByText("Saved view · Companies · Only me"),
    ).toBeInTheDocument();
  });
});

describe("editing in place", () => {
  it("opens the rows where the sentence was, and folds them back", async () => {
    open();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: EDIT }));

    expect(screen.getByDisplayValue("Berlin")).toBeInTheDocument();
    expect(firstRowField()).toHaveFocus();
    expect(screen.getByText(en["filters.noChanges"])).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: en["filters.done"] }));

    expect(screen.getByText("City is Berlin")).toBeInTheDocument();
    expect(document.querySelector(".filter-clause")).toBeNull();
    expect(screen.getByRole("button", { name: EDIT })).toHaveFocus();
  });

  it("reads a stored single clause as unchanged", async () => {
    open({ views: [{ ...VIEW, query: { filter: BERLIN.and[0] } }] });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: EDIT }));

    expect(screen.getByText(en["filters.noChanges"])).toBeInTheDocument();
  });

  it("discards a change and a proposal with it, and folds", async () => {
    open({ proposals: [{ body: HAMBURG_PROPOSED }] });
    const user = userEvent.setup();
    await changeCity(user, "Bonn");
    expect(screen.getByText(en["filters.unsavedChanges"])).toBeInTheDocument();
    await user.click(screen.getByText(en["filters.describeChanges"]));
    await user.type(
      screen.getByLabelText("Describe the contacts you want"),
      "in Hamburg",
    );
    await user.click(
      screen.getByRole("button", { name: "Propose conditions" }),
    );
    expect(
      await screen.findByText(/^Margince proposed 1 condition/),
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: en["filters.discardChanges"] }),
    );
    expect(screen.getByText("City is Berlin")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: EDIT }));
    expect(screen.queryByText(/^Margince proposed/)).toBeNull();
    expect(screen.getByText(en["filters.noChanges"])).toBeInTheDocument();
  });

  it("shows the rows again when a proposal lands after they were folded", async () => {
    const proposal = held();
    open({
      proposals: [{ body: HAMBURG_PROPOSED, answered: proposal.answered }],
    });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: EDIT }));
    await user.click(screen.getByText(en["filters.describeChanges"]));
    await user.type(
      screen.getByLabelText("Describe the contacts you want"),
      "in Hamburg",
    );
    await user.click(
      screen.getByRole("button", { name: "Propose conditions" }),
    );
    await user.click(screen.getByRole("button", { name: en["filters.done"] }));
    proposal.release();

    expect(
      await screen.findByText(/^Margince proposed 1 condition/),
    ).toBeInTheDocument();
    expect(screen.getByDisplayValue("Hamburg")).toBeInTheDocument();
  });
});

describe("saving back over the view", () => {
  it("sends the filter with the stored keys beside it, held to the version read at opening", async () => {
    const { written } = open();
    const user = userEvent.setup();
    await changeCity(user, "Hamburg");
    await saveChanges(user);

    expect(
      await screen.findByText(en["filters.changesSaved"]),
    ).toBeInTheDocument();
    expect(patches(written)).toEqual([
      {
        method: "PATCH",
        url: expect.stringContaining("/views/v1"),
        ifMatch: "3",
        body: {
          query: {
            columns: ["full_name"],
            filter: { and: [{ field: "city", op: "eq", value: "Hamburg" }] },
          },
        },
      },
    ]);
    expect(screen.getByText("City is Hamburg")).toBeInTheDocument();
    window.location.hash = "#/home";
    expect(await screen.findByText("Arrived at #/home")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
  });

  it("keeps the version it opened through a later read, meets a colleague's change with Reload, and saves on what it reloaded", async () => {
    const { written, client, changeView } = open();
    const user = userEvent.setup();
    await changeCity(user, "Hamburg");
    changeView("v1", { version: 5 });
    await act(() =>
      client.refetchQueries({ queryKey: ["views", "one", "v1"] }),
    );
    expect(screen.getByDisplayValue("Hamburg")).toBeInTheDocument();
    await saveChanges(user);

    expect(
      await screen.findByText(en["filters.view.conflict"]),
    ).toBeInTheDocument();
    expect(patches(written).map((sent) => sent.ifMatch)).toEqual(["3"]);
    await user.click(
      screen.getByRole("button", { name: en["filters.view.reload"] }),
    );
    expect(
      await screen.findByText(en["filters.noChanges"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(en["filters.view.conflict"])).toBeNull();
    expect(firstRowField()).toHaveFocus();

    await changeCity(user, "Bonn");
    await saveChanges(user);
    expect(
      await screen.findByText(en["filters.changesSaved"]),
    ).toBeInTheDocument();
    expect(patches(written).map((sent) => sent.ifMatch)).toEqual(["3", "5"]);
  });

  it("saves after a rename on the version the rename answered", async () => {
    const { written } = open();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: MORE }));
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    const name = screen.getByRole("textbox", { name: en["views.name"] });
    expect(name).toHaveValue("Berlin contacts");
    await user.clear(name);
    await user.type(name, "Berliners");
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["views.rename"],
      }),
    );
    expect(
      await screen.findByRole("heading", { level: 1, name: "Berliners" }),
    ).toBeInTheDocument();

    await changeCity(user, "Hamburg");
    await saveChanges(user);
    expect(
      await screen.findByText(en["filters.changesSaved"]),
    ).toBeInTheDocument();
    expect(
      patches(written).map(({ ifMatch, body }) => ({ ifMatch, body })),
    ).toEqual([
      { ifMatch: "3", body: { name: "Berliners" } },
      {
        ifMatch: "4",
        body: expect.objectContaining({ query: expect.anything() }),
      },
    ]);
  });

  it("says the view is gone when the save finds it deleted", async () => {
    const { changeView } = open();
    const user = userEvent.setup();
    await changeCity(user, "Hamburg");
    changeView("v1", { archived_at: "2026-09-01T00:00:00Z" });
    await saveChanges(user);

    expect(
      await screen.findByText(en["filters.view.gone"]),
    ).toBeInTheDocument();
  });

  it("holds the rows and every other way out until the save answers", async () => {
    const patch = held();
    open({ viewPatched: patch.answered });
    const user = userEvent.setup();
    await changeCity(user, "Hamburg");
    await saveChanges(user);

    expect(
      screen.getByRole("button", { name: en["filters.discardChanges"] }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: en["filters.saveAsNew"] }),
    ).toBeDisabled();
    expect(
      document.querySelector(".filter-clause")?.closest("[inert]"),
    ).not.toBeNull();
    patch.release();

    expect(
      await screen.findByText(en["filters.changesSaved"]),
    ).toBeInTheDocument();
    expect(screen.getByText("City is Hamburg")).toBeInTheDocument();
  });

  it("leaves a proposal that landed while the save was out open, and unsaved", async () => {
    const proposal = held();
    const patch = held();
    const { written } = open({
      proposals: [{ body: HAMBURG_PROPOSED, answered: proposal.answered }],
      viewPatched: patch.answered,
    });
    const user = userEvent.setup();
    await changeCity(user, "Bonn");
    await user.click(screen.getByText(en["filters.describeChanges"]));
    await user.type(
      screen.getByLabelText("Describe the contacts you want"),
      "in Hamburg",
    );
    await user.click(
      screen.getByRole("button", { name: "Propose conditions" }),
    );
    await saveChanges(user);
    proposal.release();
    expect(
      await screen.findByText(/^Margince proposed 1 condition/),
    ).toBeInTheDocument();
    patch.release();

    expect(
      await screen.findByText(en["filters.changesSaved"]),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/^Margince proposed 1 condition/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: EDIT })).toBeNull();
    expect(patches(written).map((sent) => sent.body)).toEqual([
      {
        query: {
          columns: ["full_name"],
          filter: { and: [{ field: "city", op: "eq", value: "Bonn" }] },
        },
      },
    ]);
  });
});

describe("keeping it some other way", () => {
  it("saves the changed filter as a new view and opens it", async () => {
    const { written } = open();
    const user = userEvent.setup();
    await changeCity(user, "Hamburg");
    await user.click(
      screen.getByRole("button", { name: en["filters.saveAsNew"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["views.name"] }),
      "Hamburg contacts",
    );
    await user.click(screen.getByRole("button", { name: en["views.save"] }));

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "Hamburg contacts",
      }),
    ).toBeInTheDocument();
    expect(window.location.hash).toBe("#/filters/contacts/v-new");
    expect(written.find((sent) => sent.method === "POST")?.body).toEqual({
      resource: "contacts",
      name: "Hamburg contacts",
      query: {
        filter: { and: [{ field: "city", op: "eq", value: "Hamburg" }] },
      },
    });
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
  });

  it.each([
    [true, 1],
    [false, 0],
  ])(
    "offers Save as Live List from More only while lists are on (%s)",
    async (listsOn, offered) => {
      open({ listsOn });
      const user = userEvent.setup();
      await user.click(await screen.findByRole("button", { name: MORE }));
      const item = screen.queryAllByRole("button", {
        name: en["filters.view.saveAsList"],
      });
      expect(item).toHaveLength(offered);
      if (!listsOn) {
        return;
      }
      await user.click(item[0]);
      const dialog = screen.getByRole("dialog", {
        name: en["filters.saveTitle"],
      });
      expect(
        within(dialog).getByRole("radio", { name: /^Live List/ }),
      ).toBeChecked();
      expect(
        within(dialog).getByRole("textbox", { name: en["views.name"] }),
      ).toHaveValue("Berlin contacts");
    },
  );

  it("exports what it selects from More", async () => {
    Object.defineProperties(URL, {
      createObjectURL: { configurable: true, value: vi.fn(() => "blob:test") },
      revokeObjectURL: { configurable: true, value: vi.fn() },
    });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
      () => undefined,
    );
    const { written } = open();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: MORE }));
    await user.click(
      screen.getByRole("button", { name: en["filters.exportCsv"] }),
    );

    await vi.waitFor(() =>
      expect(
        written.find((sent) => sent.url.includes("/exports"))?.body,
      ).toEqual({
        object: "contact",
        filter: BERLIN,
        format: "csv",
      }),
    );
  });

  it("deletes after asking, and leaves for the library without the guard asking", async () => {
    const { written } = open();
    const user = userEvent.setup();
    await changeCity(user, "Hamburg");
    await user.click(screen.getByRole("button", { name: MORE }));
    await user.click(
      screen.getByRole("button", { name: en["views.deleteConfirm"] }),
    );
    const dialog = screen.getByRole("dialog", {
      name: en["views.deleteTitle"],
    });
    await user.click(
      within(dialog).getByRole("button", { name: en["views.deleteConfirm"] }),
    );

    expect(await screen.findByText("Arrived at #/filters")).toBeInTheDocument();
    expect(
      screen.getByText("View deleted: “Berlin contacts”"),
    ).toBeInTheDocument();
    expect(written.some((sent) => sent.method === "DELETE")).toBe(true);
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
  });
});

describe("the unsaved guard", () => {
  it("does not ask about a view only opened, or opened for editing", async () => {
    open();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: EDIT }));
    window.location.hash = "#/home";

    expect(await screen.findByText("Arrived at #/home")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
  });

  it("asks once something changed, and not after the change is discarded", async () => {
    open();
    const user = userEvent.setup();
    await changeCity(user, "Hamburg");
    window.location.hash = "#/home";
    expect(
      await screen.findByRole("dialog", { name: ASKS }),
    ).toBeInTheDocument();
    // Closing the question keeps the page and the change on it.
    await user.keyboard("{Escape}");

    await user.click(
      screen.getByRole("button", { name: en["filters.discardChanges"] }),
    );
    window.location.hash = "#/home";
    expect(await screen.findByText("Arrived at #/home")).toBeInTheDocument();
  });
});
