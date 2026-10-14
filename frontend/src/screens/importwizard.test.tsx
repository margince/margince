/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SEARCH_DEBOUNCE_MS } from "../design-system/debouncedsearch";
import { ImportCard } from "./import";
import {
  jsonResponse,
  profile,
  render,
  renderAtImport,
  run,
  stubRoutes,
} from "./import.testkit";

const RUN_PAGE = "#/settings/import/run";
const RUN = `/imports/${run.id}`;

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.location.hash = "";
});

// One CSV dropped on the run page, and the Map step open.
async function toTheMap(user: UserEvent) {
  globalThis.location.hash = RUN_PAGE;
  render(<ImportCard subpage="run" />);
  await user.upload(
    await screen.findByLabelText("Upload your data"),
    new File(["Email\na@x.test\n"], "leads.csv"),
  );
  await user.click(await screen.findByRole("button", { name: /Next: map/ }));
  await screen.findByRole("row", { name: /Notes/ });
}

async function clearFullName(user: UserEvent) {
  await user.click(
    screen.getByRole("combobox", { name: "Field for Full Name" }),
  );
  await user.click(screen.getByRole("option", { name: "Do not import" }));
}

// A fetch that holds every PATCH until released, so a test can see whether a
// second save waits for the first.
function holdPatches() {
  const inner = globalThis.fetch;
  const started: string[] = [];
  let release: () => void = () => undefined;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      if ((request?.method ?? init?.method) === "PATCH") {
        started.push(String(request?.url ?? input));
        await gate;
      }
      return inner(input, init);
    }),
  );
  return { started, release };
}

describe("the wizard is the run", () => {
  it("reload at Map shows the saved step and the file's type", async () => {
    stubRoutes({
      [`GET ${RUN}`]: () =>
        jsonResponse({
          ...run,
          status: "pending",
          draft_step: "map",
          profile,
        }),
    });
    renderAtImport(run.id);
    expect(await screen.findByRole("row", { name: /Notes/ })).toBeVisible();
    expect(screen.getByRole("button", { name: "Run test" })).toBeEnabled();
  });

  it("reload at Files shows the files step, not the map", async () => {
    stubRoutes({
      [`GET ${RUN}`]: () =>
        jsonResponse({
          ...run,
          status: "pending",
          draft_step: "files",
          profile,
        }),
    });
    renderAtImport(run.id);
    expect(await screen.findByLabelText("Source name")).toBeVisible();
  });

  it("two quick edits make no conflict", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      let version = run.version;
      const sent = stubRoutes({
        [`PATCH ${RUN}`]: () =>
          jsonResponse({ ...run, status: "pending", version: ++version }),
      });
      await toTheMap(user);
      const held = holdPatches();
      await clearFullName(user);
      await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
      await waitFor(() => expect(held.started).toHaveLength(1));
      await user.click(
        screen.getByRole("combobox", { name: "Field for Email" }),
      );
      await user.click(screen.getByRole("option", { name: "Do not import" }));
      await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2);
      expect(held.started).toHaveLength(1);
      held.release();
      await waitFor(() =>
        expect(sent.filter((s) => s.method === "PATCH")).toHaveLength(2),
      );
      const versions = sent
        .filter((s) => s.method === "PATCH")
        .map((s) => (s.body as { version: number }).version);
      expect(versions[1]).toBeGreaterThan(versions[0]);
      expect(screen.queryByText(/changed in another tab/)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("Test run right after an edit saves that edit and validates its version", async () => {
    const user = userEvent.setup();
    let version = run.version;
    const sent = stubRoutes({
      [`PATCH ${RUN}`]: () =>
        jsonResponse({ ...run, status: "pending", version: ++version }),
    });
    await toTheMap(user);
    await clearFullName(user);
    await user.click(screen.getByRole("button", { name: "Run test" }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(`POST ${RUN}/validate`),
    );
    const writes = sent.filter((s) => s.method !== "GET");
    const patch = writes.findLast((s) => s.method === "PATCH");
    expect(patch?.body).toMatchObject({
      object: "lead",
      mapping: { Email: "email" },
      draft_step: "map",
    });
    const validate = writes.findLast((s) => s.path === `POST ${RUN}/validate`);
    expect(validate?.body).toEqual({ version });
  });

  it("a version conflict from another tab refetches and says so", async () => {
    const user = userEvent.setup();
    let reads = 0;
    stubRoutes({
      [`PATCH ${RUN}`]: () =>
        jsonResponse(
          { status: 409, code: "conflict", detail: "Changed." },
          409,
        ),
    });
    await toTheMap(user);
    const before = vi
      .mocked(fetch)
      .mock.calls.filter(([i]) =>
        String((i as Request).url).includes(RUN),
      ).length;
    await clearFullName(user);
    expect(await screen.findByText(/changed in another tab/)).toBeVisible();
    await waitFor(() => {
      reads = vi
        .mocked(fetch)
        .mock.calls.filter(([i]) =>
          String((i as Request).url).includes(RUN),
        ).length;
      expect(reads).toBeGreaterThan(before + 1);
    });
  });

  it("a superseded run offers the newer one", async () => {
    stubRoutes({
      [`GET ${RUN}`]: () =>
        jsonResponse({
          ...run,
          status: "cancelled",
          superseded_by: "019ff-newer",
        }),
    });
    renderAtImport(run.id);
    expect(
      await screen.findByText("A newer test run replaced this import"),
    ).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Open the newer test run" }),
    ).toHaveAttribute("href", expect.stringContaining("run=019ff-newer"));
  });

  it("Cancel on a draft cancels it and returns to the hub", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [`POST ${RUN}/cancel`]: () =>
        jsonResponse({ ...run, status: "cancelled" }),
    });
    globalThis.location.hash = RUN_PAGE;
    render(<ImportCard subpage="run" />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\na@x.test\n"], "leads.csv"),
    );
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(`POST ${RUN}/cancel`),
    );
    await waitFor(() => expect(globalThis.location.hash).not.toContain("run="));
  });
});
