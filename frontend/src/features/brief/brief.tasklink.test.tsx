/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Home opens the task its address names, which is where a reminder
// notification sends the reader.

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { readingsDay, taskRow } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const TASK = {
  id: "reminded-task",
  kind: "task",
  writable: true,
  subject: "Call back about the renewal",
  body: "They asked for a call after the board meeting.",
  occurred_at: "2026-09-01T10:00:00Z",
  version: 1,
  is_done: false,
};

it("opens the task the address names, read-only while no row offers to finish it", async () => {
  window.location.hash = "#/home?task=reminded-task";
  stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [])),
    "GET /activities/reminded-task": () => jsonResponse(TASK),
  });
  render(<BriefScreen />);

  expect(await screen.findByText(TASK.body)).toBeTruthy();
  expect(
    within(screen.getByRole("dialog")).queryByRole("button", {
      name: en["tasks.complete"],
    }),
  ).toBeNull();
});

it("lets the reader finish the named task when their own row offers it", async () => {
  window.location.hash = "#/home?task=reminded-task";
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { activity: ["read", "update"] } })),
    "GET /worklist": () =>
      jsonResponse(readingsDay({}, [taskRow("reminded-task", TASK.subject)])),
    "GET /activities/reminded-task": () => jsonResponse(TASK),
  });
  render(<BriefScreen />);

  expect(
    await within(await screen.findByRole("dialog")).findByRole("button", {
      name: en["tasks.complete"],
    }),
  ).toBeTruthy();
});

it("says so when the named task cannot be read, and closing clears the address", async () => {
  const user = userEvent.setup();
  window.location.hash = "#/home?task=reminded-task";
  stubApi({
    "GET /worklist": () => jsonResponse(readingsDay({}, [])),
    "GET /activities/reminded-task": () =>
      jsonResponse({ title: "Not found", status: 404 }, 404),
  });
  render(<BriefScreen />);

  const dialog = await screen.findByRole("dialog");
  expect(await within(dialog).findByRole("alert")).toBeTruthy();
  expect(screen.queryByText(TASK.body)).toBeNull();

  await user.keyboard("{Escape}");
  await waitFor(() => expect(window.location.hash).not.toContain("task="));
});
