/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { stable } from "../../format/collate";
import { en } from "../../i18n/en";
import type { List } from "../../lib/lists.queries";
import { FOLD_MAX_WIDTH, widthMedia } from "../../lib/viewport";
import {
  LIVE_ID,
  liveList,
  OTHER_OWNER_ID,
  READER_ID,
  shortlist,
  TEAM_ID,
} from "../../testing/lists.fixtures";
import { FiltersScreen } from "./filters";
import { filterView, mountFilters, type Sent } from "./filters.testkit";

// One row of the library: what it names, what it counts, where it opens and
// what its ⋯ offers. A press on ⋯ or inside a dialog it opened never opens
// the row.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const BERLIN = filterView("v1", "Berlin contacts", "contacts", {
  and: [{ field: "city", op: "eq", value: "Berlin" }],
});

/** The confirmations a write shows, drawn where a test can read them. */
const withToasts = (page: ReactNode) => (
  <ToastProvider>
    {page}
    <ToastRegion />
  </ToastProvider>
);

const rowOf = async (name: string) =>
  (await screen.findByText(name)).closest("tr") as HTMLElement;

const vocabularyReads = (seen: readonly Sent[]) =>
  seen
    .map((sent) => new URL(sent.url, "https://x.local"))
    .filter((url) => url.pathname.endsWith("/filters/vocabulary"))
    .map((url) => url.searchParams.get("resource"));

