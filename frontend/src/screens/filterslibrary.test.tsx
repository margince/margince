/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { SEARCH_DEBOUNCE_MS } from "../design-system/debouncedsearch";
import { en } from "../i18n/en";
import { FiltersScreen } from "./filters";
import { filterView, mountFilters, type Sent } from "./filters.testkit";
import { liveList, shortlist } from "./lists.fixtures";
import type { List } from "./lists.queries";

// The page `#/filters` opens on: one library of every saved view and list,
// fed by one read of each, cut by what the address holds, and never a builder.

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const BERLIN = filterView("v1", "Berlin contacts", "contacts", {
  and: [{ field: "city", op: "eq", value: "Berlin" }],
});
const FLEET = filterView("v2", "Fleet companies", "companies", {
  and: [{ field: "industry", op: "eq", value: "automotive" }],
});
const DINNER: List = {
  ...shortlist,
  id: "L-dinner",
  name: "Dinner guests",
  sharing: "private",
};
const BUILDS: List = {
  ...liveList,
  id: "L-builds",
  name: "Open builds",
  entity_type: "project",
  purpose: "Projects still being delivered",
};

const reads = (seen: readonly Sent[], path: string) =>
  seen
    .filter((sent) => sent.method === "GET")
    .map((sent) => new URL(sent.url, "https://x.local"))
    .filter((url) => url.pathname.endsWith(path));

const region = (name: string) => screen.getByRole("region", { name });

describe("before the session answers", () => {
  it("shows one pending body, and neither layout", async () => {
    const { wrapper } = mountFilters({
      meAnswered: new Promise(() => {}),
      views: [BERLIN],
    });
    render(<FiltersScreen />, { wrapper });
    expect(
      await screen.findByText(en["filters.library.loading"]),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText(en["filters.library.firstRunTitle"])).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["filters.addClause"] }),
    ).toBeNull();
  });

  it("heads an opened list as the list page it is about to be", async () => {
    const { wrapper } = mountFilters({ meAnswered: new Promise(() => {}) });
    render(<FiltersScreen list={liveList.id} />, { wrapper });
    expect(
      await screen.findByRole("heading", { level: 1, name: en["lists.page"] }),
    ).toBeInTheDocument();
    expect(screen.getByText(en["lists.loading"])).toBeInTheDocument();
  });
});

describe("with lists on", () => {
  it("reads every view and every list once, and groups them by who can find them", async () => {
    const { seen, wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN, FLEET],
      lists: [DINNER, liveList, shortlist],
    });
    render(<FiltersScreen />, { wrapper });
    expect(await screen.findByText(liveList.name)).toBeInTheDocument();
    const lists = reads(seen, "/lists");
    expect(lists).toHaveLength(1);
    expect(lists[0].searchParams.has("sharing")).toBe(false);
    const views = reads(seen, "/views");
    expect(views).toHaveLength(1);
    expect(views[0].searchParams.has("resource")).toBe(false);
    const mine = region(en["filters.library.mine"]);
    expect(within(mine).getByText(BERLIN.name)).toBeInTheDocument();
    expect(within(mine).getByText(DINNER.name)).toBeInTheDocument();
    expect(mine.querySelector(".panel-head .t-num")?.textContent).toBe("3");
    const shared = region(en["filters.library.shared"]);
    expect(within(shared).getByText(shortlist.name)).toBeInTheDocument();
    expect(shared.querySelector(".panel-head .t-num")?.textContent).toBe("2");
  });

  it("opens on one plate, with nothing to search, when nothing is saved", async () => {
    const { wrapper } = mountFilters({ listsOn: true });
    render(<FiltersScreen />, { wrapper });
    expect(
      await screen.findByText(en["filters.library.firstRunTitle"]),
    ).toBeInTheDocument();
    expect(screen.getByText(en["filters.library.firstRunBody"])).toBeVisible();
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(
      screen.queryByRole("group", { name: en["filters.objectLabel"] }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: en["lists.newShortlist"] }),
    ).toBeInTheDocument();
  });
});

