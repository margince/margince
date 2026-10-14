// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ASYNC_UTIL_TIMEOUT_MS,
  SLOWEST_MEASURED_TEST_MS,
} from "../vitest.budget";
import { navigate } from "./lib/router";
import { formatFixture } from "./screens/import.fixtures";
import { memoryStorage, renderApp } from "./testing/appharness";
import { meFixture } from "./testing/mefixture";

// The run page's unsaved guard is the shell's own (App.tsx), which holds the
// address without its query; a guard wired in the test would prove only itself.
// The run is the state. A profiled file is a draft the server keeps, so only
// an upload still in flight is work the page could lose by leaving.

const SETTLE_MS = 10_000;

// Two settles, four default waiters and the suite's allowance for the work
// between them: the sum the leave-after-commit case may spend.
const LEAVE_AFTER_COMMIT_TEST_MS =
  SETTLE_MS * 2 + ASYNC_UTIL_TIMEOUT_MS * 4 + SLOWEST_MEASURED_TEST_MS;

const RUN_PAGE = "#/settings/import/run";

const run = {
  id: "019ff-run",
  connector: "csv",
  object: "lead",
  status: "awaiting_approval",
  checkpoint: 0,
  version: 1,
  source: "import_api",
  created_at: "2026-08-13T10:00:00Z",
  updated_at: "2026-08-13T10:00:00Z",
};

// The run's status as the server holds it: approving it lands it.
let status = "pending";

const profile = {
  run_id: "019ff-run",
  source_ref: "ws/import/abc",
  object: "lead",
  file_name: "estate.csv",
  rows_profiled: 1,
  row_count: 1,
  columns: [{ header: "Email", fill_rate: 1, samples: ["ada@x.test"] }],
  suggested_mapping: { Email: "email" },
  targets: ["full_name", "email"],
};

function answers(view: string | null): Readonly<Record<string, unknown>> {
  return {
    "GET /v1/me": meFixture({
      allow: { import_run: ["create", "update", "read"] },
    }),
    "GET /v1/imports/format": formatFixture,
    "POST /v1/imports/sources": profile,
    "POST /v1/imports": run,
    "GET /v1/imports/019ff-run": {
      ...run,
      status,
      draft_step: "files",
      profile,
    },
    "GET /v1/imports": {
      data: [{ ...run, status, profile }].filter(
        (one) => (one.status === "pending") === (view === "drafts"),
      ),
    },
    "PATCH /v1/imports/019ff-run": { ...run, status },
    "GET /v1/imports/019ff-run/report": {
      run_id: run.id,
      status,
      rows_read: 1,
      disposition: { created: 1, updated: 0, unchanged: 0, skipped: 0 },
      issues: [],
      source_key_used: "Email",
    },
  };
}

// Every read the shell makes besides these fails, so each screen falls to its
// own error state rather than to an answer nobody wrote.
async function importFetch(input: Request | string | URL, init?: RequestInit) {
  const request = input instanceof Request ? input : null;
  const url = new URL(request ? request.url : String(input), "https://t.test");
  const method = request?.method ?? init?.method ?? "GET";
  const key = `${method} ${url.pathname}`;
  const lands = key === "POST /v1/imports/019ff-run/approve";
  if (lands) status = "complete";
  if (key === "POST /v1/imports/019ff-run/validate") {
    status = "awaiting_approval";
  }
  const body =
    lands || key === "POST /v1/imports/019ff-run/validate"
      ? { ...run, status }
      : answers(url.searchParams.get("view"))[key];
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
  return screen.findByLabelText("Upload your data", {}, { timeout: SETTLE_MS });
}

async function profileAFile(user: ReturnType<typeof userEvent.setup>) {
  await user.upload(
    screen.getByLabelText("Upload your data"),
    new File(["Email\nada@x.test\n"], "estate.csv"),
  );
  await screen.findByRole("combobox", { name: "Record type for estate.csv" });
}

async function leaveForTheRow(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("link", { name: "Back to data import" }));
  await waitFor(() => expect(window.location.hash).toBe("#/settings/import"));
}

// Defined on the instance below, so the browser's own answer is put back after.
const browserLanguages = Object.getOwnPropertyDescriptor(
  globalThis.navigator,
  "languages",
);

beforeEach(() => {
  status = "pending";
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
  if (browserLanguages) {
    Object.defineProperty(globalThis.navigator, "languages", browserLanguages);
  } else {
    Reflect.deleteProperty(globalThis.navigator, "languages");
  }
});

describe("the CSV import page in the shell", () => {
  it(
    "lets the reader leave a profiled file and finds it waiting as a draft",
    async () => {
      const user = userEvent.setup();
      await arriveAtRunPage();
      await profileAFile(user);

      await leaveForTheRow(user);

      expect(
        await screen.findByRole(
          "link",
          { name: "Continue" },
          { timeout: SETTLE_MS },
        ),
      ).toBeInTheDocument();
      expect(screen.getByText("Draft")).toBeInTheDocument();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    },
    LEAVE_AFTER_COMMIT_TEST_MS,
  );

  it(
    "asks before leaving a file that is still uploading",
    async () => {
      const user = userEvent.setup();
      vi.stubGlobal(
        "fetch",
        vi.fn((input: Request | string | URL, init?: RequestInit) =>
          String(input).endsWith("/v1/imports/sources")
            ? new Promise<Response>(() => {})
            : importFetch(input, init),
        ),
      );
      await arriveAtRunPage();
      await user.upload(
        screen.getByLabelText("Upload your data"),
        new File(["Email\nada@x.test\n"], "estate.csv"),
      );
      expect(await screen.findByText("Reading file…")).toBeInTheDocument();

      await leaveForTheRow(user);

      expect(
        await screen.findByRole("dialog", { name: "Discard unsaved changes?" }),
      ).toBeInTheDocument();
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
        await screen.findByText("New import", {}, { timeout: SETTLE_MS }),
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
      await user.click(screen.getByRole("button", { name: /Next: map/ }));
      await user.click(screen.getByRole("button", { name: "Run test" }));
      await user.click(
        await screen.findByRole("button", { name: "Import 1 row" }),
      );
      await screen.findByText("Import complete", {}, { timeout: SETTLE_MS });

      await leaveForTheRow(user);

      // The hash has already moved, so the hub is one render away, not a
      // settle: a default waiter is enough.
      expect(await screen.findByText("New import")).toBeInTheDocument();
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    },
    LEAVE_AFTER_COMMIT_TEST_MS,
  );

  it(
    "returns to the run page on the browser's Forward after Back",
    async () => {
      window.location.hash = "#/settings/import";
      renderApp();
      await screen.findByText("New import", {}, { timeout: SETTLE_MS });
      navigate({ screen: "settings", id: "import", id2: "run" });
      await screen.findByLabelText("Upload your data");

      globalThis.history.back();
      await screen.findByText("New import", {}, { timeout: SETTLE_MS });
      globalThis.history.forward();

      expect(
        await screen.findByLabelText(
          "Upload your data",
          {},
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
      expect(screen.queryByText("New import")).not.toBeInTheDocument();
      expect(window.location.hash).toBe("#/settings/import/junk");
    },
    SETTLE_MS * 2,
  );
});
