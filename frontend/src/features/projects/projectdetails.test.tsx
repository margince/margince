/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { RecordZoneProvider } from "../../lib/recordzone";
import { project, project360 } from "../../testing/projects.fixtures";
import { ProjectDetails, ProjectSubtitle } from "./projectdetails";

// The project's details column and the head's lines: About, Ownership, the
// empty fields folded behind one link, and the closing rule.

// The owner, the company and the grants each resolve through their own read.
// Every route answers the same empty page, so the project reads as shared
// with nobody.
beforeEach(() => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(
      new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(node: ReactNode, zone = UTC_ZONE) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone={zone}>{node}</RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function details(
  targetEnd: string | null,
  asOf = "2026-09-17T09:00:00Z",
  zone?: string,
) {
  return show(
    <ProjectDetails
      view={project360({
        as_of: asOf,
        project: project({ target_end_date: targetEnd }),
      })}
    />,
    zone,
  );
}

describe("the dates", () => {
  it("names the target end as a day, with no count beside it", () => {
    details("2026-09-20");
    expect(screen.getByText(en["project.aside.targetEnd"])).toBeTruthy();
    expect(screen.getByText("20 Sept 2026")).toBeTruthy();
    expect(screen.queryByText(/in \d+ days?/)).toBeNull();
  });

  it("folds a date nobody set behind the empty fields link", async () => {
    details(null);
    expect(screen.queryByText(en["project.aside.targetEnd"])).toBeNull();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: /Show \d+ empty fields?/ }));
    expect(screen.getByText(en["project.aside.targetEnd"])).toBeTruthy();
  });
});

describe("the details groups", () => {
  it("names the account, the phase, the dates and the owner", async () => {
    show(
      <ProjectDetails
        view={project360({
          project: project({
            target_end_date: "2026-09-20",
            started_at: "2026-03-01T00:00:00Z",
          }),
        })}
      />,
    );
    // Shared with nobody, read from the grants rather than assumed.
    expect(
      await screen.findByText(en["record.details.notShared"]),
    ).toBeTruthy();
    for (const key of [
      "record.details.about",
      "record.details.ownership",
      "project.company",
      "project.phaseLabel",
      "project.started",
      "project.owner",
      "record.details.visibleTo",
      "project.aside.closeNote",
    ] as const) {
      expect(screen.getByText(en[key]), key).toBeTruthy();
    }
  });

  it("says a project is unassigned rather than leaving the owner blank", () => {
    show(
      <ProjectDetails
        view={project360({ project: project({ owner_id: null }) })}
      />,
    );
    expect(screen.getByText(en["list.unowned"])).toBeTruthy();
  });

  it("names the account the grant withholds rather than dropping it", () => {
    show(
      <ProjectDetails
        view={project360({
          sections_omitted: ["company"],
          company: undefined,
          project: project({ company_id: "o-1" }),
        })}
      />,
    );
    expect(screen.getByTestId("project-company-withheld")).toBeTruthy();
  });

  it("drops the closing rule once the project is closed", () => {
    show(
      <ProjectDetails
        view={project360({ project: project({ phase: "closed" }) })}
      />,
    );
    expect(screen.queryByText(en["project.aside.closeNote"])).toBeNull();
  });
});

describe("the head's lines", () => {
  it("says how far the work has run once both dates are set", () => {
    show(
      <ProjectSubtitle
        view={project360({
          as_of: "2026-08-13T10:00:00Z",
          project: project({
            started_at: "2026-03-01",
            target_end_date: "2026-11-30",
          }),
        })}
      />,
      UTC_ZONE,
    );
    expect(screen.getByText("Day 166 of 275")).toBeTruthy();
  });

  it("draws no day line without dates", () => {
    show(<ProjectSubtitle view={project360()} />);
    expect(screen.queryByText(/^Day /)).toBeNull();
  });
});