describe("a row", () => {
  it("opens through a real link: a view on its filter page, a list on its own page", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN],
      lists: [liveList],
    });
    render(<FiltersScreen />, { wrapper });
    const view = await rowOf(BERLIN.name);
    expect(
      within(view).getByRole("link", { name: BERLIN.name }),
    ).toHaveAttribute("href", "/#/filters/contacts/v1");
    const list = await rowOf(liveList.name);
    expect(
      within(list).getByRole("link", { name: liveList.name }),
    ).toHaveAttribute("href", `/#/lists/${LIVE_ID}`);
  });

  it("says a view's kind in a neutral badge, marks a Live List alone as live, and counts only a list's records", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN],
      lists: [liveList, shortlist],
    });
    render(<FiltersScreen />, { wrapper });
    const view = await rowOf(BERLIN.name);
    expect(
      within(view).getByText(en["filters.library.kindView"]).closest(".badge")
        ?.className,
    ).toBe("badge");
    // The type alone: a saved view is counted when it is opened, never here.
    expect(view.querySelector(".datatable-end")?.textContent).toBe(
      en["lists.type.contact"],
    );
    const list = await rowOf(liveList.name);
    // Info, not the accent or indigo: those say the one primary action and an
    // agent's proposal. A list that keeps itself current is neither.
    const live = within(list)
      .getByText(en["lists.kind.live"])
      .closest(".badge");
    expect(live?.className).toBe("badge badge-info");
    expect(live?.querySelector(".badge-live-dot")).not.toBeNull();
    const chosen = within(await rowOf(shortlist.name))
      .getByText(en["lists.kind.shortlist"])
      .closest(".badge");
    expect(chosen?.className).toBe("badge");
    expect(chosen?.querySelector(".badge-live-dot")).toBeNull();
    expect(within(list).getByText("42 companies")).toBeInTheDocument();
    expect(
      within(list).getByText("42 companies you can see"),
    ).toBeInTheDocument();
  });

  it("shows what a Live List gained and lost since the last visit in a neutral badge, and nothing on a Shortlist", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      lists: [
        liveList,
        {
          ...shortlist,
          since_last_visit: {
            since: "2026-09-28T17:00:00Z",
            entered: 2,
            left: 0,
          },
        },
      ],
    });
    render(<FiltersScreen />, { wrapper });
    const live = await rowOf(liveList.name);
    const pulse = within(live).getByText(
      "3 joined and 1 left since your last visit",
    );
    expect(pulse.closest(".badge")?.className).toBe("badge");
    const chosen = await rowOf(shortlist.name);
    expect(within(chosen).queryByText(/since your last visit/)).toBeNull();
  });

  it("says only the type of a Live List the server could not count, and still shows what it gained and lost", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      lists: [{ ...liveList, health: "invalid", visible_count: null }],
    });
    render(<FiltersScreen />, { wrapper });
    const live = await rowOf(liveList.name);
    expect(
      within(live).getByText(en["lists.type.company"]),
    ).toBeInTheDocument();
    expect(within(live).queryByText(/you can see$/)).toBeNull();
    expect(
      within(live).getByText("3 joined and 1 left since your last visit"),
    ).toBeInTheDocument();
  });

  it("says who can find each list in either group: a named team, the owner's teams, everyone, its owner, its steward or both", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      teams: { [TEAM_ID]: "Team Germany" },
      lists: [
        { ...liveList, id: "named", name: "Named", team_id: TEAM_ID },
        { ...liveList, id: "ours", name: "Our accounts" },
        { ...liveList, id: "theirs", name: "Theirs", owner_id: OTHER_OWNER_ID },
        { ...shortlist, id: "all", name: "Launch deck" },
        {
          ...shortlist,
          id: "kept",
          name: "Kept to myself",
          sharing: "private",
          owner_id: READER_ID,
        },
        {
          ...shortlist,
          id: "stewarded",
          name: "Board introductions",
          sharing: "private",
          owner_id: OTHER_OWNER_ID,
          steward_id: READER_ID,
        },
        {
          ...shortlist,
          id: "ownerless",
          name: "Left behind",
          sharing: "private",
          owner_id: null,
          steward_id: READER_ID,
        },
        {
          ...shortlist,
          id: "cared",
          name: "Cared for together",
          sharing: "private",
          owner_id: READER_ID,
          steward_id: OTHER_OWNER_ID,
        },
        {
          ...shortlist,
          id: "theirs-alone",
          name: "Their own picks",
          sharing: "private",
          owner_id: OTHER_OWNER_ID,
          steward_id: OTHER_OWNER_ID,
        },
      ],
    });
    render(<FiltersScreen />, { wrapper });
    // One read draws every row, so once the team is named all rows are there.
    const said = (name: string) =>
      screen.getByText(name).closest("tr")?.textContent ?? "";
    await vi.waitFor(() => expect(said("Named")).toContain("Team Germany"));
    expect(said("Kept to myself")).toContain(en["lists.sharing.private"]);
    const stewarded = said("Board introductions");
    expect(stewarded).toContain(en["lists.audience.ownerAndSteward"]);
    expect(stewarded).not.toContain(en["lists.sharing.private"]);
    expect(said("Their own picks")).toContain(en["lists.audience.ownerOnly"]);
    expect(said("Cared for together")).toContain(
      en["lists.audience.youAndSteward"],
    );
    const ownerless = said("Left behind");
    expect(ownerless).toContain(en["lists.audience.stewardOnly"]);
    expect(ownerless).not.toContain(en["lists.audience.ownerOnly"]);
    expect(said("Our accounts")).toContain(en["lists.audience.yourTeams"]);
    expect(said("Theirs")).toContain(en["lists.audience.ownerTeams"]);
    expect(said("Launch deck")).toContain(en["lists.sharing.workspace"]);
  });

  it("asks who can find a row only in a group that holds a list", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN],
      lists: [{ ...liveList, owner_id: OTHER_OWNER_ID }],
    });
    render(<FiltersScreen />, { wrapper });
    await screen.findByText(liveList.name);
    const who = { name: en["lists.sharingLabel"] };
    const mine = screen.getByRole("region", {
      name: en["filters.library.mine"],
    });
    const shared = screen.getByRole("region", {
      name: en["filters.library.shared"],
    });
    expect(within(mine).getByText(BERLIN.name)).toBeInTheDocument();
    expect(within(mine).queryByRole("columnheader", who)).toBeNull();
    expect(within(shared).getByRole("columnheader", who)).toBeInTheDocument();
  });

  it("says, folded, who can find a list the reader shares from their own group", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === widthMedia(FOLD_MAX_WIDTH),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    const { wrapper } = mountFilters({ listsOn: true, lists: [liveList] });
    render(<FiltersScreen />, { wrapper });
    const mine = await screen.findByRole("region", {
      name: en["filters.library.mine"],
    });
    const facts = (await within(mine).findByText(liveList.name))
      .closest(".library-name-cell")
      ?.querySelector(".library-facts");
    expect(facts?.textContent).toContain(en["lists.audience.yourTeams"]);
    expect(
      within(mine).queryByRole("columnheader", {
        name: en["lists.sharingLabel"],
      }),
    ).toBeNull();
  });

  it("says a Live List uses a retired field", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      lists: [
        {
          ...liveList,
          health: "retired_field",
          retired_fields: ["cf_last_touch"],
        },
      ],
    });
    render(<FiltersScreen />, { wrapper });
    expect(
      within(await rowOf(liveList.name)).getByText(
        en["lists.health.retiredField"],
      ),
    ).toBeInTheDocument();
  });

  it("counts a filter's conditions until the vocabulary names them, and reads only the vocabularies it needs", async () => {
    let answer = () => {};
    const builds: List = {
      ...liveList,
      id: "L-builds",
      name: "Open builds",
      entity_type: "project",
      purpose: "",
      definition: { and: [{ field: "status", op: "eq", value: "active" }] },
    };
    const { seen, wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN],
      lists: [liveList, builds],
      vocabularyAnswered: new Promise((resolve) => {
        answer = resolve;
      }),
      vocabularies: {
        contact: {
          resource: "contact",
          fields: [
            { name: "city", type: "text", operators: ["eq"], custom: false },
          ],
        },
      },
    });
    render(<FiltersScreen />, { wrapper });
    const view = await rowOf(BERLIN.name);
    expect(within(view).getByText("1 condition")).toBeInTheDocument();
    answer();
    expect(await within(view).findByText("City is Berlin")).toBeInTheDocument();
    expect(
      within(await rowOf(liveList.name)).getByText(liveList.purpose ?? ""),
    ).toBeVisible();
    expect(
      vocabularyReads(seen).sort((a, b) => stable(String(a), String(b))),
    ).toEqual(["contact", "project"]);
  });
});