describe("with lists off", () => {
  it("draws the saved views alone, and asks nothing of /lists", async () => {
    const { seen, wrapper } = mountFilters({ views: [BERLIN] });
    render(<FiltersScreen />, { wrapper });
    const views = await screen.findByRole("region", {
      name: en["filters.library.views"],
    });
    expect(await within(views).findByText(BERLIN.name)).toBeInTheDocument();
    expect(reads(seen, "/lists")).toHaveLength(0);
    expect(
      within(views).queryByRole("columnheader", { name: en["lists.col.kind"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["lists.newShortlist"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: en["filters.library.showArchived"],
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("region", { name: en["filters.library.mine"] }),
    ).toBeNull();
  });

  it("says how a first view comes to exist when there is none", async () => {
    const { wrapper } = mountFilters({});
    render(<FiltersScreen />, { wrapper });
    expect(
      await screen.findByText(en["filters.library.viewsEmptyTitle"]),
    ).toBeInTheDocument();
    expect(
      screen.getByText(en["filters.library.viewsEmptyBody"]),
    ).toBeVisible();
  });

  it("is what an address it does not recognise opens", async () => {
    const { seen, wrapper } = mountFilters({});
    render(<FiltersScreen id="widgets" />, { wrapper });
    expect(
      await screen.findByText(en["filters.library.viewsEmptyTitle"]),
    ).toBeInTheDocument();
    expect(seen.some((sent) => sent.url.includes("/filters/vocabulary"))).toBe(
      false,
    );
  });
});

describe("the cut", () => {
  it("narrows to a record type in the address, and the verb follows it", async () => {
    window.location.hash = "#/filters?ask=1";
    const { wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN, FLEET],
      lists: [liveList],
    });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    await screen.findByText(BERLIN.name);
    await user.click(screen.getByRole("button", { name: /^Companies/ }));
    expect(window.location.hash).toBe("#/filters?ask=1&type=companies");
    expect(screen.queryByText(BERLIN.name)).toBeNull();
    expect(screen.getByText(FLEET.name)).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: en["filters.new.companies"] }),
    );
    expect(window.location.hash).toBe("#/filters/companies");
  });

  it("asks which records a new filter is for, and Escape asks nothing", async () => {
    const { wrapper } = mountFilters({ listsOn: true, views: [BERLIN] });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    await screen.findByText(BERLIN.name);
    await user.click(
      screen.getByRole("button", { name: en["filters.library.newFilter"] }),
    );
    const picker = screen.getByRole("region", {
      name: en["filters.library.newFilter"],
    });
    expect(
      within(picker).getByText(en["filters.library.whichRecords"]),
    ).toBeVisible();
    await user.keyboard("{Escape}");
    expect(screen.queryByText(en["filters.library.whichRecords"])).toBeNull();
    expect(window.location.hash).toBe("");
    await user.click(
      screen.getByRole("button", { name: en["filters.library.newFilter"] }),
    );
    await user.click(
      within(
        screen.getByRole("region", { name: en["filters.library.newFilter"] }),
      ).getByRole("button", { name: en["filters.tab.deals"] }),
    );
    expect(window.location.hash).toBe("#/filters/deals");
  });

  it("keeps asking which records under Projects, where no filter is built", async () => {
    window.location.hash = "#/filters?type=projects";
    const { wrapper } = mountFilters({
      listsOn: true,
      lists: [BUILDS, liveList],
    });
    render(<FiltersScreen />, { wrapper });
    expect(await screen.findByText(BUILDS.name)).toBeInTheDocument();
    expect(screen.queryByText(liveList.name)).toBeNull();
    expect(
      screen.getByRole("button", { name: en["filters.library.newFilter"] }),
    ).toHaveAttribute("aria-expanded", "false");
  });

  it("says a search found nothing, and clearing it brings the rows back", async () => {
    const { wrapper } = mountFilters({ views: [BERLIN] });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    await screen.findByText(BERLIN.name);
    await user.type(screen.getByRole("searchbox"), "zzz");
    expect(
      screen.getByText("No views or lists match “zzz”."),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: en["filters.library.clearSearch"] }),
    );
    expect(screen.getByText(BERLIN.name)).toBeInTheDocument();
    expect(window.location.hash).not.toContain("q=");
  });

  it("says a pill holds nothing, and Show all brings the rows back", async () => {
    window.location.hash = "#/filters?type=leads";
    const { wrapper } = mountFilters({ views: [BERLIN] });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    expect(
      await screen.findByText(en["filters.library.noTypeHits.leads"]),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: en["list.showAll"] }));
    expect(screen.getByText(BERLIN.name)).toBeInTheDocument();
  });

  it("is restored from the address, so Back returns to it", async () => {
    window.location.hash = "#/filters?q=berlin&type=contacts";
    const { wrapper } = mountFilters({
      views: [
        BERLIN,
        FLEET,
        filterView("v3", "Hamburg contacts", "contacts", {
          and: [{ field: "city", op: "eq", value: "Hamburg" }],
        }),
      ],
    });
    render(<FiltersScreen />, { wrapper });
    expect(await screen.findByText(BERLIN.name)).toBeInTheDocument();
    expect(screen.queryByText(FLEET.name)).toBeNull();
    expect(screen.queryByText("Hamburg contacts")).toBeNull();
    expect(screen.getByRole("searchbox")).toHaveValue("berlin");
  });
});

