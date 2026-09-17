// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RecordZoneProvider } from "../app/recordzone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ProjectIdentityFacts } from "./projectheaderfacts";
import { project, project360 } from "./projects.fixtures";

// The head's facts strip: which account the work is for, when it is due and
// how far off that is, and whose project it is.
//
// The target-end cell carries the two readings a running identity line could
// not: the count is in the RECORD's calendar rather than UTC's, and its wording
// comes from the reader's plural rule rather than from a plural "s" the
// catalogue spells once.

// The owner and the company each resolve through their own read (the roster,
// EntityRef's name lookup). Nothing here asserts a resolved NAME, so every
// route answers the same empty, honest page.
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
  vi.useRealTimers();
});

function show(node: ReactNode, zone = "UTC") {
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

function facts(targetEnd: string | null, zone?: string) {
  return show(
    <ProjectIdentityFacts
      view={project360({ project: project({ target_end_date: targetEnd }) })}
      locale="en"
    />,
    zone,
  );
}

describe("the target-end cell says the day and how far off it is", () => {
  // A fixed now, so the count under test is the code's reckoning and not the
  // day this suite happens to run on.
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-17T09:00:00Z"));
  });

  it("names the day and counts the mornings to it", () => {
    facts("2026-09-20");
    expect(screen.getByText(en["project.targetEnd"])).toBeTruthy();
    expect(screen.getByText(/20\/09\/2026 · in 3 days/)).toBeTruthy();
  });

  it("says one day in the singular rather than 'in 1 days'", () => {
    // The wording comes from the locale's own plural rule. A catalogue that
    // spells the plural once reads correctly for every count but the one a
    // reader sees on the day before a deadline.
    facts("2026-09-18");
    expect(screen.getByText(/in 1 day$/)).toBeTruthy();
  });

  it("counts a day past the date in the singular too", () => {
    facts("2026-09-16");
    expect(screen.getByText(/1 day past the date$/)).toBeTruthy();
  });

  it("counts the days past a date nobody met", () => {
    facts("2026-09-10");
    expect(screen.getByText(/7 days past the date$/)).toBeTruthy();
  });

  // The date and the count are two halves of one line, so they have to name
  // the same day. Counted in UTC while the date renders in the record's zone,
  // an installation thirteen hours ahead read a target of the 20th as three
  // mornings away on a morning that was already the 18th there.
  it("counts in the zone the date beside it is drawn in", () => {
    vi.setSystemTime(new Date("2026-09-17T21:00:00Z"));
    facts("2026-09-20", "Pacific/Auckland");
    expect(screen.getByText(/20\/09\/2026 · in 2 days/)).toBeTruthy();
  });

  // The cell stays where no date is set: when the work is due is a question
  // the reader came with, and a cell that vanishes answers it with silence.
  it("keeps the cell and says so when nobody has set a date", () => {
    facts(null);
    expect(screen.getByText(en["project.targetEnd"])).toBeTruthy();
    expect(screen.getByText(en["field.unset"])).toBeTruthy();
  });
});

describe("the strip answers the three facts a reader arrives with", () => {
  it("names the account, the date and the owner as cells", () => {
    facts("2026-09-20");
    for (const label of ["project.company", "project.targetEnd"] as const) {
      expect(screen.getByText(en[label]), label).toBeTruthy();
    }
    expect(screen.getByText(en["project.owner"])).toBeTruthy();
  });

  it("says a project is unassigned rather than leaving the owner blank", () => {
    // An empty value here reads as a rendering fault. "Unassigned" is a fact
    // about the project, and it is the one somebody acts on.
    show(
      <ProjectIdentityFacts
        view={project360({ project: project({ owner_id: null }) })}
        locale="en"
      />,
    );
    expect(screen.getByText(en["list.unowned"])).toBeTruthy();
  });

  // A project drawn with no company reads as a project that HAS none, which is
  // a different and wrong statement about a record the grant simply withholds.
  it("names the account cell the grant withholds rather than dropping it", () => {
    show(
      <ProjectIdentityFacts
        view={project360({
          sections_omitted: ["company"],
          company: undefined,
          project: project({ company_id: null }),
        })}
        locale="en"
      />,
    );
    expect(screen.getByText(en["project.company"])).toBeTruthy();
    expect(screen.getByTestId("project-company-withheld")).toBeTruthy();
  });
});
