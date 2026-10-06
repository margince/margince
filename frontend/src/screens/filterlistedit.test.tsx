// @vitest-environment happy-dom
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
import { ListScreen } from "./listpage";
import { LIVE_ID, listsMe, liveList } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// `#/filters/list/<id>`: a Live List's filter, opened straight into its rows
// and saved back to the list; and the list page's way there.

const COMPANY_VOCAB: FilterVocabulary = {
  resource: "company",
  fields: [
    { name: "industry", type: "text", operators: ["eq", "neq"], custom: false },
    {
      name: "cf_last_touch",
      type: "date",
      operators: ["lt", "gt"],
      custom: true,
    },
  ],
};

const saveTo = en["lists.saveFilterTo"].replace("{name}", liveList.name);
const ASKS = en["unsaved.title"];

/** The list's filter with its industry changed, as the PATCH carries it. */
const RETAIL = {
  and: [
    { field: "industry", op: "eq", value: "Retail" },
    { field: "cf_last_touch", op: "lt", value: { days_ago: 45 } },
  ],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

/** A list filter's address draws the page; every other is somewhere else. */
const listFilterAt = (address: string) => {
  const at = /^#\/filters\/list\/([\w-]+)$/.exec(address);
  return at ? <FiltersScreen key={address} id="list" view={at[1]} /> : null;
};

/**
 * `cached` is a copy of the list the client kept from an earlier visit, marked
 * to be read again as a write elsewhere leaves it.
 */
function open(server: FiltersServer = {}, cached?: object) {
  window.location.hash = `#/filters/list/${LIVE_ID}`;
  const mounted = mountFilters({
    listsOn: true,
    lists: [liveList],
    preview: { match_count: 42 },
    vocabularies: { company: COMPANY_VOCAB },
    ...server,
  });
  if (cached !== undefined) {
    const key = ["lists", "one", LIVE_ID];
    mounted.client.setQueryData(key, cached);
    mounted.client.getQueryCache().find({ queryKey: key })?.invalidate();
  }
  render(<GuardedFilters page={listFilterAt} />, { wrapper: mounted.wrapper });
  return mounted;
}

type User = ReturnType<typeof userEvent.setup>;

/** The list's industry, changed from Manufacturing to Retail. */
async function changeIndustry(user: User) {
  const industry = await screen.findByDisplayValue("Manufacturing");
  await user.clear(industry);
  await user.type(industry, "Retail");
}

const confirmSave = async (user: User) => {
  await user.click(screen.getByRole("button", { name: saveTo }));
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", { name: en["lists.saveFilterConfirm"] }),
  );
  return dialog;
};

const patches = (written: readonly Sent[]) =>
  written.filter((sent) => sent.method === "PATCH").map((sent) => sent.body);

