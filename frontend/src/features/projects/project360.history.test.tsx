// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { jsonResponse } from "../../testing/company.fixtures";
import { project360 } from "../../testing/projects.fixtures";
import { RecordShell } from "../../testing/recordshell.testkit";
import { ProjectScreen } from "./project360";
import { projectsBackend } from "./projects.testing";

type PhaseTransition = components["schemas"]["Project360PhaseTransition"];
type Commitments = components["schemas"]["Project360"]["commitments"];

const WINNER = {
  actor_type: "human",
  actor_id: "human:u-ada",
  actor_name: "Ada Winner",
} as const;

// A project's full history reads a stored value as the rest of the page does.
// An owner's id reads as the colleague's name, a calendar day as a date.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a project's change history", () => {
  it("names the colleagues and dates a change row stores", async () => {
    const before = "01a1e489-5d07-7cef-bd9c-8016f0e9bf54";
    const after = "01a1e489-5d12-7486-9f86-666ea4c6556f";
    const change = (field: string, from: string | null, to: string) => ({
      id: `f-${field}`,
      entity_type: "project",
      entity_id: "pr-1",
      field,
      old_value: from,
      new_value: to,
      changed_at: "2026-07-14T10:00:00Z",
      actor_type: "human",
      actor_id: "u-me",
    });
    projectsBackend({
      view: project360(),
      respond: async (url) => {
        const path = new URL(url).pathname;
        if (path === "/v1/users/names") {
          return jsonResponse({
            data: [
              { id: before, display_name: "Ada Owner" },
              { id: after, display_name: "Ben Owner" },
            ],
          });
        }
        if (path.endsWith("/field-history")) {
          return jsonResponse({
            data: [
              change("owner_id", before, after),
              change("target_end_date", null, "2026-01-01"),
            ],
            page: { next_cursor: null },
          });
        }
        return null;
      },
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: en["record.moreActions"] }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["record.fullHistory"] }),
    );
    const drawer = await screen.findByRole("dialog", {
      name: en["record.fullHistory"],
    });
    await user.click(within(drawer).getByRole("button", { name: "Changes" }));

    expect(await within(drawer).findByText("Ben Owner")).toBeTruthy();
    expect(within(drawer).getByText("Ada Owner")).toBeTruthy();
    expect(within(drawer).getByText("1 Jan 2026")).toBeTruthy();
    expect(within(drawer).queryByText(after)).toBeNull();
  });
});

// A phase the server moved because a deal was won names that cause, so the row
// does not read as a move someone chose.
describe("a phase move a won deal caused", () => {
  const opened: PhaseTransition = {
    id: "ph-1",
    from_phase: null,
    to_phase: "pursuing",
    reason: null,
    changed_at: "2026-06-01T09:00:00Z",
    changed_by: WINNER,
  };
  const moved: PhaseTransition = {
    id: "ph-2",
    from_phase: "pursuing",
    to_phase: "delivering",
    reason: null,
    changed_at: "2026-06-10T09:00:00Z",
    changed_by: WINNER,
  };

  function showHistory(visit: PhaseTransition) {
    projectsBackend({
      view: project360({
        phase_history: { data: [opened, visit], phase_durations: [] },
      }),
    });
    render(<ProjectScreen id="pr-1" />);
  }

  it("names the deal in place of the colleague who won it", async () => {
    showHistory({
      ...moved,
      cause: { kind: "deal_won", deal_id: "d-1", deal_name: "Wendt renewal" },
    });
    const link = await screen.findByRole("link", { name: "Wendt renewal" });
    expect(link.getAttribute("href")).toContain("#/deals/d-1");
    const sub = link.closest(".overviewrow-sub")?.textContent;
    expect(sub).toBe(
      `Since 10 Jun · ${en["project.history.dealWon"].replace("{name}", "Wendt renewal")}`,
    );
    expect(sub).not.toContain(WINNER.actor_name);
  });

  it("says a deal was won when the reader may not see which", async () => {
    showHistory({
      ...moved,
      cause: { kind: "deal_won", deal_id: null, deal_name: null },
    });
    expect(
      await screen.findByText(
        `Since 10 Jun · ${en["project.history.dealWonUnnamed"]}`,
      ),
    ).toBeTruthy();
  });

  it("credits the colleague and their reason for a move they chose", async () => {
    showHistory({ ...moved, reason: "Contract signed" });
    expect(
      await screen.findByText(
        `Since 10 Jun · ${en["project.history.movedBy"].replace("{subject}", WINNER.actor_name)} · Contract signed`,
      ),
    ).toBeTruthy();
  });

  it("credits the colleague for a cause this build has no words for", async () => {
    // A newer server's cause kind, which the generated types cannot name.
    const later = {
      ...moved,
      cause: { kind: "contract_signed", deal_id: null, deal_name: null },
    };
    projectsBackend({
      respond: async (url) =>
        new URL(url).pathname.endsWith("/360")
          ? jsonResponse({
              ...project360(),
              phase_history: { data: [opened, later], phase_durations: [] },
            })
          : null,
    });
    render(<ProjectScreen id="pr-1" />);
    expect(
      await screen.findByText(
        `Since 10 Jun · ${en["project.history.movedBy"].replace("{subject}", WINNER.actor_name)}`,
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/won$/)).toBeNull();
  });
});

describe("the All tasks drawer", () => {
  it("is titled All tasks, and the history drawer Full history", async () => {
    const commitments: Commitments = {
      data: [
        {
          activity_id: "a-1",
          subject: "Confirm the depot slot",
          due_at: null,
          due_has_time: false,
          assignee_id: null,
          assignee_name: null,
          version: 1,
          overdue: false,
        },
      ],
      page: { has_more: false },
    };
    projectsBackend({ view: project360({ commitments }) });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: en["project.commitments.all"] }),
    );
    expect(
      await screen.findByRole("dialog", {
        name: en["project.commitments.all"],
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("dialog", { name: en["record.fullHistory"] }),
    ).toBeNull();
  });
});
