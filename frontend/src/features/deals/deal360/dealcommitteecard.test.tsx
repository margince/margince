/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider, translate } from "../../../i18n";
import { en } from "../../../i18n/en";
import { meFixture } from "../../../testing/mefixture";
import { DealCommitteeCard } from "./dealcommitteecard";

// The card draws the coverage read. Each seat shows its role and whether we
// are in touch, each gap gets a row, and one line counts both. Nothing is
// claimed that the read did not say. A gap is named by its rule, never by a
// role nobody measured.

type DealCoverage = components["schemas"]["DealCoverage"];

const DEAL = "01a02e25-a5ac-7099-8099-581cbf001a99";
const OWNER = "01a02be9-0000-7000-8000-00000000aa01";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function stubFetch(
  grants: readonly ("read" | "create")[] = ["read", "create"],
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
  it("names each seat with its role, and an engaged one as engaged", async () => {
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
      screen.getByText(translate("en", "co.roleLabel.economic_buyer")),
    ).toBeInTheDocument();
  });

  it("names a gap by the rule that found it, not by a role", async () => {
    stubFetch();
    draw(
      coverage({
        risks: [{ kind: "coverage_gap", summary: "no engaged champion" }],
      }),
    );
    expect(
      await screen.findByText(translate("en", "coverage.risk.coverage_gap")),
    ).toBeInTheDocument();
    expect(
      screen.getByText(translate("en", "deal.committee.gap.coverageGap")),
    ).toBeInTheDocument();
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
