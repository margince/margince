// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatPercent } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";

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
    return t(labelFor(SOURCE_LABEL, source));
  }
  if (viewerId && setter === viewerId) {
    return t("deal.winProbability.source.you");
  }
  const name = deal.win_probability_set_by_name;
  return name
    ? t("deal.winProbability.source.colleague", { name })
    : t(labelFor(SOURCE_LABEL, source));
}

type ReasonDeal = Pick<
  Deal,
  | "status"
  | "win_probability"
  | "win_probability_source"
  | "stage_win_probability"
  | "win_probability_set_by_user_id"
  | "win_probability_set_by_name"
>;

type ReasonContext = Readonly<{
  stageName: string | undefined;
  t: Translator;
  locale: Locale;
  viewerId?: string;
  // Margince's own reason for lowering the figure, where the reader may read
  // one; without it the general rule stands in.
  recorded?: string | null;
  // Whether the reader may change the figure, so the reason says how.
  editable: boolean;
}>;

/**
 * Why a deal has its probability, in sentences. They name who or what set it,
 * the stage's own figure beside an override, and how to change it.
 */
export function probabilityReason(
  deal: ReasonDeal,
  context: ReasonContext,
): string | undefined {
  const { stageName, t } = context;
  if (deal.win_probability == null) {
    return undefined;
  }
  const stage = stageName
    ? t("deal.winProbability.reason.stageNamed", { name: stageName })
    : t("deal.winProbability.reason.stageUnnamed");
  if (deal.status === "won" || deal.status === "lost") {
    return t("deal.winProbability.reason.closed", { stage });
  }
  return openReason(deal, stage, context).filter(Boolean).join(" ");
}

// The sentences for an open deal, by where its figure comes from. A falsy
// entry is a sentence that does not apply.
function openReason(
  deal: ReasonDeal,
  stage: string,
  { t, locale, viewerId, recorded, editable }: ReasonContext,
): (string | false | undefined)[] {
  const stagePercent =
    deal.stage_win_probability == null
      ? undefined
      : formatPercent(deal.stage_win_probability / 100, locale);
  const override = editable && t("deal.winProbability.reason.override");
  switch (deal.win_probability_source) {
    case "margince":
      return [
        stagePercent
          ? t("deal.winProbability.reason.margince", {
              percent: stagePercent,
              stage,
            })
          : t("deal.winProbability.reason.marginceBare"),
        recorded || t("deal.winProbability.reason.marginceRule"),
        override,
      ];
    case "human":
      return [
        setterSentence(deal, t, viewerId),
        stagePercent &&
          t("deal.winProbability.reason.stageGives", {
            percent: stagePercent,
            stage,
          }),
        editable && t("deal.winProbability.reason.clear"),
      ];
    default:
      return [t("deal.winProbability.reason.stage", { stage }), override];
  }
}

function setterSentence(
  deal: ReasonDeal,
  t: Translator,
  viewerId: string | undefined,
): string {
  const setter = deal.win_probability_set_by_user_id;
  if (viewerId && setter === viewerId) {
    return t("deal.winProbability.reason.you");
  }
  const name = setter ? deal.win_probability_set_by_name : undefined;
  return name
    ? t("deal.winProbability.reason.colleague", { name })
    : t("deal.winProbability.reason.human");
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
