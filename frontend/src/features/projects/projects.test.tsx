// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ABSENT } from "../../format/absent";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { jsonResponse } from "../../testing/company.fixtures";
import { openFilterValues } from "../../testing/listquery.testkit";
import { phoneRowOf, stubWindowWidth } from "../../testing/phonerow.testkit";
import { COMPANY, project } from "../../testing/projects.fixtures";
import { ProjectsScreen } from "./projects";
import { projectsBackend } from "./projects.testing";

// The projects list: rows from GET /projects, the phase dial sent to the
// server, the instructional first-run plate, and the create dialog's own
// refusal of a key the contract would refuse.

const FAR_COMPANY = { ...COMPANY, id: "o-77", display_name: "Wendt Logistik" };

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

describe("ProjectsScreen", () => {
  it("renders a row per project with its key, company, phase and owner", async () => {
    projectsBackend({
      projects: [
        project({ id: "pr-1", name: "CRM rollout", key: "ACME-CRM" }),
        project({
          id: "pr-2",
          name: "Fleet retrofit",
          key: null,
          phase: "delivering",
          owner_id: null,
          last_activity_at: null,
        }),
      ],
    });
    render(<ProjectsScreen />);
    expect(await screen.findByText("CRM rollout")).toBeTruthy();
    // An empty cell carries the same mark as an empty Company cell.
    const fleet = screen.getByRole("link", { name: /Fleet retrofit/ });
    expect(fleet.closest("tr")?.textContent).toContain(ABSENT);
    // The tab and the phase say one word for one fact.
    expect(screen.getAllByText("Delivering").length).toBeGreaterThan(1);
    expect(screen.queryByText("In delivery")).toBeNull();
    expect(screen.getByText("ACME-CRM")).toBeTruthy();
    expect(screen.getByText("Fleet retrofit")).toBeTruthy();
    expect(screen.getByText("Initiative")).toBeTruthy();
    expect(fleet.closest("tr")?.textContent).toContain("Delivering");
    expect(await screen.findAllByText("Brandt Automotive")).toHaveLength(2);
    expect(screen.getByText("Unassigned")).toBeTruthy();
    // The row is a link to the project's own page.
    expect(
      screen.getByRole("link", { name: /CRM rollout/ }).getAttribute("href"),
    ).toBe("/#/projects/pr-1");
  });

  it("narrows to one phase through the server", async () => {
    const user = userEvent.setup();
    const { urls } = projectsBackend({
      projects: [project({ id: "pr-1", name: "CRM rollout" })],
    });
    render(<ProjectsScreen />);
    await screen.findByText("CRM rollout");

    const values = await openFilterValues(user, "Phase");
    await user.click(within(values).getByRole("radio", { name: "Delivering" }));

    await waitFor(() =>
      expect(urls.some((url) => url.includes("phase=delivering"))).toBe(true),
    );
    // The list opens sorted by the timeline's clock, newest activity first.
    const firstRead = urls.find((url) => url.includes("/projects?"));
    expect(firstRead).toContain("sort=-last_activity_at");
  });

  it("offers a saved view as a tab and sends its filter when selected", async () => {
    const user = userEvent.setup();
    const { urls } = projectsBackend({
      projects: [project({ id: "pr-1", name: "CRM rollout" })],
      respond: async (url, method) =>
        method === "GET" && url.includes("/views")
          ? jsonResponse({
              data: [
                {
                  id: "v-1",
                  resource: "projects",
                  name: "In delivery, mine",
                  query: {
                    list: {
                      sort: "name",
                      filters: {
                        phase: "delivering",
                        owner_id: "u-1",
                        key: "ACME-CRM",
                      },
                    },
                  },
                  created_at: "2026-06-01T00:00:00Z",
                  updated_at: "2026-06-01T00:00:00Z",
                },
              ],
              page: { next_cursor: null },
            })
          : null,
    });
    render(<ProjectsScreen />);
    await screen.findByText("CRM rollout");

    await user.click(
      await screen.findByRole("button", { name: "In delivery, mine" }),
    );

    await waitFor(() => {
      const read = urls.find(
        (url) => url.includes("/projects?") && url.includes("phase=delivering"),
      );
      expect(read).toContain("owner_id=u-1");
      expect(read).toContain("key=ACME-CRM");
      expect(read).toContain("sort=name");
    });
  });

  it("keeps the table and its saved-view rail when the live list is empty but a view exists", async () => {
    const user = userEvent.setup();
    const { urls } = projectsBackend({
      projects: [],
      respond: async (url, method) =>
        method === "GET" && url.includes("/views")
          ? jsonResponse({
              data: [
                {
                  id: "v-2",
                  resource: "projects",
                  name: "Archived too",
                  query: { list: { includeArchived: true, filters: {} } },
                  created_at: "2026-06-01T00:00:00Z",
                  updated_at: "2026-06-01T00:00:00Z",
                },
              ],
              page: { next_cursor: null },
            })
          : null,
    });
    render(<ProjectsScreen />);
    await user.click(
      await screen.findByRole("button", { name: "Archived too" }),
    );
    expect(screen.queryByText(englishCopy["project.emptyTitle"])).toBeNull();
    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            url.includes("/projects?") && url.includes("include_archived=true"),
        ),
      ).toBe(true),
    );
  });

  it("shows the instructional plate on a first run", async () => {
    projectsBackend({ projects: [] });
    render(<ProjectsScreen />);
    expect(
      await screen.findByText(englishCopy["project.emptyTitle"]),
    ).toBeTruthy();
    expect(
      screen.getByText(/starts in the initiative phase during the deal/),
    ).toBeTruthy();
    // The plate has to explain what the key is FOR — that a subject carrying it
    // files the mail — not merely that a key exists. Matched on the promise
    // rather than on the bracket glyph, which the copy is free to reword.
    expect(
      screen.getByText(/filed under the project automatically/),
    ).toBeTruthy();
    expect(screen.getByTestId("new-record")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  // The table carries the page's name in its own header, so the shell prints
  // none on this screen — and the first-run plate replaces that table. Without a
  // heading of its own this arm left the route with no h1 at all, on the one
  // screen a reader most needs to be told where they are. Exactly one, at level
  // 1: the plate's own title reads a level below it.
  it("names the page at heading level on the first run too", async () => {
    projectsBackend({ projects: [] });
    render(<ProjectsScreen />);
    await screen.findByText(englishCopy["project.emptyTitle"]);
    expect(
      screen
        .getAllByRole("heading", { level: 1 })
        .map((one) => one.textContent),
    ).toEqual(["Projects"]);
  });

  // The same name, in the arm that draws the table — so creating the first
  // project does not rename the page under the reader who created it.
  it("names the page at heading level with rows too", async () => {
    projectsBackend({
      projects: [project({ id: "pr-1", name: "CRM rollout" })],
    });
    render(<ProjectsScreen />);
    await screen.findByText("CRM rollout");
    expect(
      screen
        .getAllByRole("heading", { level: 1 })
        .map((one) => one.textContent),
    ).toEqual(["Projects"]);
  });

  it("shows the server's key and posts the chosen key, phase and deal", async () => {
    const user = userEvent.setup();
    let posted: unknown = null;
    const previewed: string[] = [];
    projectsBackend({
      projects: [],
      respond: async (url, method, request) => {
        const read = new URL(url);
        if (method === "POST" && read.pathname === "/v1/projects") {
          posted = JSON.parse(await request.text());
          return jsonResponse(project({ id: "pr-new" }), 201);
        }
        // The suggestion is the server's: the form shows what it answers.
        if (read.pathname === "/v1/projects/key-preview") {
          previewed.push(read.searchParams.get("name") ?? "");
          return jsonResponse({ key: "CR" });
        }
        if (
          read.pathname === "/v1/deals" &&
          read.searchParams.get("company_id") === FAR_COMPANY.id &&
          read.searchParams.get("status") === "open"
        ) {
          return jsonResponse({
            data: [{ id: "d-1", name: "Wendt renewal", project_id: null }],
            page: { next_cursor: null, has_more: false },
          });
        }
        // The company sits past any first page: only a search reaches it.
        if (read.pathname === "/v1/companies" && read.searchParams.get("q")) {
          return jsonResponse({
            data: read.searchParams.get("q") === "Wendt" ? [FAR_COMPANY] : [],
            page: { next_cursor: null, has_more: false },
          });
        }
        return null;
      },
    });
    render(<ProjectsScreen />);
    await user.click(await screen.findByTestId("new-record"));
    await user.type(screen.getByLabelText("Project name *"), "CRM rollout");
    await screen.findByText(/Leave empty to use CR\./);
    expect(previewed).toContain("CRM rollout");
    await user.type(screen.getByLabelText("Key"), "crm");

    await user.type(screen.getByLabelText("Company *"), "Wendt");
    await user.click(
      await screen.findByRole("button", { name: /Wendt Logistik/ }),
    );
    // The pick is named by the candidate it came from, not by its bare id.
    expect(screen.getByText("Wendt Logistik")).toBeTruthy();
    expect(screen.queryByText(FAR_COMPANY.id)).toBeNull();
    // The form opens on the phase the server would give a project anyway.
    const phase = screen.getByRole("combobox", { name: /Starting phase/ });
    expect(phase.textContent).toContain("Initiative");
    await user.click(phase);
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "Pursuing",
      }),
    );
    await user.click(screen.getByRole("combobox", { name: "Deal" }));
    await user.click(
      await within(screen.getByRole("listbox")).findByRole("option", {
        name: "Wendt renewal",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toEqual({
      name: "CRM rollout",
      company_id: FAR_COMPANY.id,
      owner_id: null,
      description: null,
      started_at: null,
      target_end_date: null,
      key: "crm",
      phase: "pursuing",
      deal_id: "d-1",
      source: "manual",
    });
    expect(window.location.hash).toBe("#/projects/pr-new");
  });
});

describe("ProjectsScreen — the customer column", () => {
  it("says a hidden customer is hidden and a missing one is a dash", async () => {
    projectsBackend({
      projects: [
        project({
          id: "pr-1",
          name: "Hidden one",
          company_id: null,
          masked_fields: ["company_id"],
        }),
        project({ id: "pr-2", name: "Bare one", company_id: null }),
      ],
    });
    render(<ProjectsScreen />);
    await screen.findByText("Hidden one");
    expect(screen.getAllByTestId("project-company-withheld")).toHaveLength(1);
    expect(screen.getAllByTestId("project-company-none")).toHaveLength(1);
  });

  it("names a hidden customer on a phone row and draws nothing for a missing one", async () => {
    stubWindowWidth(390);
    projectsBackend({
      projects: [
        project({
          id: "pr-1",
          name: "Hidden one",
          company_id: null,
          masked_fields: ["company_id"],
        }),
      ],
    });
    render(<ProjectsScreen />);
    const row = await phoneRowOf("Hidden one");
    expect(row.caption).toEqual([englishCopy["state.withheld"]]);
  });
});

describe("ProjectsScreen — a row on a phone", () => {
  it("ends the name's line in the phase and names the customer under it", async () => {
    stubWindowWidth(390);
    projectsBackend({
      projects: [
        project({ id: "pr-1", name: "CRM rollout", phase: "delivering" }),
      ],
    });
    render(<ProjectsScreen />);
    await screen.findByText("Brandt Automotive");
    const row = await phoneRowOf("CRM rollout");
    expect(row.end).toEqual(["Delivering"]);
    expect(row.caption).toEqual(["Brandt Automotive"]);
    expect(
      screen.queryByRole("link", { name: "Brandt Automotive" }),
    ).toBeNull();
  });

  it("draws no line two for a project with no customer", async () => {
    stubWindowWidth(390);
    projectsBackend({
      projects: [
        project({ id: "pr-1", name: "CRM rollout", company_id: null }),
      ],
    });
    render(<ProjectsScreen />);
    const row = await phoneRowOf("CRM rollout");
    expect(row.caption).toEqual([]);
    expect(row.row).not.toHaveTextContent("—");
  });
});
