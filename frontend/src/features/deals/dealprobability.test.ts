// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type Translator, translate } from "../../i18n";
import { advanceNote } from "./dealprobability";
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
