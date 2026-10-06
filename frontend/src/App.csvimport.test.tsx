// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "./app/mefixture";
import { memoryStorage, renderApp } from "./testing/appharness";

// The run page's unsaved guard is the shell's own (App.tsx), which holds the
// address without its query; a guard wired in the test would prove only itself.

const SETTLE_MS = 10_000;
const RUN_PAGE = "#/settings/import/run";

const run = {
  id: "019ff-run",
  connector: "csv",
  object: "lead",
  status: "awaiting_approval",
  checkpoint: 0,
  source: "import_api",
  created_at: "2026-08-13T10:00:00Z",
  updated_at: "2026-08-13T10:00:00Z",
};

const answers: Readonly<Record<string, unknown>> = {
  "GET /v1/me": meFixture({
    allow: { import_run: ["create", "update", "read"] },
  }),
  "POST /v1/imports/sources": {
    source_ref: "ws/import/abc",
    object: "lead",
    rows_profiled: 1,
    columns: [{ header: "Email", fill_rate: 1, samples: ["ada@x.test"] }],
    suggested_mapping: { Email: "email" },
    targets: ["full_name", "email"],
  },
  "POST /v1/imports": run,
  "POST /v1/imports/019ff-run/approve": { ...run, status: "complete" },
  "GET /v1/imports/019ff-run/report": {
    run_id: run.id,
    status: "awaiting_approval",
    rows_read: 1,
    disposition: { created: 1, updated: 0, unchanged: 0, skipped: 0 },
    issues: [],
    source_key_used: "Email",
  },
};

// Every read the shell makes besides these fails, so each screen falls to its
// own error state rather than to an answer nobody wrote.
async function importFetch(input: Request | string | URL, init?: RequestInit) {
  const request = input instanceof Request ? input : null;
  const url = new URL(request ? request.url : String(input), "https://t.test");
  const method = request?.method ?? init?.method ?? "GET";
  const body = answers[`${method} ${url.pathname}`];
  return body === undefined
    ? new Response(JSON.stringify({ code: "unavailable" }), {
        status: 503,
        headers: { "Content-Type": "application/problem+json" },
      })
    : new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
}

async function arriveAtRunPage() {
  window.location.hash = RUN_PAGE;
  renderApp();
  return screen.findByRole(
    "button",
    { name: "Choose file" },
    { timeout: SETTLE_MS },
  );
}

async function profileAFile(user: ReturnType<typeof userEvent.setup>) {
  await user.upload(
    screen.getByLabelText("CSV file"),
    new File(["Email\nada@x.test\n"], "estate.csv"),
  );
  await screen.findByRole("row", { name: /Email/ });
}

async function leaveForTheRow(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("link", { name: "Back to Data import" }));
  await waitFor(() => expect(window.location.hash).toBe("#/settings/import"));
}

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  Object.defineProperty(globalThis.navigator, "languages", {
    value: ["en-US"],
    configurable: true,
  });
  vi.stubGlobal("fetch", vi.fn(importFetch));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("the CSV import page in the shell", () => {
  it(
    "asks before leaving a profiled file that was never imported",
    async () => {
      const user = userEvent.setup();
      await arriveAtRunPage();
      await profileAFile(user);

      await leaveForTheRow(user);

      expect(
        await screen.findByRole("dialog", { name: "Discard unsaved changes?" }),
      ).toBeInTheDocument();
      expect(screen.getByRole("row", { name: /Email/ })).toBeInTheDocument();
    },
    SETTLE_MS * 2,
  );

  it(
    "lets the reader leave before a file is chosen",
    async () => {
      const user = userEvent.setup();
      await arriveAtRunPage();

      await leaveForTheRow(user);

      expect(
        await screen.findByRole(
          "button",
          { name: "Start import" },
          { timeout: SETTLE_MS },
        ),
      ).toBeInTheDocument();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    },
    SETTLE_MS * 3,
  );

  it(
    "lets the reader leave once the import is committed",
    async () => {
      const user = userEvent.setup();
      await arriveAtRunPage();
      await profileAFile(user);
      await user.click(screen.getByRole("button", { name: "Preview import" }));
      await user.click(
        await screen.findByRole("button", { name: "Import 1 row" }),
      );
      await screen.findByText("Import complete");

      await leaveForTheRow(user);

      expect(
        await screen.findByRole(
          "button",
          { name: "Start import" },
          { timeout: SETTLE_MS },
        ),
      ).toBeInTheDocument();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    },
    SETTLE_MS * 3,
  );

  it(
    "returns to the run page on the browser's Forward after Back",
    async () => {
      const user = userEvent.setup();
      window.location.hash = "#/settings/import";
      renderApp();
      await user.click(
        await screen.findByRole(
          "button",
          { name: "Start import" },
          { timeout: SETTLE_MS },
        ),
      );
      await screen.findByRole("button", { name: "Choose file" });

      globalThis.history.back();
      await screen.findByRole(
        "button",
        { name: "Start import" },
        { timeout: SETTLE_MS },
      );
      globalThis.history.forward();

      expect(
        await screen.findByRole(
          "button",
          { name: "Choose file" },
          { timeout: SETTLE_MS },
        ),
      ).toBeInTheDocument();
      expect(window.location.hash).toBe(RUN_PAGE);
    },
    SETTLE_MS * 4,
  );

  it(
    "answers a segment the import page does not have as an unknown address",
    async () => {
      window.location.hash = "#/settings/import/junk";
      renderApp();

      expect(
        await screen.findByText(
          "Settings page not found",
          {},
          { timeout: SETTLE_MS },
        ),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Start import" }),
      ).not.toBeInTheDocument();
      expect(window.location.hash).toBe("#/settings/import/junk");
    },
    SETTLE_MS * 2,
  );
});