describe("opening a Live List's filter", () => {
  it("says lists are off under its fallback name, and reads no list", async () => {
    const { seen } = open({ listsOn: false });

    expect(
      await screen.findByText(en["lists.unavailable"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 1, name: en["lists.editFilter"] }),
    ).toBeInTheDocument();
    expect(seen.some((sent) => sent.url.includes("/lists"))).toBe(false);
  });

  it("heads itself while the list is still being read", async () => {
    let answer = () => {};
    open({
      listAnswered: new Promise<void>((resolve) => {
        answer = resolve;
      }),
    });

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: en["lists.editFilter"],
      }),
    ).toBeInTheDocument();
    answer();
    expect(
      await screen.findByRole("heading", { level: 1, name: liveList.name }),
    ).toBeInTheDocument();
  });

  it("opens straight into the rows, naming the list a save would change", async () => {
    open();

    expect(
      await screen.findByDisplayValue("Manufacturing"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 1, name: liveList.name }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        en["lists.editingTitle"].replace("{name}", liveList.name),
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/^Live List · Companies · /)).toBeInTheDocument();
    expect(screen.getByText(en["filters.noChanges"])).toBeInTheDocument();
    expect(screen.getByRole("button", { name: saveTo })).toBeDisabled();
  });

  it("discards a change and keeps the reader's place in the rows", async () => {
    open();
    const user = userEvent.setup();
    await changeIndustry(user);
    await user.click(
      screen.getByRole("button", { name: en["filters.discardChanges"] }),
    );

    expect(screen.getByDisplayValue("Manufacturing")).toBeInTheDocument();
    expect(screen.getByText(en["filters.noChanges"])).toBeInTheDocument();
    expect(firstRowField()).toHaveFocus();
  });

  it.each([
    [
      "holds projects",
      { lists: [{ ...liveList, entity_type: "project" as const }] },
    ],
    ["is not there", { lists: [] }],
  ])(
    "says the filter cannot be opened here when the list %s",
    async (_, server) => {
      open(server);
      const user = userEvent.setup();

      expect(
        await screen.findByText(en["lists.filterCannotOpen"]),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("heading", { level: 1, name: en["lists.editFilter"] }),
      ).toBeInTheDocument();
      await user.click(
        screen.getByRole("button", { name: en["filters.backToLibrary"] }),
      );
      expect(
        await screen.findByText("Arrived at #/filters"),
      ).toBeInTheDocument();
    },
  );
});

