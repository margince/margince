// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ScreenView } from "./App.views";
import { Shell } from "./app/shell";
import { withListPage } from "./components/listqueryparams";
import { LocaleProvider } from "./i18n";
import { navigateReplacing, useHash, useRoute } from "./lib/router";
import { settingsRouteTarget } from "./lib/settingsrouting";
import { useUnsavedGuard } from "./lib/unsavedclaims";
import { currentParams, type UrlParams, useUrlParams } from "./lib/urlstate";
import { meFixture, READS_RECORDS } from "./testing/mefixture";

// Each routed screen shows the address it reads, carries a draft, and exposes
// its dial writer so a test can write late.
const writers = new Map<string, (next: UrlParams) => void>();

vi.mock("./App.screens", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./App.screens")>()),
  ContactsScreen: () => <AddressedPage name="contacts" />,
  ContactPageV2: () => <AddressedPage name="contact" />,
  CompaniesScreen: () => <AddressedPage name="companies" />,
  SettingsScreen: ({
    route,
  }: {
    route: Parameters<typeof settingsRouteTarget>[0];
  }) => <p>{`settings ${settingsRouteTarget(route).kind}`}</p>,
}));

function AddressedPage({ name }: Readonly<{ name: string }>) {
  const hash = useHash();
  const route = useRoute();
  const [, setParams] = useUrlParams();
  writers.set(name, setParams);
  const [typed, setTyped] = useState("");
  useUnsavedGuard(typed !== "");
  return (
    <section aria-label={name}>
      <p>{`reads ${hash} as ${route.screen}`}</p>
      <label>
        Draft
        <input
          value={typed}
          onChange={(event) => setTyped(event.target.value)}
        />
      </label>
    </section>
  );
}

// The route as App hands it to ScreenView: three segments, read live.
function Routed() {
  const route = useRoute();
  return <ScreenView screen={route.screen} id={route.id} id2={route.id2} />;
}

function renderRouted(at: string, framed = false) {
  window.history.replaceState(null, "", at);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({ allow: READS_RECORDS }));
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {framed ? (
          <Shell onOpenSearch={() => undefined}>
            <Routed />
          </Shell>
        ) : (
          <Routed />
        )}
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function moveTo(hash: string) {
  await act(async () => {
    window.history.pushState(null, "", hash);
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
}

async function holdDraftOn(
  at: string,
  name: string,
  leaveFor: string,
  framed = false,
) {
  const user = userEvent.setup(
    framed ? { advanceTimers: vi.advanceTimersByTime } : undefined,
  );
  renderRouted(at, framed);
  const page = await screen.findByRole("region", { name });
  await user.type(page.querySelector("input") ?? document.body, "Unsaved");
  await moveTo(leaveFor);
  await screen.findByRole("dialog");
  return user;
}

beforeEach(() => {
  writers.clear();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "#/");
});

describe("a screen held on screen past a move", () => {
  it("reads the address it was drawn for, dials included", async () => {
    await holdDraftOn("#/contacts?page=3", "contacts", "#/companies?x=1");

    expect(
      screen.getByText("reads #/contacts?page=3 as contacts"),
    ).toBeInTheDocument();
  });

  it("writes none of its dials into the entry the reader arrived on", async () => {
    await holdDraftOn("#/contacts?page=3", "contacts", "#/companies?x=1");

    act(() => writers.get("contacts")?.(new Map([["page", "9"]])));

    expect(window.location.hash).toBe("#/companies?x=1");
  });

  it("still turns its own dials while it is the live page", async () => {
    renderRouted("#/contacts?page=3");
    await screen.findByRole("region", { name: "contacts" });

    act(() => writers.get("contacts")?.(new Map([["page", "4"]])));

    expect(window.location.hash).toBe("#/contacts?page=4");
    expect(
      await screen.findByText("reads #/contacts?page=4 as contacts"),
    ).toBeInTheDocument();
  });

  it("is not held by a dial turned under a draft", async () => {
    const user = userEvent.setup();
    renderRouted("#/contacts?page=3");
    const page = await screen.findByRole("region", { name: "contacts" });
    await user.type(page.querySelector("input") ?? document.body, "Unsaved");

    act(() => writers.get("contacts")?.(new Map([["page", "4"]])));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      await screen.findByText("reads #/contacts?page=4 as contacts"),
    ).toBeInTheDocument();
  });
});

describe("the frame over a held draft", () => {
  it("names the held page and shows no wait", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await holdDraftOn("#/contacts", "contacts", "#/companies", true);

    act(() => vi.advanceTimersByTime(1000));

    const trail = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(trail).toHaveTextContent("Contacts");
    expect(trail).not.toHaveTextContent("Companies");
    expect(document.title.startsWith("Contacts ·")).toBe(true);
    expect(
      document.querySelector("nav.rail a.navitem.active")?.textContent,
    ).toBe("Contacts");
    const column = document.querySelector("main > .scroll");
    expect(column).not.toBeNull();
    expect(column).not.toHaveAttribute("aria-busy");
    expect(document.querySelector(".pagewait-line")).toBeNull();
  });
});

describe("a list the reader has left", () => {
  it("cannot send the list they opened back to page one", async () => {
    renderRouted("#/companies");
    await screen.findByRole("region", { name: "companies" });
    await moveTo("#/contacts?page=3");
    await screen.findByText("reads #/contacts?page=3 as contacts");

    // The write a list's reset makes when its narrowing changes on the way out.
    act(() => writers.get("companies")?.(withListPage(currentParams(), 1)));

    expect(window.location.hash).toBe("#/contacts?page=3");
    expect(
      screen.getByText("reads #/contacts?page=3 as contacts"),
    ).toBeInTheDocument();
  });
});

describe("Stay on a held draft", () => {
  const HELD = "#/contacts/c1/notes/more?page=3";

  it("goes back through history to the entry the draft was on", async () => {
    const go = vi.spyOn(window.history, "go");
    const user = await holdDraftOn(HELD, "contact", "#/companies");
    const length = window.history.length;

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(go.mock.calls).toEqual([[-1]]);
    await waitFor(() => expect(window.location.hash).toBe(HELD));
    expect(window.history.length).toBe(length);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("waits for a traversal under way and overwrites no entry", async () => {
    const go = vi.spyOn(window.history, "go").mockImplementation(() => {});
    const user = await holdDraftOn(HELD, "contact", "#/companies");

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(go.mock.calls).toEqual([[-1]]);
    expect(window.location.hash).toBe("#/companies");
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    act(() => window.history.back());
    await waitFor(() => expect(window.location.hash).toBe(HELD));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("puts the whole held address back on an entry history cannot leave", async () => {
    const go = vi.spyOn(window.history, "go");
    const user = userEvent.setup();
    renderRouted(HELD);
    const page = await screen.findByRole("region", { name: "contact" });
    await user.type(page.querySelector("input") ?? document.body, "Unsaved");
    act(() => navigateReplacing({ screen: "companies" }));
    await screen.findByRole("dialog");
    const length = window.history.length;

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(go).not.toHaveBeenCalled();
    expect(window.location.hash).toBe(HELD);
    expect(window.history.length).toBe(length);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("the fourth segment", () => {
  it("reaches settings, which refuses a segment below a subpage", async () => {
    renderRouted("#/settings/import/run/x");

    expect(await screen.findByText("settings unknown")).toBeInTheDocument();
  });
});
