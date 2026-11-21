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
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { jsonResponse } from "../../testing/company.fixtures";
import { project360 } from "../../testing/projects.fixtures";
import { RecordShell } from "../../testing/recordshell.testkit";
import { ProjectScreen } from "./project360";
import { projectsBackend } from "./projects.testing";

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
