// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import { jsonResponse } from "../../testing/company.fixtures";
import { meFixture } from "../../testing/mefixture";
import { project, project360 } from "../../testing/projects.fixtures";
import { RecordShell } from "../../testing/recordshell.testkit";
import { ProjectScreen } from "./project360";
import { projectsBackend } from "./projects.testing";

// The Next bar's Snooze confirms like every other move of a task: the new
// deadline, and an Undo that puts the old one back.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

type Write = { body: unknown; ifMatch: string | null };

const NEXT = {
  activity_id: "a-next",
  subject: "Confirm the depot slot",
  due_at: "2026-09-11T13:00:00.000Z",
  due_has_time: true,
  due_intent: { wall_clock: "2026-09-11T15:00", timezone: "Europe/Berlin" },
  version: 7,
  assignee_id: null,
  assignee_name: null,
  overdue: false,
};

function renderProject(): Write[] {
  const writes: Write[] = [];
  projectsBackend({
    view: project360({
      commitments: { data: [NEXT], page: { has_more: false } },
    }),
    respond: async (url, method, request) => {
      if (url.endsWith("/v1/me")) {
        return jsonResponse(
          meFixture({
            allow: {
              project: ["read", "update"],
              activity: ["read", "update"],
            },
          }),
        );
      }
      if (method === "PATCH" && url.includes("/v1/activities/a-next")) {
        writes.push({
          body: await request.clone().json(),
          ifMatch: request.headers.get("If-Match"),
        });
        return jsonResponse({ id: "a-next", version: 8 + writes.length });
      }
      return null;
    },
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ViewerZoneProvider zone="Europe/Berlin">
          <ToastProvider>
            <RecordShell>
              <ProjectScreen id={project().id} />
            </RecordShell>
            <ToastRegion />
          </ToastProvider>
        </ViewerZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return writes;
}

describe("the Next bar's Snooze", () => {
  it("confirms the new deadline with an Undo pinned on the version the snooze produced", async () => {
    vi.setSystemTime(new Date("2026-09-01T08:00:00Z"));
    const writes = renderProject();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /^Snooze/ }));
    expect(await screen.findByText("Moved to 12 Sept, 15:00")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes[0].body).toMatchObject({
      due_intent: { wall_clock: "2026-09-12T15:00" },
    });
    expect(writes[0].ifMatch).toContain("7");
    expect(writes[1].body).toEqual({
      due_at: NEXT.due_at,
      due_has_time: true,
      due_intent: NEXT.due_intent,
    });
    expect(writes[1].ifMatch).toContain("9");
  });
});
