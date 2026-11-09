// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatPercent } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Deal = components["schemas"]["Deal"];
type Source = NonNullable<Deal["win_probability_source"]>;

// A probability a colleague set names them where the deal says who it was;
// these are the words when it does not.
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

/**
 * An open deal's win probability in words, or undefined when none is known.
 * A probability the reader set reads as theirs, a colleague's names them.
 */
export function probabilityWords(
  deal: Pick<
    Deal,
    | "win_probability"
    | "win_probability_source"
    | "stage_win_probability"
    | "win_probability_set_by_user_id"
    | "win_probability_set_by_name"
  >,
  t: Translator,
  locale: Locale,
  viewerId?: string,
): ProbabilityWords | undefined {
  if (deal.win_probability == null) {
    return undefined;
  }
  const source = deal.win_probability_source;
  const overridden = source !== undefined && source !== "stage";
  return {
    value: formatPercent(deal.win_probability / 100, locale),
    source: source && sourceWords(deal, source, t, viewerId),
    stage:
      overridden && deal.stage_win_probability != null
        ? t("deal.winProbability.stageValue", {
            percent: formatPercent(deal.stage_win_probability / 100, locale),
          })
        : undefined,
  };
}

function sourceWords(
  deal: Pick<
    Deal,
    "win_probability_set_by_user_id" | "win_probability_set_by_name"
  >,
  source: Source,
  t: Translator,
  viewerId: string | undefined,
): string {
  const setter = deal.win_probability_set_by_user_id;
  if (source !== "human" || !setter) {
    return t(SOURCE_LABEL[source]);
  }
  if (viewerId && setter === viewerId) {
    return t("deal.winProbability.source.you");
  }
  const name = deal.win_probability_set_by_name;
  return name
    ? t("deal.winProbability.source.colleague", { name })
    : t(SOURCE_LABEL[source]);
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
