/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanyProjectsPanel } from "./companyprojects";

type Company360 = components["schemas"]["Company360"];

afterEach(cleanup);

function draw(view: Company360 | undefined) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyProjectsPanel companyId="o-1" view={view} readOnly={false} />
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