describe("a saved view's ⋯", () => {
  it("renames it in place against the version it was read at", async () => {
    const { written, wrapper } = mountFilters({ views: [BERLIN] });
    const user = userEvent.setup();
    render(withToasts(<FiltersScreen />), { wrapper });
    await user.click(
      within(await rowOf(BERLIN.name)).getByRole("button", {
        name: "More actions for Berlin contacts",
      }),
    );
    await user.click(screen.getByRole("button", { name: en["views.rename"] }));
    const field = screen.getByRole("textbox", { name: en["views.name"] });
    expect(field).toHaveValue(BERLIN.name);
    await user.clear(field);
    await user.type(field, "Berlin and Potsdam");
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["views.rename"],
      }),
    );
    expect(await screen.findByText(en["views.renamed"])).toBeInTheDocument();
    expect(await screen.findByText("Berlin and Potsdam")).toBeInTheDocument();
    const patch = written.find((sent) => sent.method === "PATCH");
    expect(patch?.url).toContain("/views/v1");
    expect(patch?.ifMatch).toBe("1");
    expect(patch?.body).toEqual({ name: "Berlin and Potsdam" });
    expect(window.location.hash).toBe("");
  });

  it("deletes it after naming it, and stays on the library", async () => {
    const { written, wrapper } = mountFilters({ views: [BERLIN] });
    const user = userEvent.setup();
    render(withToasts(<FiltersScreen />), { wrapper });
    await user.click(
      within(await rowOf(BERLIN.name)).getByRole("button", {
        name: "More actions for Berlin contacts",
      }),
    );
    await user.click(
      screen.getByRole("button", { name: en["views.deleteConfirm"] }),
    );
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByText(
        "“Berlin contacts” leaves Filters and views. No records change.",
      ),
    ).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole("button", { name: en["views.deleteConfirm"] }),
    );
    expect(
      await screen.findByText("View deleted: “Berlin contacts”"),
    ).toBeInTheDocument();
    await vi.waitFor(() => expect(screen.queryByRole("link")).toBeNull());
    expect(written.map((sent) => sent.method)).toEqual(["DELETE"]);
    expect(window.location.hash).toBe("");
    const firstRun = screen.getByText(en["filters.library.viewsEmptyTitle"]);
    await vi.waitFor(() =>
      expect(firstRun.closest(".library-anchor")).toHaveFocus(),
    );
  });

  it("hands focus to the saved-views group, which outlives the deleted row", async () => {
    const potsdam = filterView("v2", "Potsdam contacts", "contacts", {
      and: [{ field: "city", op: "eq", value: "Potsdam" }],
    });
    const { wrapper } = mountFilters({ views: [BERLIN, potsdam] });
    const user = userEvent.setup();
    render(withToasts(<FiltersScreen />), { wrapper });
    await user.click(
      within(await rowOf(BERLIN.name)).getByRole("button", {
        name: "More actions for Berlin contacts",
      }),
    );
    await user.click(
      screen.getByRole("button", { name: en["views.deleteConfirm"] }),
    );
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["views.deleteConfirm"],
      }),
    );
    await vi.waitFor(() =>
      expect(screen.queryByRole("link", { name: BERLIN.name })).toBeNull(),
    );
    const group = screen.getByRole("region", {
      name: en["filters.library.views"],
    });
    await vi.waitFor(() => expect(group.parentElement).toHaveFocus());
  });
});

