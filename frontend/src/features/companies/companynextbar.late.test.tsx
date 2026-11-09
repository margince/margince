/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import { installFetchStub, meRoute } from "../../testing/story-utils";
import { CompanyHowItStands } from "./companyhowitstands";
import { CompanyNextBar } from "./companynextbar";

afterEach(cleanup);

type Company360 = components["schemas"]["Company360"];

// Due 23:00 Berlin on the 11th, read at 10:00 Berlin on the 13th: 35 hours,
// so the server's elapsed count says one day while the calendar says two.
const VIEW: Company360 = {
  as_of: "2026-08-13T08:00:00Z",
  company: {
    id: "o-1",
    display_name: "Acme",
    source: "manual",
    captured_by: "human:test",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
  },
  sections_omitted: [],
  moment: {
    claim_key: "moment:account_promise_task:t-1",
    evidence_fingerprint: "fp-1",
    rule: "overdue_promise",
    headline: "You owe them: send the contract",
    why_now: "Due one day ago and still open.",
    confidence: "observed_fact",
    evidence: [{ type: "task", id: "t-1", label: "Send the contract" }],
    recommended_action: {
      kind: "complete_task",
      label: "Open the task",
      state: "available",
    },
  },
  next_steps: {
    data: [
      {
        activity_id: "t-1",
        subject: "Send the contract",
        due_at: "2026-08-11T21:00:00Z",
        overdue: true,
      },
    ],
    page: { has_more: false, next_cursor: null },
  },
};

describe("the Next bar's late promise", () => {
  it("says the same number of days in its badge and its sentence", () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider initial="en">
          <ViewerZoneProvider zone="Europe/Berlin">
            <CompanyNextBar view={VIEW} onOpenTasks={() => {}} />
          </ViewerZoneProvider>
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(screen.getByText("2 days late")).toBeTruthy();
    // The bar's line opens with who holds the task.
    expect(screen.getByText(/Due 2 days ago and still open\./)).toBeTruthy();
    expect(screen.queryByText("Due one day ago and still open.")).toBeNull();
  });

  it("gives How it stands the bar's sentence, not the server's", async () => {
    installFetchStub({ "GET /me": meRoute({}) });
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider initial="en">
          <ViewerZoneProvider zone="Europe/Berlin">
            <CompanyHowItStands
              companyId="o-1"
              // Health withheld, so the card's sentence is what is owed.
              view={{ ...VIEW, sections_omitted: ["health"] }}
              loading={false}
              failed={false}
            />
          </ViewerZoneProvider>
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(
      await screen.findByText("Due 2 days ago and still open."),
    ).toBeTruthy();
    expect(screen.queryByText("Due one day ago and still open.")).toBeNull();
    vi.unstubAllGlobals();
  });
});
