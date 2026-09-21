/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { LeadReadings } from "./leadreadings";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";

// Whose judgement the score rests on.
//
// The `manual:` prefix says a human supplied a factor; it does not say which
// human, or how certain they claimed to be. A breakdown that stops there is an
// unattributed claim, and a rep deciding whether to trust the number cannot
// tell a verified figure from a colleague's estimate.

type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u-1",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-04T08:00:00Z",
};

// One manual factor and one machine factor, which is the pair that matters:
// the machine one is what would expose a mapper handing every row an author.
const explained = {
  score: 72,
  explained: true,
  current: {
    score: 72,
    score_computed: 72,
    raw_sum: 23,
    rounded_sum: 23,
    computed_at: "2026-06-04T08:00:00Z",
    factors: [
      { factor: "decision_maker_title", points: 15 },
      {
        factor: "manual:employees",
        points: 8,
        set_by: "u-7",
        signal_kind: "assumption",
        reason: "they list four offices on the site",
      },
    ],
  },
};

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

function withReadings(node: ReactNode) {
  stubWithSession(
    {
      "GET /leads/l-1/score": () => jsonResponse(explained),
      // EntityRef resolves a user against the workspace roster, so the author
      // has to BE somebody: an id with no name behind it renders as a raw
      // uuid, which would pass an assertion about attribution while showing a
      // reader nothing they can act on.
      "GET /users": () =>
        jsonResponse({
          data: [
            { id: "u-7", display_name: "Anna Weber", email: "anna@acme.test" },
          ],
          page: { has_more: false, next_cursor: null },
        }),
    },
    { lead: ["read", "update"] },
  );
  return render(<StoryProviders>{node}</StoryProviders>);
}

describe("the score's breakdown", () => {
  it("says which human supplied a manual factor, and how certain they were", async () => {
    withReadings(<LeadReadings lead={lead} />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Evidence" }),
    );

    expect(await screen.findByText(/Anna Weber/)).toBeTruthy();
    expect(
      screen.getByText(new RegExp(en["lead.factorKind.assumption"])),
    ).toBeTruthy();
    expect(screen.getByText(/they list four offices on the site/)).toBeTruthy();
  });

  it("hands a machine factor no author, rather than an empty one", async () => {
    withReadings(<LeadReadings lead={lead} />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Evidence" }),
    );

    // The row is found by its own term and then asked what it carries: a
    // qualifier under an auto-captured signal would read as an author nobody
    // named, which is a worse statement than saying nothing.
    const machine = (
      await screen.findByText(en["lead.factor.decision_maker_title"])
    ).closest(".factlist-row");
    expect(machine).toBeTruthy();
    expect(machine?.querySelector(".factlist-note")).toBeNull();
  });
});

// Every reading states its own absence and its own ending in words. The four
// slots were borrowing them: a create form's label over the company, a badge's
// lower-case word under the score, and the ladder word overwritten by the
// filing that followed it.
describe("what each reading says when the lead is not simply open", () => {
  it("keeps the ladder word as the reading and the ending under it", async () => {
    withReadings(<LeadReadings lead={{ ...lead, status: "promoted" }} />);

    const status = (await screen.findByText("Qualified")).closest(".stat-card");
    expect(status?.textContent).toContain("Archived");
  });

  // A merge leaves `status` where it stood, so the ending is the only thing
  // that says the lead is not still being worked.
  it("says a merged lead went somewhere, without claiming where", async () => {
    withReadings(<LeadReadings lead={{ ...lead, merged_into_id: "l-2" }} />);

    const status = (await screen.findByText("Merged")).closest(".stat-card");
    expect(status?.textContent).toContain("Into another lead");
  });

  it("says a lead has no company, in a reading's words and not a form's", async () => {
    withReadings(<LeadReadings lead={lead} />);

    const company = (await screen.findByText("Company")).closest(".stat-card");
    expect(company?.textContent).toContain("None");
    expect(company?.textContent).not.toContain("Not set");
  });

  it("writes the override as a line of its own, not as the header's badge", async () => {
    withReadings(
      <LeadReadings
        lead={{ ...lead, score_override_reason: "Strong buying signal" }}
      />,
    );

    expect(await screen.findByText("Set by hand")).toBeTruthy();
    expect(screen.queryByText("overridden")).toBeNull();
  });

  // "On time" is a verdict on a response nobody has sent. What is true is
  // that it is owed, and the deadline under it says the rest.
  it("calls an unanswered lead inside its target owed, not on time", async () => {
    withReadings(
      <LeadReadings
        lead={{
          ...lead,
          sla_state: "within_target",
          sla_deadline_at: "2026-06-05T08:00:00Z",
        }}
      />,
    );

    const response = (await screen.findByText("First response")).closest(
      ".stat-card",
    );
    expect(response?.textContent).toContain("Owed");
    expect(response?.textContent).toMatch(/Due /);
    expect(screen.queryByText("On time")).toBeNull();
  });
});
