/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { DataTasksHealthCard } from "./datataskshealth";
import { runningDataTask } from "./datataskshealth.fixtures";

function routes(
  data: unknown,
  allow: GrantSpec = { data_task: ["read", "update"] },
  status = 200,
  seat: "full" | "read" = "full",
) {
  const writes: unknown[] = [];
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const request = input instanceof Request ? input : new Request(input);
    if (request.url.endsWith("/me"))
      return jsonResponse(meFixture({ allow, seat }));
    if (request.method === "POST") {
      writes.push(await request.json());
      return new Response(null, { status: 204 });
    }
    return jsonResponse(data, status);
  });
  vi.stubGlobal("fetch", fetch);
  return { writes, fetch };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("distinguishes rows checked, changes and an unknown estimate", async () => {
  routes({ data: [runningDataTask] });
  render(<DataTasksHealthCard />);
  expect(await screen.findByText("Correct held mail")).toBeInTheDocument();
  expect(screen.getByText("Not estimated")).toBeInTheDocument();
  expect(screen.getByText("200")).toBeInTheDocument();
  expect(screen.getByText("12")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Pause" })).toBeInTheDocument();
});

it("sends the visible step and version when an operator pauses", async () => {
  const { writes } = routes({ data: [runningDataTask] });
  render(<DataTasksHealthCard />);
  fireEvent.click(await screen.findByRole("button", { name: "Pause" }));
  await waitFor(() =>
    expect(writes).toEqual([
      {
        task_id: runningDataTask.task_id,
        step: "classify",
        version: 4,
        action: "pause",
      },
    ]),
  );
});

it("renders a capped estimate as a lower bound", async () => {
  routes({
    data: [{ ...runningDataTask, estimate: 1000, estimate_capped: true }],
  });
  render(<DataTasksHealthCard />);
  expect(await screen.findByText("At least 1,000")).toBeInTheDocument();
});

it("keeps read-only progress visible without write controls", async () => {
  routes({ data: [runningDataTask] }, { data_task: ["read"] });
  render(<DataTasksHealthCard />);
  await screen.findByText("Correct held mail");
  expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Estimate rows" })).toBeNull();
});

it("does not fetch a report the reader cannot see", async () => {
  const { fetch } = routes({ data: [] }, {});
  render(<DataTasksHealthCard />);
  await screen.findByText(
    "Permission is required to read data updates. Ask an admin for access.",
  );
  expect(
    fetch.mock.calls.every(([input]) =>
      (input instanceof Request ? input.url : String(input)).endsWith("/me"),
    ),
  ).toBe(true);
});

it.each([500, 200])(
  "does not turn a failed or malformed report into an empty queue (%i)",
  async (status) => {
    routes(
      status === 500
        ? { code: "internal", message: "Unavailable" }
        : { data: [{ ...runningDataTask, scanned: null }] },
      undefined,
      status,
    );
    render(<DataTasksHealthCard />);
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.queryByText("No data updates are registered.")).toBeNull();
  },
);

it("shows a failed task and its retry without offering resume", async () => {
  routes({
    data: [
      {
        ...runningDataTask,
        status: "failed",
        blocked_reason: "failed",
        error_sentence: "The update stopped.",
        error_remedy: "Check the worker.",
      },
    ],
  });
  render(<DataTasksHealthCard />);
  expect(
    await screen.findByRole("button", { name: "Retry" }),
  ).toBeInTheDocument();
  expect(
    screen.getByText("The update stopped. Check the worker."),
  ).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
});

it("does not offer controls until the worker registers a version", async () => {
  routes({
    data: [
      {
        ...runningDataTask,
        status: "pending",
        version: 0,
        blocked_reason: "waiting_for_worker",
      },
    ],
  });
  render(<DataTasksHealthCard />);
  await screen.findByText("Waiting for a worker.");
  expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
});

it("refuses write controls on a read seat even with an update grant", async () => {
  routes(
    { data: [runningDataTask] },
    { data_task: ["read", "update"] },
    200,
    "read",
  );
  render(<DataTasksHealthCard />);
  await screen.findByText("Correct held mail");
  expect(screen.queryByRole("button", { name: "Pause" })).toBeNull();
});

it("keeps known rows visible when another row has a newer server state", async () => {
  routes({
    data: [
      runningDataTask,
      {
        ...runningDataTask,
        task_id: "newer-task",
        title: "Newer task",
        status: "new_state",
        blocked_reason: "new_reason",
      },
    ],
  });
  render(<DataTasksHealthCard />);
  await screen.findByText("Newer task");
  expect(
    screen.getAllByText("Update the application to manage this state."),
  ).toHaveLength(2);
  expect(screen.getAllByRole("button", { name: "Pause" })).toHaveLength(1);
});

it("refreshes a stale click and uses the new version without retaining the old conflict", async () => {
  let version = 4;
  const versions: number[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      if (request.url.endsWith("/me"))
        return jsonResponse(
          meFixture({ allow: { data_task: ["read", "update"] } }),
        );
      if (request.method === "POST") {
        const body: unknown = await request.json();
        if (
          !body ||
          typeof body !== "object" ||
          !("version" in body) ||
          typeof body.version !== "number"
        )
          throw new Error("Missing version");
        versions.push(body.version);
        version = 5;
        return jsonResponse(
          { code: "conflict", detail: "The task changed. Refresh the report." },
          409,
        );
      }
      return jsonResponse({ data: [{ ...runningDataTask, version }] });
    }),
  );
  render(<DataTasksHealthCard />);
  const pause = await screen.findByRole("button", { name: "Pause" });
  fireEvent.click(pause);
  await waitFor(() => {
    expect(versions).toEqual([4]);
    expect(pause).not.toHaveAttribute("aria-disabled", "true");
    expect(screen.queryByRole("alert")).toBeNull();
  });
  fireEvent.click(pause);
  await waitFor(() => expect(versions).toEqual([4, 5]));
});

it("keeps focus on the state control when pause becomes resume", async () => {
  let paused = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      if (request.url.endsWith("/me"))
        return jsonResponse(
          meFixture({ allow: { data_task: ["read", "update"] } }),
        );
      if (request.method === "POST") {
        paused = true;
        return new Response(null, { status: 204 });
      }
      return jsonResponse({
        data: [
          {
            ...runningDataTask,
            status: paused ? "paused" : "running",
            version: paused ? 5 : 4,
          },
        ],
      });
    }),
  );
  render(<DataTasksHealthCard />);
  const pause = await screen.findByRole("button", { name: "Pause" });
  pause.focus();
  fireEvent.click(pause);
  expect(await screen.findByRole("button", { name: "Resume" })).toHaveFocus();
});