describe("saving to the list", () => {
  it("names the automations watching the list, writes the version read at opening, and leaves for the list", async () => {
    const { written } = open({
      lists: [
        {
          ...liveList,
          dependencies: [
            {
              kind: "automation",
              occurred_at: "2026-09-02T00:00:00Z",
              blocking: false,
              role: "watches",
              automation_id: "01a0f000-0000-7000-8000-000000000041",
              automation_name: "Tell me about new buyers",
            },
          ],
        },
      ],
    });
    const user = userEvent.setup();
    await changeIndustry(user);
    await user.click(screen.getByRole("button", { name: saveTo }));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(en["lists.rules.settingsLeadLive"]),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        en["lists.rules.watches"].replace("{name}", "Tell me about new buyers"),
      ),
    ).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole("button", {
        name: en["lists.saveFilterConfirm"],
      }),
    );

    expect(
      await screen.findByText(`Arrived at #/lists/${LIVE_ID}`),
    ).toBeInTheDocument();
    expect(screen.getByText(`Saved to “${liveList.name}”`)).toBeInTheDocument();
    expect(patches(written)).toEqual([
      { version: liveList.version, definition: RETAIL },
    ]);
    expect(screen.queryByRole("dialog", { name: ASKS })).toBeNull();
  });

  it("keeps the version it opened through a later read, and says plainly when somebody changed the list since", async () => {
    const { written, client, changeList } = open();
    const user = userEvent.setup();
    await changeIndustry(user);
    changeList(LIVE_ID, { version: liveList.version + 1 });
    await act(() =>
      client.refetchQueries({ queryKey: ["lists", "one", LIVE_ID] }),
    );
    const dialog = await confirmSave(user);

    expect(
      await within(dialog).findByText(en["lists.saveFilterConflict"]),
    ).toBeInTheDocument();
    expect(patches(written)).toEqual([
      { version: liveList.version, definition: RETAIL },
    ]);
  });

  it("writes the version the server holds now, not that of a copy kept from before", async () => {
    const renamed = {
      ...liveList,
      name: "Renamed buyers",
      version: liveList.version + 1,
    };
    const { written } = open({ lists: [renamed] }, liveList);
    const user = userEvent.setup();

    expect(
      await screen.findByRole("heading", { level: 1, name: renamed.name }),
    ).toBeInTheDocument();
    await changeIndustry(user);
    await user.click(
      screen.getByRole("button", {
        name: en["lists.saveFilterTo"].replace("{name}", renamed.name),
      }),
    );
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: en["lists.saveFilterConfirm"],
      }),
    );

    expect(
      await screen.findByText(`Arrived at #/lists/${LIVE_ID}`),
    ).toBeInTheDocument();
    expect(patches(written)).toEqual([
      { version: renamed.version, definition: RETAIL },
    ]);
  });

  it("writes the filter the reader confirmed, not an answer that lands behind the dialog", async () => {
    let answer = () => {};
    const { written } = open({
      proposals: [
        {
          body: {
            resource: "company",
            filter: { field: "industry", op: "neq", value: "Mining" },
            unsupported: [],
          },
          answered: new Promise<void>((resolve) => {
            answer = resolve;
          }),
        },
      ],
    });
    const user = userEvent.setup();
    await changeIndustry(user);
    await user.click(screen.getByText(en["filters.describeChanges"]));
    await user.type(
      screen.getByLabelText("Describe the companies you want"),
      "not mining",
    );
    await user.click(
      screen.getByRole("button", { name: "Propose conditions" }),
    );
    await user.click(screen.getByRole("button", { name: saveTo }));
    const dialog = await screen.findByRole("dialog");
    answer();
    expect(
      await screen.findByText(/^Margince proposed 1 condition/),
    ).toBeInTheDocument();

    await user.click(
      within(dialog).getByRole("button", {
        name: en["lists.saveFilterConfirm"],
      }),
    );
    await screen.findByText(`Arrived at #/lists/${LIVE_ID}`);
    expect(patches(written)).toEqual([
      { version: liveList.version, definition: RETAIL },
    ]);
  });

  it("offers saving as a new view and the exports from More", async () => {
    open();
    const user = userEvent.setup();
    await screen.findByDisplayValue("Manufacturing");
    await user.click(
      screen.getByRole("button", { name: en["filters.footMore"] }),
    );
    expect(
      screen.getByRole("button", { name: en["filters.exportCsv"] }),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: en["filters.saveAsNew"] }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: en["filters.saveTitle"],
    });
    expect(
      within(dialog).getByRole("radio", { name: /^Saved view/ }),
    ).toBeChecked();
  });

  it("opens a list the reader may not change with a new filter's Save", async () => {
    const { written } = open({ lists: [{ ...liveList, can_edit: false }] });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["filters.save"] }),
    );
    expect(screen.queryByRole("button", { name: saveTo })).toBeNull();
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("radio", { name: /^Live List/ }));
    await user.type(
      within(dialog).getByRole("textbox", { name: en["views.name"] }),
      "My copy",
    );
    await user.click(
      within(dialog).getByRole("button", {
        name: en["filters.saveListConfirm"],
      }),
    );

    expect(
      await screen.findByText("Arrived at #/lists/new-list"),
    ).toBeInTheDocument();
    expect(written.find((sent) => sent.method === "POST")?.body).toMatchObject({
      name: "My copy",
      entity_type: "company",
      list_type: "dynamic",
      definition: liveList.definition,
    });
  });
});

describe("the Edit filter action on a list page", () => {
  function page(list = liveList) {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(list),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: [], page: { has_more: false } }),
    });
    return render(
      <StoryProviders>
        <ListScreen listID={LIVE_ID} />
      </StoryProviders>,
    );
  }

  it("opens the builder on this list for its steward", async () => {
    page();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["lists.editFilter"] }),
    );
    expect(window.location.hash).toBe(`#/filters/list/${LIVE_ID}`);
  });

  it("is offered from the retired-field notice too", async () => {
    page({ ...liveList, health: "retired_field", retired_fields: ["cf_x"] });
    await screen.findByText(en["lists.retiredField.title"]);
    expect(
      screen.getAllByRole("button", { name: en["lists.editFilter"] }),
    ).toHaveLength(2);
  });

  it("is not offered to a reader who may not change the list", async () => {
    page({ ...liveList, can_edit: false });
    await screen.findByText(liveList.name);
    expect(
      screen.queryByRole("button", { name: en["lists.editFilter"] }),
    ).toBeNull();
  });
});
