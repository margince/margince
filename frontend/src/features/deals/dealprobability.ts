// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatAnalyticsNumber, formatPercent } from "../../format/format";
import { type Locale, type Translator, translatePlural } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];
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
  | "win_chance"
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
  const stageGives =
    stagePercent &&
    t("deal.winProbability.reason.stageGives", {
      percent: stagePercent,
      stage,
    });
  switch (deal.win_probability_source) {
    case "margince":
      return [
        t("deal.winProbability.reason.margince"),
        recorded || t("deal.winProbability.reason.marginceRule"),
        stageGives,
        override,
      ];
    case "human":
      return [
        setterSentence(deal, t, viewerId),
        stageGives,
        editable && t("deal.winProbability.reason.clear"),
      ];
    default:
      return deal.win_chance
        ? [...winChanceSentences(deal, stage, t, locale), override]
        : [t("deal.winProbability.reason.stage", { stage }), override];
  }
}

const SIGNAL_WORDS: Record<string, MessageKey> = {
  quiet: "deal.winChance.signal.quiet",
  close_pushed: "deal.winChance.signal.close_pushed",
  no_next_step: "deal.winChance.signal.no_next_step",
  stuck: "deal.winChance.signal.stuck",
};

// How the one win chance was made, from the structured parts the server sends.
// They are the typed stage figure, what closed deals from the stage show, the
// risk effect, and the result.
function winChanceSentences(
  deal: ReasonDeal,
  stage: string,
  t: Translator,
  locale: Locale,
): (string | false | undefined)[] {
  const chance = deal.win_chance;
  if (!chance || deal.win_probability == null) {
    return [];
  }
  const percent = (value: number) => formatPercent(value / 100, locale);
  let learned: string | false = false;
  if (!chance.self_correcting) {
    learned = t("deal.winChance.learningOff");
  } else if (chance.evidence_n && chance.won_share != null) {
    learned = translatePlural(
      locale,
      "deal.winChance.evidence",
      chance.evidence_n,
      {
        count: formatAnalyticsNumber(chance.evidence_n, locale),
        percent: percent(chance.won_share),
      },
    );
  }
  return [
    t("deal.winChance.typed", { stage, percent: percent(chance.typed) }),
    learned,
    effectSentence(chance, deal.win_probability, t, locale),
    t("deal.winChance.result", { percent: percent(deal.win_probability) }),
  ];
}

function effectSentence(
  chance: NonNullable<Deal["win_chance"]>,
  result: number,
  t: Translator,
  locale: Locale,
): string | false {
  const signalKey = chance.effect_signal && SIGNAL_WORDS[chance.effect_signal];
  if (!signalKey) {
    return false;
  }
  const signal = t(signalKey, {
    days: formatAnalyticsNumber(chance.effect_days ?? 0, locale),
  });
  if (!chance.effect_learned) {
    return t("deal.winChance.effect.halved", { signal });
  }
  const count = chance.effect_sample ?? 0;
  const params = { signal, count: formatAnalyticsNumber(count, locale) };
  return result > chance.in_use
    ? translatePlural(locale, "deal.winChance.effect.raises", count, params)
    : translatePlural(locale, "deal.winChance.effect.lowers", count, params);
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

// Each kind of move with and without the probability its stage set.
const ADVANCE_NOTE: Record<
  "moved" | "won" | "lost" | "reopened",
  readonly [bare: MessageKey, withProbability: MessageKey]
> = {
  moved: ["deals.advanced", "deals.advancedProbability"],
  won: ["deals.closedWon", "deals.closedWonProbability"],
  lost: ["deals.closedLost", "deals.closedLostProbability"],
  reopened: ["deals.reopened", "deals.reopenedProbability"],
};

function moveKind(
  move: Readonly<{ toStage: Stage; reopens?: boolean }>,
): keyof typeof ADVANCE_NOTE {
  if (move.reopens) {
    return "reopened";
  }
  return move.toStage.semantic === "open" ? "moved" : move.toStage.semantic;
}

/**
 * What a stage move did: moved, closed or reopened, and the probability the
 * new stage set when it set one. Written from the fields; the answer's own
 * `note` is English.
 */
export function advanceNote(
  advanced: Advanced | undefined,
  move: Readonly<{ toStage: Stage; reopens?: boolean }>,
  t: Translator,
  locale: Locale,
): string {
  const [bare, withProbability] = ADVANCE_NOTE[moveKind(move)];
  const set = advanced?.probability_set;
  return set
    ? t(withProbability, {
        stage: move.toStage.name,
        percent: formatPercent(set.win_probability / 100, locale),
      })
    : t(bare, { stage: move.toStage.name });
}
