// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { UTC_ZONE } from "../../../format/timezone";
import { translate, translatePlural } from "../../../i18n";
import { deal, stages } from "../deals.testkit";
import { riskDim } from "./dealhowitstands";
import type { DealStandingInputs } from "./dealstanding";

type DealStatusCard = components["schemas"]["DealStatusCard"];
type DealCoverage = components["schemas"]["DealCoverage"];

// The Risk reading counts what the records flag. It never rates a deal, and a
// read it could not make is not "none found".

const AS_OF = "2026-05-30T09:00:00Z";
const t = (
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values);
const words = { t, locale: "en", zone: UTC_ZONE } as const;

function card(over: Partial<DealStatusCard> = {}): DealStatusCard {
  return {
    deal_id: "d1",
    story: { sentences: [] },
    generated_at: AS_OF,
    generated_by: "model",
    ...over,
  };
}

function coverage(risks: DealCoverage["risks"]): DealCoverage {
  return {
    deal_id: "d1",
    stakeholders: [],
    our_side: [],
    risks,
    sections_omitted: [],
  };
}

function read(over: Partial<DealStandingInputs>) {
  const inputs: DealStandingInputs = {
    deal: deal({ stage_id: "s2" }),
    stages,
    asOf: AS_OF,
    activities: [],
    offers: [],
    coverage: { coverage: coverage([]), withheld: false, pending: false },
    card: card(),
    ladder: { advancing: false, advanceRefused: false, onAdvance: () => {} },
    ...over,
  };
  return riskDim(inputs, words);
}

describe("the Risk reading", () => {
  it("counts a reply owed and each coverage finding, and names the first two", () => {
    const dim = read({
      card: card({ reply_to: "m1" }),
      coverage: {
        coverage: coverage([
          { kind: "going_cold", summary: "quiet" },
          { kind: "champion_left", summary: "left" },
        ]),
        withheld: false,
        pending: false,
      },
    });
    expect(dim.value).toBe(
      translatePlural("en", "deal.standing.risk.found", 3, { count: "3" }),
    );
    expect(dim.sub).toBe(
      `${t("deal.standing.risk.replyOwed")} · ${t("coverage.risk.going_cold")}`,
    );
    expect(dim.tone).toBe("warning");
  });

  it("says none found only once the card is read and somebody is named", () => {
    expect(
      read({
        coverage: {
          coverage: {
            ...coverage([]),
            stakeholders: [
              {
                contact_id: "c1",
                contact_name: "Mai",
                role: "champion",
                engaged: true,
              },
            ],
          },
          withheld: false,
          pending: false,
        },
      }).value,
    ).toBe(t("deal.standing.risk.none"));
  });

  it("does not call a deal nobody sits on all clear", () => {
    expect(read({}).value).toBe(t("standing.notAssessed"));
  });

  it("says it has not read the deal while the card is on its way", () => {
    const dim = read({ card: undefined });
    expect(dim.value).toBe(t("deal.standing.committee.unread"));
    expect(dim.tone).toBe("neutral");
  });

  it("says not shown when the coverage is withheld from the reader", () => {
    const dim = read({
      coverage: { coverage: undefined, withheld: true, pending: false },
    });
    expect(dim.value).toBe(t("record.notShown"));
  });
});
