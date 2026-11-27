// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { pretendReaderZone } from "../../testing/readerzone.testkit";
import { StoryProviders } from "../../testing/story-utils";
import { CompanyTaskRows } from "./companytaskrows";

// The screen reads the browser's zone here, so the suite names it.
beforeEach(() => pretendReaderZone(UTC_ZONE));

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

function renderRows(steps: NextStep[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyTaskRows view={view(steps)} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function rowOf(subject: string): HTMLElement {
  const row = screen.getByText(subject).closest<HTMLElement>(".co-taskrow");
  if (!row) {
    throw new Error(`no task row holds ${subject}`);
  }
  return row;
}

describe("CompanyTaskRows: where each task stands on the 360's own day", () => {
  it("counts a late task's days from as_of, and draws no due chip on an undated one", () => {
    renderRows([
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
      within(rowOf("Find a second contact")).queryByText("No due date"),
    ).toBeNull();
  });

  it("draws no tick for a reader who cannot write", () => {
    renderRows([step({ due_at: "2026-08-13T15:00:00Z" })]);

    expect(screen.getByText("Due today")).toBeTruthy();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("says the account has no open task rather than drawing no row", () => {
    const { container } = renderRows([]);

    expect(screen.getByText("No open tasks for this company.")).toBeTruthy();
    expect(container.querySelector(".co-taskrow")).toBeNull();
  });
});

describe("CompanyTaskRows: a promise our later mail may have kept", () => {
  const kept = step({
    may_be_done: {
      contact_id: "p-1",
      email_activity_id: "e-1",
      wrote_at: "2026-08-12T10:00:00Z",
      wrote_precision: "instant",
      claim_key: "moment:may_be_done:a-1",
      evidence_fingerprint: "fp-1",
    },
  });

  it("asks the Worklist's question where the list offers the tick", () => {
    render(
      <StoryProviders>
        <CompanyTaskRows view={view([kept])} renderTick={() => null} />
      </StoryProviders>,
    );
    const row = rowOf(kept.subject);
    expect(
      within(row).getByRole("button", { name: en["worklist.mayBeDone.done"] }),
    ).toBeTruthy();
    expect(
      within(row).getByRole("button", {
        name: en["worklist.mayBeDone.notYet"],
      }),
    ).toBeTruthy();
  });

  it("asks nothing in a list that offers no tick", () => {
    renderRows([kept]);
    expect(
      screen.queryByRole("button", { name: en["worklist.mayBeDone.done"] }),
    ).toBeNull();
  });
});

describe("CompanyTaskRows: the chip line Home's cards draw", () => {
  it("names the kind of work and the task's lead paragraph, not its details", () => {
    renderRows([
      step({
        task_type: "meet",
        body: "Dana agreed to look at the samples together.\n\nRoom 2, bring the binder.",
      }),
    ]);
    const row = rowOf("Send Dana the renewal paperwork");

    expect(within(row).getByText(en["task.type.meet"])).toBeTruthy();
    expect(
      within(row).getByText("Dana agreed to look at the samples together."),
    ).toBeTruthy();
    expect(within(row).queryByText(/Room 2/)).toBeNull();
  });
});
