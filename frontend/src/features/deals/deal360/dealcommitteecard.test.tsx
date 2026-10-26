/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider, translate } from "../../../i18n";
import { en } from "../../../i18n/en";
import { meFixture } from "../../../testing/mefixture";
import { DealCommitteeCard } from "./dealcommitteecard";

// The card draws our team from the deal's responsibilities and their team
// from the coverage read. Each seat shows its role and whether we are in
// touch, each gap names the seat it misses, and one line counts their side.
// Nothing is claimed that a read did not say.

type DealCoverage = components["schemas"]["DealCoverage"];

const DEAL = "01a02e25-a5ac-7099-8099-581cbf001a99";
const OWNER = "01a02be9-0000-7000-8000-00000000aa01";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

type RecordAssignment = components["schemas"]["RecordAssignment"];

// One responsibility as the assignments read returns it.
function held(
  id: string,
  name: string,
  role: string,
  kind: RecordAssignment["subject_kind"] = "user",
): RecordAssignment {
  return {
    id: `a-${id}`,
    record_type: "deal",
    record_id: DEAL,
    subject_kind: kind,
    subject_id: id,
    subject_name: name,
    subject_inactive: false,
    role_id: `r-${role}`,
    role_key: role.toLowerCase().replaceAll(" ", "_"),
    role_label: role,
    role_active: true,
    version: 1,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  };
}

function stubFetch(
  grants: readonly ("read" | "create")[] = ["read", "create"],
  team: readonly RecordAssignment[] = [],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.url.endsWith("/v1/me")) {
        return json(meFixture({ allow: { relationship: grants } }));
      }
      if (request.url.includes("/users/names")) {
        return json({ data: [{ id: OWNER, display_name: "Lena Fischer" }] });
      }
      if (request.url.includes(`/records/deal/${DEAL}/assignments`)) {
        return json({ data: team });
      }
      return json({ data: [], page: { next_cursor: null } });
    }),
  );
}

const coverage = (over: Partial<DealCoverage> = {}): DealCoverage => ({
  deal_id: DEAL,
  stakeholders: [
    {
      contact_id: "c-1",
      contact_name: "Mai Trần",
      role: "champion",
      engaged: true,
    },
    {
      contact_id: "c-2",
      contact_name: "Bảo Nguyễn",
      role: "economic_buyer",
      engaged: false,
    },
  ],
  our_side: [],
  risks: [],
  sections_omitted: [],
  ...over,
});

