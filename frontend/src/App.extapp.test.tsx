/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Heading } from "./design-system/heading";
import {
  memoryStorage,
  renderApp,
  sessionOnlyFetch,
  teardownApp,
} from "./testing/appharness";
import { type GrantSpec, meFixture } from "./testing/mefixture";

// A composed unit that declares an app, beside one that declares none and one
// whose package ships the app but no default export. The labels are core keys,
// because the merged catalogue holds no unit copy in this lane and a core key
// renders the same way.
vi.mock("@composition/extensions", () => ({
  extensions: [
    {
      name: "notes",
      title: "",
      verbs: [
        {
          operationId: "notesList",
          route: "/ext/notes/list",
          method: "POST",
          title: "List demo notes",
          version: "1.0.0",
          rbacObject: "ext_notes_note",
        },
      ],
      entries: [],
      panels: [],
      secretScope: "",
      app: {
        icon: "plug",
        label: "extUnits.user.title",
        group: "work",
        object: "ext_notes_note",
        export: "NotesApp",
      },
    },
    {
      name: "plain",
      title: "",
      verbs: [],
      entries: [],
      panels: [],
      secretScope: "",
    },
    {
      name: "bare",
      title: "",
      verbs: [
        {
          operationId: "barePing",
          route: "/ext/bare/ping",
          method: "POST",
          title: "Ping the bare unit",
          version: "1.0.0",
          rbacObject: "ext_bare_thing",
        },
      ],
      entries: [],
      panels: [],
      secretScope: "",
      app: {
        icon: "plug",
        label: "extUnits.workspace.title",
        group: "records",
        object: "ext_bare_thing",
        export: "BareApp",
      },
    },
  ],
}));

vi.mock("@composition/screens", () => ({
  extensionScreens: {
    notes: () => <Heading size="xlarge">Notes settings screen</Heading>,
    plain: () => <Heading size="xlarge">Plain screen</Heading>,
  },
  extensionPanels: {},
  extensionApps: {
    notes: () => <Heading size="xlarge">Notes app screen</Heading>,
    bare: () => <Heading size="xlarge">Bare app screen</Heading>,
  },
}));

let allow: GrantSpec = {};

beforeEach(() => {
  allow = { ext_notes_note: ["read"], ext_bare_thing: ["read"] };
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  const session = sessionOnlyFetch();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.endsWith("/v1/me")) {
        return new Response(JSON.stringify(meFixture({ allow })), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return session(input);
    }),
  );
});

afterEach(async () => {
  cleanup();
  await teardownApp();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("a unit's app in the home side drawer", () => {
  it("offers the entry under its heading, and pressing it draws the app", async () => {
    window.location.hash = "#/contacts";
    renderApp();
    const entry = await screen.findByRole("link", { name: "Other accounts" });
    expect(entry.getAttribute("href")).toMatch(/#\/app\/notes$/);
    expect(entry.querySelector("svg")).not.toBeNull();
    await userEvent.setup().click(entry);
    expect(
      await screen.findByRole("heading", { name: "Notes app screen" }),
    ).toBeTruthy();
    expect(screen.queryByText("Notes settings screen")).toBeNull();
  });

  it("offers a reader without the unit's object neither the entry nor the route", async () => {
    allow = { ext_bare_thing: ["read"] };
    window.location.hash = "#/app/notes";
    renderApp();
    // The drawer shows the entry the reader may open, and only that one.
    expect(
      await screen.findByRole("link", { name: "Installation add-ons" }),
    ).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Other accounts" })).toBeNull();
    expect(screen.queryByText("Notes app screen")).toBeNull();
    expect(await screen.findAllByText("Not found")).not.toHaveLength(0);
    // The page chrome does not name the app to a reader who may not open it.
    expect(screen.queryByText("Other accounts")).toBeNull();
  });

  it("keeps the sub-setting at its own address, under Settings", async () => {
    window.location.hash = "#/ext/notes";
    renderApp();
    expect(
      await screen.findByRole("heading", { name: "Notes settings screen" }),
    ).toBeTruthy();
    expect(screen.queryByText("Notes app screen")).toBeNull();
  });

  it("links the app to the unit's settings page", async () => {
    window.location.hash = "#/app/notes";
    renderApp();
    await screen.findByRole("heading", { name: "Notes app screen" });
    const main = screen.getByRole("main");
    const link = within(main).getByRole("link", { name: "Settings for Notes" });
    expect(link.getAttribute("href")).toMatch(/#\/ext\/notes$/);
  });

  it("names the page by the app's own words, with no Settings trail", async () => {
    window.location.hash = "#/app/notes";
    renderApp();
    await screen.findByRole("heading", { name: "Notes app screen" });
    const trail = screen.getByRole("navigation", { name: /breadcrumb/i });
    expect(within(trail).queryByText("Settings")).toBeNull();
    expect(within(trail).getByText("Other accounts")).toBeTruthy();
  });

  it("shows the fallback card at the sub-setting address when the unit ships no default export", async () => {
    window.location.hash = "#/ext/bare";
    renderApp();
    expect(await screen.findByText(/Ping the bare unit/)).toBeTruthy();
  });

  it("gives a unit that declares no app no route of its own", async () => {
    window.location.hash = "#/app/plain";
    renderApp();
    expect(await screen.findAllByText("Not found")).not.toHaveLength(0);
    expect(screen.queryByText("Plain screen")).toBeNull();
  });
});
