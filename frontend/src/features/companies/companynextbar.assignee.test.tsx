// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { meFixture } from "../../testing/mefixture";
import { CompanyNextBar } from "./companynextbar";

// The bar's task belongs to the account, so its line names who holds it: the
// reader, a colleague, or nobody yet.

type Company360 = components["schemas"]["Company360"];
type NextStep = NonNullable<Company360["next_steps"]>["data"][number];

const ME = meFixture().user.id;

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(step: NextStep) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(meFixture())),
  );
  const view: Company360 = {
    as_of: "2026-08-07T09:00:00Z",
    company: {
      id: "o-1",
      display_name: "Acme",
      source: "manual",
      captured_by: "human:test",
      created_at: "2026-08-01T09:00:00Z",
      updated_at: "2026-08-01T09:00:00Z",
    },
    sections_omitted: [],
    next_steps: { data: [step], page: { has_more: false } },
  };
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyNextBar view={view} onOpenTasks={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

// "You" depends on the session read, so a check that it is absent waits for it.
async function sessionRead(client: QueryClient) {
  await waitFor(() =>
    expect(client.getQueryState(["me"])?.status).toBe("success"),
  );
}

const task = (over: Partial<NextStep>): NextStep => ({
  activity_id: "t-1",
  subject: "Send the revised quote",
  overdue: false,
  ...over,
});

describe("the company's Next bar", () => {
  it("names a colleague who holds the task", async () => {
    await sessionRead(
      show(task({ assignee_id: "u-sam", assignee_name: "Sam Holder" })),
    );
    expect(screen.getByText("Sam Holder")).toBeInTheDocument();
    expect(screen.queryByText("You")).toBeNull();
  });

  it("names nobody when the server withholds the holder's name", async () => {
    await sessionRead(
      show(task({ assignee_id: "u-left", assignee_name: null })),
    );
    expect(screen.queryByText("u-left")).toBeNull();
    expect(screen.queryByText("You")).toBeNull();
    expect(screen.queryByText("Unassigned")).toBeNull();
  });

  it("says You when the reader holds it", async () => {
    show(task({ assignee_id: ME, assignee_name: "Test User" }));
    expect(await screen.findByText("You")).toBeInTheDocument();
  });

  it("says Unassigned when nobody holds it", () => {
    show(task({ assignee_id: null, assignee_name: null }));
    expect(screen.getByText("Unassigned")).toBeInTheDocument();
  });
});