describe("a list's ⋯", () => {
  it("edits a Live List's filter for whoever may, and offers a Shortlist nothing", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      lists: [liveList, shortlist],
    });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    const chosen = await rowOf(shortlist.name);
    expect(
      within(chosen).queryByRole("button", { name: /^More actions for/ }),
    ).toBeNull();
    await user.click(
      within(await rowOf(liveList.name)).getByRole("button", {
        name: `More actions for ${liveList.name}`,
      }),
    );
    await user.click(
      screen.getByRole("button", { name: en["lists.editFilter"] }),
    );
    expect(window.location.hash).toBe(`#/filters/list/${LIVE_ID}`);
  });
});

describe("New Shortlist", () => {
  it("starts a Shortlist of the chosen record type and opens it", async () => {
    const { written, wrapper } = mountFilters({
      listsOn: true,
      lists: [liveList],
    });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    await user.click(
      await screen.findByRole("button", { name: en["lists.newShortlist"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.name"] }),
      "Dinner",
    );
    await user.click(
      screen.getByRole("combobox", { name: en["lists.recordTypeLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: en["lists.type.company"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.purpose"] }),
      "October",
    );
    await user.click(screen.getByRole("button", { name: en["lists.create"] }));
    await vi.waitFor(() =>
      expect(window.location.hash).toBe("#/lists/new-list"),
    );
    expect(written[0]?.body).toEqual({
      name: "Dinner",
      entity_type: "company",
      list_type: "static",
      purpose: "October",
      sharing: "team",
    });
  });

  it("starts a Shortlist only the reader can find", async () => {
    const { written, wrapper } = mountFilters({
      listsOn: true,
      lists: [liveList],
    });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    await user.click(
      await screen.findByRole("button", { name: en["lists.newShortlist"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.name"] }),
      "Dinner",
    );
    await user.click(
      screen.getByRole("combobox", { name: en["lists.sharingLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: en["lists.sharing.private"] }),
    );
    await user.click(screen.getByRole("button", { name: en["lists.create"] }));
    await vi.waitFor(() => expect(written).toHaveLength(1));
    expect(written[0]?.body).toMatchObject({ sharing: "private" });
  });
});
