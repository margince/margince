/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { LeadScreen, terminalBadge } from "./leads";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";

// What a merged-away lead's page says it is.
//
// The merge archives the loser, points it at the survivor and leaves the
// ladder status untouched — so the page has one fact and one fact only to
// read this ending from, and reading it from anything else says a different
// thing about a real prospect.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

// Merged away MID CONVERSATION, which is the case the ladder cannot describe:
// `contacted` with a first response already sent, archived, and pointed at the
// lead it turned out to be.
const mergedAway = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted" as const,
  score: 72,
  captured_by: "human:u-1",
  source: "manual",
  writable: false,
  version: 2,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
  first_response_at: "2026-06-02T09:00:00Z",
  merged_into_id: "l-2",
  archived_at: "2026-06-20T08:00:00Z",
};

const survivor = {
  ...mergedAway,
  id: "l-2",
  full_name: "Jonas Pedersen",
  version: 1,
  first_response_at: null,
  merged_into_id: null,
  archived_at: null,
  writable: true,
};

function stub(): void {
  stubWithSession(
    {
      "GET /leads/l-1": () => jsonResponse(mergedAway),
      "GET /leads/l-2": () => jsonResponse(survivor),
    },
    { lead: ["read", "update"], activity: ["read"] },
  );
}

describe("a merged-away lead's page", () => {
  it("says the lead was merged and names the lead it was merged into", async () => {
    stub();
    render(
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>,
    );

    expect(await screen.findByText(en["lead.mergedTitle"])).toBeTruthy();
    // The pointer is the useful half: the survivor by NAME, as a link, because
    // a reader who learns the lead was merged wants the one it went into.
    const link = await screen.findByRole("link", { name: /Jonas Pedersen/ });
    expect(link.getAttribute("href")).toContain("l-2");
  });

  it("does not read as open work, which its ladder status still says it is", async () => {
    stub();
    render(
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>,
    );

    expect(await screen.findByText(en["lead.standing.merged"])).toBeTruthy();
    // `contacted` with a first response is "their move" on a live lead. Drawn
    // here it would send a rep back to a prospect somebody else already owns.
    expect(screen.queryByText(en["lead.standing.theirMove"])).toBeNull();
  });
});

describe("the terminal badge a merged-away lead wears", () => {
  // The badge is what every lead SURFACE reads — the list row, the readings
  // card, the header — so the pointer has to decide it there too. Read from
  // the ladder alone this lead is `contacted` and wears nothing at all.
  it("says merged, on a lead whose ladder never left the conversation", () => {
    expect(
      terminalBadge({ status: "contacted", merged_into_id: "l-2" }),
    ).toEqual({ label: "lead.merged", tone: "warn" });
    expect(
      terminalBadge({ status: "contacted", merged_into_id: null }),
    ).toBeNull();
  });
});
