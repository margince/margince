/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { CompanyProjectsPanel } from "./companyprojects";

type Company360 = components["schemas"]["Company360"];

// Every case answers /me, so no render reaches for a server that is not there;
// a case about the create verb re-stubs with the grants it is about.
beforeEach(() => {
  stubServer({ project: ["read"] });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function draw(view: Company360 | undefined, readOnly = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyProjectsPanel companyId="o-1" view={view} readOnly={readOnly} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The panel's own state guard, read the way the deals section reads its own:
// while the 360 is still arriving, or where this reader may not see the
// projects, the panel says so: an absent list handed straight to
// CompanyProjects drew "No projects yet" with an Attach verb over a section
// that had not answered.
describe("the account's projects panel", () => {
  it("holds the loading state, not an empty plate, while the 360 is in flight", () => {
    draw(undefined);
    expect(screen.queryByRole("button", { name: "Attach project" })).toBeNull();
    expect(screen.queryByText("No projects yet")).toBeNull();
    // The panel is still named, so a reader can tell WHICH reading is on its
    // way.
    expect(
      screen.getByRole("heading", { level: 2, name: "Projects" }),
    ).toBeTruthy();
  });

  it("says the section is hidden when this reader's role withholds it", () => {
    draw({
      as_of: "2026-08-25T09:00:00Z",
      company: {
        id: "o-1",
        display_name: "Brandt Automotive GmbH",
        captured_by: "human:u1",
        source: "manual",
        version: 1,
        created_at: "2026-06-01T08:00:00Z",
        updated_at: "2026-08-01T08:00:00Z",
      },
      sections_omitted: ["projects"],
    });
    expect(screen.queryByRole("button", { name: "Attach project" })).toBeNull();
    expect(screen.queryByText("No projects yet")).toBeNull();
  });

  it("lists the account's live projects once the section answers", () => {
    draw({
      as_of: "2026-08-25T09:00:00Z",
      company: {
        id: "o-1",
        display_name: "Brandt Automotive GmbH",
        captured_by: "human:u1",
        source: "manual",
        version: 1,
        created_at: "2026-06-01T08:00:00Z",
        updated_at: "2026-08-01T08:00:00Z",
      },
      sections_omitted: [],
      projects: [
        {
          project_id: "pr-1",
          name: "Depot fit-out",
          key: "DEP-12",
          phase: "delivering",
          quiet: false,
        },
      ],
    });
    expect(screen.getByText("Depot fit-out")).toBeTruthy();
  });
});

const ANSWERED: Company360 = {
  as_of: "2026-08-25T09:00:00Z",
  company: {
    id: "o-1",
    display_name: "Brandt Automotive GmbH",
    captured_by: "human:u1",
    source: "manual",
    version: 1,
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  sections_omitted: [],
  projects: [],
  projects_page: { has_more: false, next_cursor: null },
};

// A workspace's own project column, as the custom-field catalog answers it.
const PROJECT_CODE: components["schemas"]["CustomField"] = {
  id: "cf-code",
  object: "project",
  label: "Cost centre",
  slug: "cost_centre",
  type: "text",
  status: "active",
  column_name: "cf_cost_centre",
  created_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

// /me with exactly `allow`, a POST /projects that records its body, and a
// project custom-field catalog carrying `custom`.
function stubServer(
  allow: GrantSpec,
  custom: ReadonlyArray<components["schemas"]["CustomField"]> = [],
) {
  const created: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/me")) {
        return Response.json({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: meFixture({ allow }).authorization,
        });
      }
      if (pathname.endsWith("/projects") && request.method === "POST") {
        created.push(await request.json());
        return Response.json(
          { id: "pr-9", name: "Depot fit-out" },
          {
            status: 201,
          },
        );
      }
      const data = pathname.endsWith("/custom-fields") ? custom : [];
      return Response.json({ data, page: { has_more: false } });
    }),
  );
  return created;
}

// A company page is where a rep already stands when the account needs a new
// delivery. The project is born with THIS company on it, so the form never
// asks which company: that is the one answer the rep could get wrong.
describe("starting a project from the account", () => {
  it("creates a project already linked to the company", async () => {
    const created = stubServer({ project: ["create", "read"] });
    const user = userEvent.setup();
    draw(ANSWERED);

    await user.click(
      await screen.findByRole("button", { name: "New project" }),
    );
    expect(screen.queryByLabelText(/Company/)).toBeNull();
    await user.type(screen.getByLabelText(/Project name/), "Depot fit-out");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() =>
      expect(created).toEqual([
        expect.objectContaining({ name: "Depot fit-out", company_id: "o-1" }),
      ]),
    );
  });

  // The projects screen's form and this one are one form, so a column the
  // workspace added to projects is asked here too.
  it("asks the workspace's project custom fields and sends them", async () => {
    const created = stubServer({ project: ["create", "read"] }, [PROJECT_CODE]);
    const user = userEvent.setup();
    draw(ANSWERED);

    await user.click(
      await screen.findByRole("button", { name: "New project" }),
    );
    await user.type(screen.getByLabelText(/Project name/), "Depot fit-out");
    await user.type(await screen.findByLabelText(/Cost centre/), "CC-7");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() =>
      expect(created).toEqual([
        expect.objectContaining({
          name: "Depot fit-out",
          company_id: "o-1",
          cf_cost_centre: "CC-7",
        }),
      ]),
    );
  });

  it("offers no create verb to a reader who may not create projects", async () => {
    stubServer({ project: ["read"] });
    draw(ANSWERED);

    expect(await screen.findByText("Attach project")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "New project" })).toBeNull();
  });

  it("offers no create verb on an archived account", async () => {
    stubServer({ project: ["create", "read"] });
    draw(ANSWERED, true);

    expect(await screen.findByText("Projects")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "New project" })).toBeNull();
  });
});
