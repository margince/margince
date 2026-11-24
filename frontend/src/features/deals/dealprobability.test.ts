// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type Translator, translate } from "../../i18n";
import { advanceNote, probabilityReason } from "./dealprobability";
import { stages } from "./deals.testkit";

const t: Translator = (key, params) => translate("en", key, params);

const proposal = stages[1];
const won = stages[2];
const lost = { ...won, id: "s4", name: "Lost", semantic: "lost" as const };

function setTo(percent: number, stage: string) {
  return {
    probability_set: {
      win_probability: percent,
      stage_name: stage,
      note: "",
    },
  };
}

describe("the note a stage move leaves", () => {
  it("names the stage a deal moved to, and the probability it now has", () => {
    expect(advanceNote(undefined, { toStage: proposal }, t, "en")).toBe(
      "Moved to Proposal",
    );
    expect(
      advanceNote(setTo(40, "Proposal"), { toStage: proposal }, t, "en"),
    ).toBe("Moved to Proposal · probability now 40%");
  });

  it("says a deal closed, and how", () => {
    expect(advanceNote(setTo(0, "Lost"), { toStage: lost }, t, "en")).toBe(
      "Closed as lost · probability 0%",
    );
    expect(advanceNote(undefined, { toStage: won }, t, "en")).toBe(
      "Closed as won",
    );
  });

  it("says a closed deal was reopened, and where", () => {
    expect(
      advanceNote(
        setTo(40, "Proposal"),
        { toStage: proposal, reopens: true },
        t,
        "en",
      ),
    ).toBe("Reopened in Proposal · probability 40%");
  });
});

type Chance = NonNullable<
  Parameters<typeof probabilityReason>[0]["win_chance"]
>;

function reasonOf(win: number, chance: Chance) {
  return probabilityReason(
    {
      status: "open",
      win_probability: win,
      win_probability_source: "stage",
      stage_win_probability: chance.typed,
      win_chance: chance,
    },
    { stageName: "Proposal", t, locale: "en", editable: false },
  );
}

const base: Chance = {
  typed: 60,
  in_use: 45,
  evidence_n: 40,
  won_share: 45,
  self_correcting: true,
  explanation: "",
};

describe("why a deal has its win chance", () => {
  it("names the typed figure, the closed deals, the learned quiet effect and the result", () => {
    expect(
      reasonOf(41, {
        ...base,
        effect_signal: "quiet",
        effect_days: 12,
        effect_learned: true,
        effect_sample: 31,
      }),
    ).toBe(
      "Typed at 60% on the stage Proposal. 40 deals from here have closed, 45% won. quiet for 12 days lowers it (learned from 31 closed deals). The win chance is 41%.",
    );
  });

  it("says the halving stands in while too few closed deals show the effect", () => {
    expect(
      reasonOf(22, {
        ...base,
        effect_signal: "quiet",
        effect_days: 12,
        effect_learned: false,
        effect_sample: 3,
      }),
    ).toContain("quiet for 12 days halves it, because too few closed deals");
  });

  it("says when learning from closed deals is switched off", () => {
    expect(reasonOf(60, { ...base, in_use: 60, self_correcting: false })).toBe(
      "Typed at 60% on the stage Proposal. Learning from closed deals is switched off. The win chance is 60%.",
    );
  });
});
