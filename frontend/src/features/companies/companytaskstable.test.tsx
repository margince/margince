// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { CompanyTaskTable } from "./companytaskstable";

type Company360 = components["schemas"]["Company360"];
type NextStep = NonNullable<Company360["next_steps"]>["data"][number];

afterEach(cleanup);

const AS_OF = "2026-08-13T09:00:00Z";

function step(overrides: Partial<NextStep>): NextStep {
  return {
    activity_id: "a-1",
    subject: "Send Dana the renewal paperwork",
    overdue: false,
    ...overrides,
  };
}

function view(steps: NextStep[]): Company360 {
  return {
    as_of: AS_OF,
    company: {
      id: "o-1",
      display_name: "Brandt Automotive GmbH",
      source: "manual",
      captured_by: "human:u-1",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-06-01T08:00:00Z",
    },
    sections_omitted: [],
    next_steps: { data: steps, page: { has_more: false, next_cursor: null } },
  };
}

function renderTable(steps: NextStep[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyTaskTable view={view(steps)} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function rowOf(subject: string): HTMLElement {
  const row = screen.getByText(subject).closest("tr");
  if (!row) {
    throw new Error(`no table row holds ${subject}`);
  }
  return row;
}

describe("CompanyTaskTable: where each task stands on the 360's own day", () => {
  it("counts a late task's days from as_of, and the days left on a dated one", () => {
    renderTable([
      step({
        activity_id: "a-1",
        subject: "Send Dana the renewal paperwork",
        due_at: "2026-07-25T12:00:00Z",
        overdue: true,
      }),
      step({
        activity_id: "a-2",
        subject: "Prepare the walkthrough brief",
        due_at: "2026-08-19T12:00:00Z",
      }),
      step({ activity_id: "a-3", subject: "Find a second contact" }),
    ]);

    expect(
      within(rowOf("Send Dana the renewal paperwork")).getByText(
        "19 days late",
      ),
    ).toBeTruthy();
    expect(
      within(rowOf("Prepare the walkthrough brief")).getByText("In 6 days"),
    ).toBeTruthy();
    expect(
      within(rowOf("Find a second contact")).getByText("No due date"),
    ).toBeTruthy();
  });

  it("names the table after its card and draws no tick for a reader who cannot write", () => {
    renderTable([step({ due_at: "2026-08-13T15:00:00Z" })]);

    expect(screen.getByRole("table", { name: "Tasks" })).toBeTruthy();
    expect(screen.getByText("Due today")).toBeTruthy();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("says the account has no open task rather than drawing an empty table", () => {
    renderTable([]);

    expect(screen.getByText("No open tasks for this company.")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });
});