describe("a read that fails", () => {
  it("leaves the saved views standing when the lists did not load", async () => {
    const { seen, wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN],
      listsFail: true,
    });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    const mine = await screen.findByRole("region", {
      name: en["filters.library.mine"],
    });
    expect(await within(mine).findByText(BERLIN.name)).toBeInTheDocument();
    expect(
      within(mine).getByText(en["filters.library.listsFailed"]),
    ).toBeInTheDocument();
    const shared = region(en["filters.library.shared"]);
    expect(
      within(shared).getByText(en["filters.library.listsFailed"]),
    ).toBeInTheDocument();
    const before = reads(seen, "/lists").length;
    await user.click(
      within(shared).getByRole("button", { name: en["common.retry"] }),
    );
    await vi.waitFor(() =>
      expect(reads(seen, "/lists").length).toBeGreaterThan(before),
    );
  });

  it("says the saved views did not load where they would be", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      lists: [DINNER],
      viewsFail: true,
    });
    render(<FiltersScreen />, { wrapper });
    const mine = await screen.findByRole("region", {
      name: en["filters.library.mine"],
    });
    expect(
      await within(mine).findByText(en["filters.library.viewsFailed"]),
    ).toBeInTheDocument();
    expect(within(mine).getByText(DINNER.name)).toBeInTheDocument();
  });

  it("claims no empty pill and prints no count while a read is refused", async () => {
    window.location.hash = "#/filters?type=companies";
    const { wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN],
      listsFail: true,
    });
    render(<FiltersScreen />, { wrapper });
    const mine = await screen.findByRole("region", {
      name: en["filters.library.mine"],
    });
    expect(
      await within(mine).findByText(en["filters.library.listsFailed"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(en["filters.library.noTypeHits.companies"]),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: en["filters.library.all"] }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["filters.tab.companies"] }),
    ).toBeInTheDocument();
  });
});

