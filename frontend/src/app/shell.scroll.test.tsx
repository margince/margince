// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { RecordTabs } from "../design-system/recordtabs";
import { useHash, useRoute } from "../lib/router";
import { useReportShownAddress } from "../lib/screenaddress";
import { forgetScrollMemory, keepPlaceOnNextMove } from "../lib/scrollmemory";
import { ignoreSearch, render } from "../testing/shellharness";
import { Shell } from "./shell";

// The routed screen's part of the contract: it names the address it shows.
function Showing({ address }: Readonly<{ address: string }>) {
  useReportShownAddress(address);
  return <p>{address}</p>;
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
