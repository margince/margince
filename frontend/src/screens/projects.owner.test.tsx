// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "../features/companies/company.fixtures";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { ProjectsScreen } from "./projects";
import { project } from "./projects.fixtures";
import { projectsBackend } from "./projects.testing";

// The New project dialog's Owner: who it offers, what it sends, and what it
// says when the colleague list did not load.

const ME = meFixture().user.id;

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const roster = {
  data: [
    {
      id: ME,
      display_name: "Test User",
      email: "me@example.test",
      is_agent: false,
    },
    {
      id: "u-42",
      display_name: "Jane Doe",
      email: "jane@example.test",
      is_agent: false,
    },
    {
      id: "agent-1",
      display_name: "Runner Bot",
      email: "bot@example.test",
      is_agent: true,
    },
  ],
  page: { has_more: false },
};

describe("the New project dialog's owner", () => {
  it("creates a project owned by the colleague picked as its owner", async () => {
    const user = userEvent.setup();
    let posted: { owner_id?: string | null } | null = null;
    projectsBackend({
      projects: [],
      respond: async (url, method, request) => {
        if (new URL(url).pathname === "/v1/users") {
          return jsonResponse(roster);
        }
        if (method === "POST" && url.endsWith("/projects")) {
          posted = JSON.parse(await request.text());
          return jsonResponse(project({ id: "pr-new" }), 201);
        }
        return null;
      },
    });
    render(<ProjectsScreen />);
    await user.click(await screen.findByTestId("new-record"));
    await user.type(screen.getByLabelText("Project name *"), "CRM rollout");
    await user.type(screen.getByLabelText("Company *"), "Brandt");
    await user.click(
      await screen.findByRole("button", { name: /Brandt Automotive/ }),
    );
    // The server refuses an agent seat as an owner, so none is offered.
    await user.click(screen.getByLabelText("Owner"));
    await screen.findByRole("option", { name: "Jane Doe" });
    expect(screen.queryByRole("option", { name: "Runner Bot" })).toBeNull();
    await user.click(screen.getByRole("option", { name: "Jane Doe" }));
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted?.owner_id).toBe("u-42"));
  });

  it("says the colleague list did not load instead of offering only the reader", async () => {
    const user = userEvent.setup();
    projectsBackend({
      projects: [],
      respond: async (url) =>
        new URL(url).pathname === "/v1/users"
          ? jsonResponse({ title: "unavailable" }, 503)
          : null,
    });
    render(<ProjectsScreen />);
    await user.click(await screen.findByTestId("new-record"));
    expect(await screen.findByText(en["owner.rosterFailed"])).toBeTruthy();
  });
});
