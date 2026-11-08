/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { dueInstant } from "../format/calendarday";
import { dueTimeOf } from "../format/timezone";
import { ViewerZoneProvider } from "../lib/viewerzone";
import { renderWithQueryClient } from "../testing/queryrender";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { TaskVerb } from "./bulktask";

// East of UTC and of the fallback zone, so a writer that used the viewer's
// zone or the fallback lands on another instant.
const RECORD_ZONE = "Asia/Bangkok";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

// A bulk task has no time field, so it is due by the end of its day on the
// RECORD's clock, the one rule every task screen reads. The viewer's zone, or
// the day's last millisecond in it, would show a stray hour on the record.
it("dates a bulk task at the end of the picked day on the picker's clock", async () => {
  installFetchStub({ "GET /users": () => jsonResponse({ data: [] }) });
  const ready = vi.fn<(task: components["schemas"]["BulkTask"]) => void>();
  const user = userEvent.setup();
  renderWithQueryClient(
    <ViewerZoneProvider zone={RECORD_ZONE}>
      <TaskVerb disabled={false} onReady={ready} />
    </ViewerZoneProvider>,
  );
  await user.click(screen.getByRole("button", { name: "Create task" }));
  await user.type(
    screen.getByRole("textbox", { name: "What has to be done" }),
    "Call back",
  );
  await user.type(
    screen.getByRole("textbox", { name: "Due date" }),
    "15/09/2026{Enter}",
  );
  await user.click(screen.getByRole("button", { name: "Preview" }));

  expect(ready).toHaveBeenCalledTimes(1);
  const dueAt = ready.mock.calls[0]?.[0].due_at ?? "";
  expect(dueAt).toBe(dueInstant("2026-09-15", RECORD_ZONE));
  expect(dueTimeOf(dueAt, false, RECORD_ZONE)).toBe("");
});