describe("a read the server cut short", () => {
  it("says so, drops the counts, and takes a paused search to the server", async () => {
    const { seen, wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN],
      lists: [shortlist, liveList],
      listsCap: 1,
    });
    render(<FiltersScreen />, { wrapper });
    const caption = "Showing the first 1. Search to narrow the list.";
    expect(await screen.findByText(caption)).toBeInTheDocument();
    const uncounted = () =>
      screen.getByRole("button", { name: en["filters.library.all"] });
    expect(uncounted()).toBeInTheDocument();
    expect(
      region(en["filters.library.mine"]).querySelector(".t-num"),
    ).toBeNull();
    const searched = () =>
      reads(seen, "/lists").flatMap((url) => url.searchParams.getAll("q"));
    // Typed faster than the pause, so only the word the reader stopped on goes.
    vi.useFakeTimers();
    const box = screen.getByRole("searchbox");
    fireEvent.change(box, { target: { value: "qui" } });
    fireEvent.change(box, { target: { value: "quiet" } });
    act(() => {
      vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS);
    });
    vi.useRealTimers();
    expect(await screen.findByText(liveList.name)).toBeInTheDocument();
    expect(searched()).toEqual(["quiet"]);
    // Found in full, yet the library itself is still past its cap.
    expect(screen.getByText(caption)).toBeInTheDocument();
    expect(uncounted()).toBeInTheDocument();
  });
});

describe("archived lists", () => {
  it("are asked for on request, marked, and put away again", async () => {
    const old: List = {
      ...shortlist,
      id: "L-old",
      name: "Old campaign",
      archived_at: "2026-09-01T00:00:00Z",
    };
    let answer = () => {};
    const { seen, wrapper } = mountFilters({
      listsOn: true,
      lists: [liveList, old],
      archivedAnswered: new Promise((resolve) => {
        answer = resolve;
      }),
    });
    const user = userEvent.setup();
    render(<FiltersScreen />, { wrapper });
    await screen.findByText(liveList.name);
    expect(screen.queryByText(old.name)).toBeNull();
    await user.click(
      screen.getByRole("button", { name: en["filters.library.showArchived"] }),
    );
    // The rows held stay up while the archived ones are asked for.
    expect(screen.getByText(liveList.name)).toBeInTheDocument();
    expect(screen.queryByText(en["filters.library.loading"])).toBeNull();
    answer();
    const row = (await screen.findByText(old.name)).closest(
      "tr",
    ) as HTMLElement;
    expect(within(row).getByText(en["record.archived"])).toBeInTheDocument();
    expect(
      reads(seen, "/lists").some(
        (url) => url.searchParams.get("include_archived") === "true",
      ),
    ).toBe(true);
    expect(
      screen.getByRole("button", { name: en["filters.library.hideArchived"] }),
    ).toBeInTheDocument();
  });
});

describe("a group's own address", () => {
  it("lands on Shared for #/filters/lists", async () => {
    const { wrapper } = mountFilters({
      listsOn: true,
      views: [BERLIN],
      lists: [liveList],
    });
    render(<FiltersScreen id="lists" />, { wrapper });
    await screen.findByText(liveList.name);
    await vi.waitFor(() =>
      expect(region(en["filters.library.shared"]).parentElement).toHaveFocus(),
    );
  });

  it("lands on Only me for #/filters/views, when both reads were already held", async () => {
    const { client, wrapper } = mountFilters({ listsOn: true });
    client.setQueryData(
      ["me"],
      meFixture({ settingsAvailability: { lists: true } }),
    );
    client.setQueryData(["views", "all"], {
      views: [BERLIN],
      truncated: false,
    });
    client.setQueryData(["lists", "all", { includeArchived: false }], {
      data: [liveList],
      page: { has_more: false },
    });
    render(<FiltersScreen id="views" />, { wrapper });
    await vi.waitFor(() =>
      expect(region(en["filters.library.mine"]).parentElement).toHaveFocus(),
    );
  });

  it("sends #/lists, which names no list, to the shared lists without a history entry", async () => {
    window.location.hash = "#/lists";
    const length = window.history.length;
    const { wrapper } = mountFilters({ listsOn: true });
    render(<FiltersScreen list="" />, { wrapper });
    await vi.waitFor(() =>
      expect(window.location.hash).toBe("#/filters/lists"),
    );
    expect(window.history.length).toBe(length);
  });
});