function draw(
  view: DealCoverage | undefined,
  state: Partial<{ withheld: boolean; pending: boolean }> = {},
) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <DealCommitteeCard
          dealId={DEAL}
          ownerId={OWNER}
          coverage={view}
          withheld={state.withheld ?? false}
          pending={state.pending ?? false}
          roleLabel={(role) => ROLE_WORDS[role] ?? role}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const ROLE_WORDS: Record<string, string> = {
  champion: en["co.roleLabel.champion"],
  economic_buyer: en["co.roleLabel.economic_buyer"],
  blocker: en["co.roleLabel.blocker"],
  influencer: en["co.roleLabel.influencer"],
  user: en["co.roleLabel.user"],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the deal's committee card", () => {
  it("names each seat with its role and whether we are in touch", async () => {
    stubFetch();
    draw(coverage());
    expect(await screen.findByText("Mai Trần")).toBeInTheDocument();
    expect(
      screen.getByText(
        translate("en", "deal.committee.seatSub", {
          role: translate("en", "co.roleLabel.champion"),
          engagement: translate("en", "deal.committee.legendEngaged"),
        }),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        translate("en", "deal.committee.seatSub", {
          role: translate("en", "co.roleLabel.economic_buyer"),
          engagement: translate("en", "deal.committee.legendQuiet"),
        }),
      ),
    ).toBeInTheDocument();
  });

  it("names the missing champion and opens the stakeholder form on that role", async () => {
    stubFetch();
    const user = userEvent.setup();
    draw(
      coverage({
        risks: [{ kind: "coverage_gap", summary: "no engaged champion" }],
      }),
    );
    const gap = await screen.findByText(
      translate("en", "deal.committee.missing.champion"),
    );
    const verbs = await screen.findAllByRole("button", {
      name: translate("en", "rel.addStakeholder"),
    });
    const forGap = verbs.find(
      (verb) => verb.getAttribute("aria-describedby") === gap.id,
    );
    expect(forGap).toBeDefined();
    if (forGap) {
      await user.click(forGap);
    }
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByRole("textbox", {
        name: translate("en", "rel.role"),
      }),
    ).toHaveValue("champion");
  });

  // Six colleagues hold jobs on the deal and one handles most exchanges. The
  // card lists all six and marks the one. The finding's words claim no headcount.
  it("lists six colleagues and marks the one the finding names", async () => {
    const six = [
      held(OWNER, "Lena Fischer", "Sales owner"),
      held("u-2", "Tomás Reyes", "Delivery owner"),
      held("u-3", "Ana Lima", "Solution engineer"),
      held("u-4", "Kai Brandt", "Legal"),
      held("u-5", "Mia Chen", "Finance"),
      held("u-6", "Noah Weber", "Executive sponsor"),
    ];
    stubFetch(["read", "create"], six);
    draw(
      coverage({
        risks: [
          {
            kind: "single_threaded_ours",
            summary: "one colleague carries almost all the contact",
            user_ids: ["u-3"],
          },
        ],
      }),
    );
    const ours = within(
      await screen.findByRole("region", {
        name: translate("en", "deal.committee.ourTeam"),
      }),
    );
    await ours.findByText("Noah Weber");
    expect(ours.getAllByRole("listitem")).toHaveLength(6);
    expect(ours.getByText("Owner, Sales owner")).toBeInTheDocument();
    const badge = ours.getByText(
      translate("en", "deal.committee.carriesContact"),
    );
    expect(badge.closest("li")).toHaveTextContent("Ana Lima");
    for (const words of [
      translate("en", "coverage.risk.single_threaded_ours"),
      document.body.textContent ?? "",
    ]) {
      expect(words).not.toMatch(/only one colleague|covered by one colleague/i);
    }
  });

  it("says the rest of our team could not be read rather than listing the owner as all of it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.endsWith("/v1/me")) {
          return json(meFixture({ allow: { relationship: ["read"] } }));
        }
        if (request.url.includes("/users/names")) {
          return json({ data: [{ id: OWNER, display_name: "Lena Fischer" }] });
        }
        if (request.url.includes("/assignments")) {
          return json({ title: "Forbidden", status: 403 }, 403);
        }
        return json({ data: [], page: { next_cursor: null } });
      }),
    );
    draw(coverage());
    expect(
      await screen.findByText(translate("en", "deal.committee.teamUnread")),
    ).toBeInTheDocument();
    expect(screen.getByText("Lena Fischer")).toBeInTheDocument();
  });

  it("counts who is named and engaged in the card's foot, in warning ink while a gap stands", async () => {
    stubFetch();
    draw(
      coverage({
        risks: [{ kind: "single_threaded_theirs", summary: "one thread" }],
      }),
    );
    const foot = await screen.findByText(
      translate("en", "deal.committee.foot", { named: "2", engaged: "1" }),
    );
    expect(foot).toHaveClass("overviewcard-foot-warn");
  });

  it("puts our owner on our side of the picture", async () => {
    stubFetch();
    draw(coverage());
    expect(await screen.findByText("Lena Fischer")).toBeInTheDocument();
  });

  it("says nobody is named once the read has answered with no one", async () => {
    stubFetch();
    draw(coverage({ stakeholders: [] }));
    expect(
      await screen.findByText(translate("en", "deal.standing.committee.none")),
    ).toBeInTheDocument();
  });

  it("says the seats are withheld rather than that nobody is named", async () => {
    stubFetch();
    draw(coverage({ stakeholders: [] }), { withheld: true });
    await screen.findByText(translate("en", "deal.committee.card"));
    expect(
      screen.queryByText(translate("en", "deal.standing.committee.none")),
    ).toBeNull();
  });

  it("offers no way to add a seat to a reader whose role holds no grant", async () => {
    stubFetch(["read"]);
    draw(coverage());
    await screen.findByText("Mai Trần");
    expect(screen.queryByTestId("add-relationship")).toBeNull();
  });
});
