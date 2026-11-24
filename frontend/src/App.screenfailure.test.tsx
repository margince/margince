/** @vitest-environment happy-dom */
import { act, cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Heading } from "./design-system/heading";
import {
  memoryStorage,
  renderApp,
  sessionOnlyFetch,
  teardownApp,
} from "./testing/appharness";

// A routed screen that throws while it renders, inside the real shell. The
// composed screen registry is the one place a test can put its own screen
// behind a real route without stubbing the dispatch it is testing.
const fragile = vi.hoisted(() => ({ broken: true }));

vi.mock("@composition/extensions", () => ({
  extensions: [
    {
      name: "notes",
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
    },
  ],
}));

vi.mock("@composition/screens", () => ({
  extensionScreens: {
    notes: () => {
      if (fragile.broken) {
        // What a malformed record does to a screen: a string method on a number.
        throw new TypeError("name.split is not a function");
      }
      return <Heading size="xlarge">Demo Notepad</Heading>;
    },
  },
  extensionPanels: {},
  extensionApps: {},
}));

const TITLE = "This view stopped working";

beforeEach(() => {
  fragile.broken = true;
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  vi.stubGlobal("fetch", vi.fn(sessionOnlyFetch()));
  // React logs the throw the boundary catches; here it is the case itself.
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(async () => {
  cleanup();
  await teardownApp();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.location.hash = "";
});

async function openBrokenScreen() {
  window.location.hash = "#/ext/notes";
  renderApp();
  return screen.findByRole("alert", {}, { timeout: 5000 });
}

describe("a screen that throws while it renders", () => {
  it("fails inside the shell, with the rail and the top bar still drawn", async () => {
    const failure = await openBrokenScreen();

    expect(failure.textContent).toContain(TITLE);
    expect(
      screen.getByRole("navigation", { name: "Primary navigation" }),
    ).toBeTruthy();
    expect(document.querySelector("header.topbar")).not.toBeNull();
    expect(document.querySelector(".scroll")?.contains(failure)).toBe(true);
    expect(failure.classList.contains("errorboundary-page")).toBe(false);
    expect(document.body.textContent).not.toContain("split is not a function");
  });

  it("draws the screen again when the reader retries", async () => {
    const user = userEvent.setup();
    await openBrokenScreen();

    fragile.broken = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(
      await screen.findByRole("heading", { name: "Demo Notepad" }),
    ).toBeTruthy();
    expect(screen.queryByText(TITLE)).toBeNull();
  });

  it("drops the failure once the reader moves to another route", async () => {
    await openBrokenScreen();

    act(() => {
      window.location.hash = "#/ext/constructor";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    await vi.waitFor(() => expect(screen.queryByText(TITLE)).toBeNull());
    expect(
      screen.getByRole("navigation", { name: "Primary navigation" }),
    ).toBeTruthy();
  });

  it("is drawn afresh when the reader comes back to the screen", async () => {
    await openBrokenScreen();

    act(() => {
      window.location.hash = "#/ext/constructor";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    await vi.waitFor(() => expect(screen.queryByText(TITLE)).toBeNull());
    fragile.broken = false;
    act(() => {
      window.location.hash = "#/ext/notes";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });

    expect(
      await screen.findByRole("heading", { name: "Demo Notepad" }),
    ).toBeTruthy();
  });
});
