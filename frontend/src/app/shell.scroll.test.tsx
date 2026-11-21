// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  within,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecordTabs } from "../design-system/recordtabs";
import { en } from "../i18n/en";
import { samePage, useHash, useRoute } from "../lib/router";
import { useReportShownAddress } from "../lib/screenaddress";
import { forgetScrollMemory, keepPlaceOnNextMove } from "../lib/scrollmemory";
import { ignoreSearch, render } from "../testing/shellharness";
import { Shell } from "./shell";

// The routed screen's part of the contract: it names the address it shows.
function Showing({
  address,
  pending = false,
}: Readonly<{ address: string; pending?: boolean }>) {
  useReportShownAddress(address, pending);
  return <p>{address}</p>;
}

// A screen whose next page is still loading: it stays on its page until Land.
function Loading({ from }: Readonly<{ from: string }>) {
  const live = useHash();
  const [address, setAddress] = useState(from);
  return (
    <>
      <Showing address={address} pending={!samePage(address, live)} />
      <button type="button" onClick={() => setAddress(live)}>
        Land
      </button>
    </>
  );
}

// A screen that is never held: it shows whatever the bar says.
function ShowingTheBar() {
  return <Showing address={useHash()} />;
}

type Tab = "overview" | "tasks";

// An account page with its strip and, in the Overview's body, a door to Tasks.
function Account() {
  const route = useRoute();
  const tab: Tab = route.id2 === "tasks" ? "tasks" : "overview";
  // A new history entry, as the browser makes for a hash move.
  const open = (next: Tab) => {
    window.history.pushState(null, "", `#/companies/o-1/${next}`);
    window.dispatchEvent(new PopStateEvent("popstate"));
  };
  return (
    <>
      <Showing address={useHash()} />
      <RecordTabs
        options={["overview", "tasks"]}
        value={tab}
        onChange={open}
        keepPlace
        labels={{ overview: "Overview", tasks: "Tasks" }}
      />
      <button type="button" onClick={() => open("tasks")}>
        All tasks
      </button>
    </>
  );
}

// A deal's strip: its tab lives in state, so a press moves no address.
function Deal() {
  const [tab, setTab] = useState<Tab>("overview");
  return (
    <>
      <Showing address={useHash()} />
      <RecordTabs
        options={["overview", "tasks"]}
        value={tab}
        onChange={setTab}
        keepPlace
        labels={{ overview: "Overview", tasks: "Tasks" }}
      />
    </>
  );
}

async function pressAndArrive(name: string, hash: string) {
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name }));
  });
  expect(window.location.hash).toBe(hash);
}

function column(): HTMLElement {
  const scroller = document.querySelector<HTMLElement>(".scroll");
  if (!scroller) throw new Error("the shell drew no content column");
  return scroller;
}

function scrollColumnTo(top: number) {
  column().scrollTop = top;
  act(() => {
    column().dispatchEvent(new Event("scroll"));
  });
}

async function moveTo(hash: string, state: unknown = null) {
  await act(async () => {
    window.history.pushState(state, "", hash);
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  forgetScrollMemory();
  window.history.replaceState(null, "", "#/");
});

describe("the content column's place", () => {
  it("stays where the reader left a screen still on screen past a move", async () => {
    window.history.replaceState(null, "", "#/contacts");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Showing address="#/contacts" />
      </Shell>,
    );
    scrollColumnTo(300);

    await moveTo("#/companies");

    expect(column().scrollTop).toBe(300);
  });

  it("follows an entry that differs from the last only in its dials", async () => {
    window.history.replaceState(null, "", "#/contacts?page=2");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <ShowingTheBar />
      </Shell>,
    );
    scrollColumnTo(300);

    await moveTo("#/contacts?page=3");

    expect(column().scrollTop).toBe(0);
  });

  it("keeps the reader's place on another tab of the same record", async () => {
    window.history.replaceState(null, "", "#/contacts/p-1");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <ShowingTheBar />
      </Shell>,
    );
    scrollColumnTo(300);

    keepPlaceOnNextMove();
    await moveTo("#/contacts/p-1/timeline");
    expect(column().scrollTop).toBe(300);

    keepPlaceOnNextMove();
    await moveTo("#/contacts/p-2");
    expect(column().scrollTop).toBe(0);
  });

  it("opens another tab of the record at its top when no press asked to keep the place", async () => {
    window.history.replaceState(null, "", "#/companies/o-1");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <ShowingTheBar />
      </Shell>,
    );
    scrollColumnTo(300);

    await moveTo("#/companies/o-1/tasks");
    expect(column().scrollTop).toBe(0);
  });

  it("keeps the place when a tab move drops a dial of the record", async () => {
    window.history.replaceState(null, "", "#/contacts/p-1?thread=k");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <ShowingTheBar />
      </Shell>,
    );
    scrollColumnTo(300);

    keepPlaceOnNextMove();
    await moveTo("#/contacts/p-1/timeline");
    expect(column().scrollTop).toBe(300);
  });

  it.each([
    ["a settings tab", "#/settings/voice", "#/settings/voice/samples"],
    ["an analytics section", "#/analytics/overview", "#/analytics/reports"],
  ])("opens %s at the top", async (_name, from, to) => {
    window.history.replaceState(null, "", from);
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <ShowingTheBar />
      </Shell>,
    );
    scrollColumnTo(300);

    keepPlaceOnNextMove();
    await moveTo(to);
    expect(column().scrollTop).toBe(0);
  });

  it("keeps the place for a strip press and opens a door's tab at its top", async () => {
    window.history.replaceState(null, "", "#/companies/o-1");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Account />
      </Shell>,
    );
    scrollColumnTo(300);
    await pressAndArrive("Tasks", "#/companies/o-1/tasks");
    expect(column().scrollTop).toBe(300);

    await moveTo("#/companies/o-1");
    scrollColumnTo(300);
    await pressAndArrive("All tasks", "#/companies/o-1/tasks");
    expect(column().scrollTop).toBe(0);
  });

  it("opens a new entry of the record at its top after a press that moved no address", async () => {
    window.history.replaceState(null, "", "#/deals/d-1");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Deal />
      </Shell>,
    );
    scrollColumnTo(300);
    fireEvent.click(screen.getByRole("button", { name: "Tasks" }));
    expect(column().scrollTop).toBe(300);

    // The same deal opened again, from search.
    await moveTo("#/deals/d-1?from=search");
    expect(column().scrollTop).toBe(0);
  });
});

