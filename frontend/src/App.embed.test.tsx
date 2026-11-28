/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Heading } from "./design-system/heading";
import {
  memoryStorage,
  renderApp,
  sessionOnlyFetch,
  teardownApp,
} from "./testing/appharness";

// The embed path is the one page an operator lets another origin frame, such as
// a browser extension's panel. It shows a unit's screen and nothing of the app
// around it, and a reader with no session is sent to a tab to sign in, because
// a provider sign-in refuses to run inside a frame.
vi.mock("@composition/extensions", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@composition/extensions")>()),
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
    notes: () => <Heading size="xlarge">Demo Notepad</Heading>,
  },
  extensionPanels: {},
  extensionApps: {},
}));

const refused = async (input: Request | string | URL) => {
  const url = String(input instanceof Request ? input.url : input);
  if (url.endsWith("/v1/auth/capabilities")) return Response.json({});
  return Response.json(
    { status: 401, code: "unauthorized" },
    { status: 401, headers: { "Content-Type": "application/problem+json" } },
  );
};

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(async () => {
  cleanup();
  await teardownApp();
  vi.unstubAllGlobals();
  history.replaceState(null, "", "/");
});

describe("a unit's screen on the embed path", () => {
  it("shows the unit's screen without the app's navigation", async () => {
    vi.stubGlobal("fetch", vi.fn(sessionOnlyFetch()));
    history.replaceState(null, "", "/embed/#/ext/notes");
    renderApp();
    expect(
      await screen.findByRole("heading", { name: "Demo Notepad" }),
    ).toBeTruthy();
    expect(screen.queryByRole("navigation")).toBeNull();
  });

  it("sends a reader with no session to a tab to sign in, never the sign-in form", async () => {
    vi.stubGlobal("fetch", vi.fn(refused));
    history.replaceState(null, "", "/embed/#/ext/notes");
    renderApp();
    expect(
      await screen.findByRole("heading", { name: "Sign in to Margince" }),
    ).toBeTruthy();
    const open = screen.getByRole("link", { name: /Open Margince/ });
    expect(open.getAttribute("href")).toBe(`${location.origin}/`);
    expect(open.getAttribute("target")).toBe("_blank");
    expect(screen.queryByLabelText(/password/i)).toBeNull();
  });

  it("keeps the sign-in form on the app's own path", async () => {
    vi.stubGlobal("fetch", vi.fn(refused));
    history.replaceState(null, "", "/#/ext/notes");
    renderApp();
    expect(
      (await screen.findAllByLabelText(/password/i)).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByRole("link", { name: /Open Margince/ })).toBeNull();
  });
});
