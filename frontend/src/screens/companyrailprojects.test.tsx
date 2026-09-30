// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { ProjectsSection } from "./companyrailprojects";

type Company360 = components["schemas"]["Company360"];
type Company = components["schemas"]["Company"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const COMPANY: Company = {
  writable: true,
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  owner_id: "u-1",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function noProjects(company: Company): Company360 {
  return {
    as_of: "2026-06-01T09:00:00Z",
    company,
    sections_omitted: [],
    projects: [],
    projects_page: { has_more: false, next_cursor: null },
  };
}

function stubMe(project: GrantSpec["project"] = ["create", "read"]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({
        user: { id: "u-1", display_name: "Mira Voss" },
        authorization: meFixture({ allow: { project } }).authorization,
      }),
    ),
  );
}

function draw(view: Company360, onTab = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ProjectsSection view={view} loading={false} onTab={onTab} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return onTab;
}

// An account with no projects offers exactly one verb under the empty line:
// "New project" creates one where the reader may, and "Add" leads to the tab
// that lists them everywhere else.
describe("the rail's empty projects section", () => {
  it("starts a project on the account when the reader may write it", async () => {
    stubMe();
    const user = userEvent.setup();
    const onTab = draw(noProjects(COMPANY));

    await user.click(
      await screen.findByRole("button", { name: "New project" }),
    );

    expect(screen.getByRole("heading", { name: "New project" })).toBeTruthy();
    expect(onTab).not.toHaveBeenCalled();
  });

  it("points to the tab instead on an account the reader may not change", async () => {
    stubMe();
    const user = userEvent.setup();
    const onTab = draw(
      noProjects({ ...COMPANY, archived_at: "2026-07-01T00:00:00Z" }),
    );

    await user.click(await screen.findByRole("button", { name: "Add" }));

    expect(onTab).toHaveBeenCalledWith("deals");
    expect(screen.queryByRole("button", { name: "New project" })).toBeNull();
  });

  it("points to the tab on a writable account when the reader may not create projects", async () => {
    stubMe(["read"]);
    const user = userEvent.setup();
    const onTab = draw(noProjects(COMPANY));

    await user.click(await screen.findByRole("button", { name: "Add" }));

    expect(onTab).toHaveBeenCalledWith("deals");
    expect(screen.queryByRole("button", { name: "New project" })).toBeNull();
  });
});
