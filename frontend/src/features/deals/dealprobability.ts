// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatPercent } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Deal = components["schemas"]["Deal"];
type Source = NonNullable<Deal["win_probability_source"]>;

// The contract does not say who set a probability by hand, so the words do
// not name anybody.
const SOURCE_LABEL: Record<Source, MessageKey> = {
  stage: "deal.winProbability.source.stage",
  human: "deal.winProbability.source.human",
  margince: "deal.winProbability.source.margince",
};

export type ProbabilityWords = Readonly<{
  value: string;
  // Where the figure comes from, and the stage's own figure beside one set on
  // the deal. Absent from a server that does not say.
  source?: string;
  stage?: string;
}>;

/** An open deal's win probability in words, or undefined when none is known. */
export function probabilityWords(
  deal: Pick<
    Deal,
    "win_probability" | "win_probability_source" | "stage_win_probability"
  >,
  t: Translator,
  locale: Locale,
): ProbabilityWords | undefined {
  if (deal.win_probability == null) {
    return undefined;
  }
  const source = deal.win_probability_source;
  const overridden = source !== undefined && source !== "stage";
  return {
    value: formatPercent(deal.win_probability / 100, locale),
    source: source && t(SOURCE_LABEL[source]),
    stage:
      overridden && deal.stage_win_probability != null
        ? t("deal.winProbability.stageValue", {
            percent: formatPercent(deal.stage_win_probability / 100, locale),
          })
        : undefined,
  };
}

type Advanced = Pick<components["schemas"]["DealAdvanced"], "probability_set">;

/**
 * What a stage move did to the deal's probability, or undefined when it set
 * none. Written from the two fields; the answer's own `note` is English.
 */
export function probabilitySetNote(
  advanced: Advanced | undefined,
  t: Translator,
  locale: Locale,
): string | undefined {
  const set = advanced?.probability_set;
  return set
    ? t("deal.winProbability.setByStage", {
        percent: formatPercent(set.win_probability / 100, locale),
        stage: set.stage_name,
      })
    : undefined;
}