function main(): HTMLElement {
  const column = document.querySelector<HTMLElement>("main");
  if (!column) throw new Error("the shell drew no main column");
  return column;
}

function trail(): string[] {
  const nav = screen.getByRole("navigation", {
    name: en["shell.breadcrumbAria"],
  });
  return within(nav)
    .getAllByRole("listitem")
    .map((stop) => stop.textContent ?? "");
}

function wait(): HTMLElement {
  const region = document.querySelector<HTMLElement>(".pagewait");
  if (!region) throw new Error("the shell drew no page wait");
  return region;
}

function activeRow(): Element | null {
  return document.querySelector("nav.rail a.navitem.active");
}

const LOADING_SCHEDULED = en["settings.pageLoading"].replace(
  "{page}",
  en["nav.scheduled"],
);

describe("the chrome while the next page loads", () => {
  it("names the page still on screen, once", async () => {
    window.history.replaceState(null, "", "#/analytics");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Loading from="#/analytics" />
      </Shell>,
    );

    await moveTo("#/scheduled");

    const headings = screen.getAllByRole("heading", { level: 1 });
    expect(headings.map((heading) => heading.textContent)).toEqual([
      "Analytics",
    ]);
    expect(trail()).toEqual(["Analytics"]);
    expect(document.title.startsWith("Analytics ·")).toBe(true);
    expect(activeRow()?.textContent).toBe("Analytics");
    expect(activeRow()).toHaveAttribute("aria-current", "page");
  });

  it("shows the wait only after 200 ms, and drops it when the page lands", async () => {
    vi.useFakeTimers();
    window.history.replaceState(null, "", "#/analytics");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Loading from="#/analytics" />
      </Shell>,
    );
    await moveTo("#/scheduled");

    act(() => vi.advanceTimersByTime(199));
    expect(main()).not.toHaveAttribute("aria-busy");
    expect(document.querySelector(".pagewait-line")).toBeNull();
    expect(wait()).toHaveTextContent("");

    act(() => vi.advanceTimersByTime(1));
    expect(main()).toHaveAttribute("aria-busy", "true");
    expect(document.querySelector(".pagewait-line")).not.toBeNull();
    expect(wait()).toHaveAttribute("role", "status");
    expect(wait()).toHaveTextContent(LOADING_SCHEDULED);

    fireEvent.click(screen.getByRole("button", { name: "Land" }));
    expect(main()).not.toHaveAttribute("aria-busy");
    expect(document.querySelector(".pagewait-line")).toBeNull();
    expect(
      screen.getByRole("heading", { level: 1, name: en["nav.scheduled"] }),
    ).toBeInTheDocument();
  });

  it.each([
    [
      "a settings page by its own name",
      "#/settings/account",
      en["settings.tab.account"],
    ],
    [
      "a record whose name is not read by its list",
      "#/contacts/0a7e5c1d-6b2f-4c8e-9d3a-1f2e3d4c5b6a",
      en["nav.contacts"],
    ],
  ])("names %s", async (_case, destination, name) => {
    vi.useFakeTimers();
    window.history.replaceState(null, "", "#/analytics");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Loading from="#/analytics" />
      </Shell>,
    );
    await moveTo(destination);

    await act(() => vi.advanceTimersByTimeAsync(200));
    expect(wait()).toHaveTextContent(
      en["settings.pageLoading"].replace("{page}", name),
    );
  });

  it("shows no wait for a move that lands inside 200 ms", async () => {
    vi.useFakeTimers();
    window.history.replaceState(null, "", "#/analytics");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Loading from="#/analytics" />
      </Shell>,
    );
    await moveTo("#/scheduled");
    act(() => vi.advanceTimersByTime(150));
    fireEvent.click(screen.getByRole("button", { name: "Land" }));

    act(() => vi.advanceTimersByTime(1000));
    expect(main()).not.toHaveAttribute("aria-busy");
    expect(document.querySelector(".pagewait-line")).toBeNull();
  });

  it("names the bar's page before any screen has reported", () => {
    window.history.replaceState(null, "", "#/analytics");
    render(<Shell onOpenSearch={ignoreSearch}>{null}</Shell>);

    expect(
      screen.getByRole("heading", { level: 1, name: "Analytics" }),
    ).toBeInTheDocument();
    expect(activeRow()?.textContent).toBe("Analytics");
    expect(main()).not.toHaveAttribute("aria-busy");
  });

  it("keeps the reading column of the page on screen", async () => {
    window.history.replaceState(null, "", "#/settings/voice");
    render(
      <Shell onOpenSearch={ignoreSearch}>
        <Loading from="#/settings/voice" />
      </Shell>,
    );
    expect(main()).toHaveClass("main-gridded");

    await moveTo("#/contacts");
    expect(main()).toHaveClass("main-gridded");

    fireEvent.click(screen.getByRole("button", { name: "Land" }));
    expect(main()).not.toHaveClass("main-gridded");
  });
});
