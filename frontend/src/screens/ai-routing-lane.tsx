// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { Badge, Button } from "../design-system/atoms";
import { PanelRow } from "../design-system/panel";
import { formatUsdPerMTok } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { useAiStatus } from "./ai-admin";
import { processingLabel } from "./ai-decision-labels";
import {
  inputOnlyLane,
  type ModelCatalogue,
  type ModelLane,
  unreadablePrice,
} from "./ai-models";

// The Model tiers card's rows: one per tier, the embedder, and the optional
// decision model. Apart from the card that holds them, because a row is a
// reading of one binding and knows nothing of the document around it.

type DecisionsBinding = components["schemas"]["AiDecisionsBinding"];

// The decision lane's name, as the routing document spells its key. Shown raw,
// like every tier name down the same column.
const DECISIONS = "decisions";

// One lane, read as a row: which lane, which vendor, which model, and what is
// wrong with that pairing. Editing opens a dialog rather than fields under the
// row, so the column of rows stays a reading of the whole ladder.
//
// The two pills are the whole reason a reader can be shown a binding without also
// being shown the key card and the price sheet. Both are joins, and both stay
// silent rather than guessing: a key list that has not arrived claims nothing, and
// an empty price sheet means the reader cannot read it rather than that nothing on
// this installation is priced.
export function LaneRow({
  name,
  lane,
  binding,
  catalogue,
  unkeyed,
  onEdit,
  testId,
  chip,
  facts,
}: Readonly<{
  name: string;
  lane: ModelLane;
  binding: { provider: string; model: string; base_url?: string };
  catalogue: ModelCatalogue;
  unkeyed: ReadonlySet<string> | null;
  onEdit: () => void;
  testId?: string;
  // A fact about the binding that only this lane states, beside the vendor.
  chip?: ReactNode;
  // The line under the row: how the lane is used and whether it is answering.
  facts?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <PanelRow>
      <div data-testid={testId ?? `ai-routing-tier-${name}`}>
        <div className="ai-lane">
          {/* The lane's own id, and under it what the lane is FOR. The id is
              the routing document's vocabulary and what an operator greps for;
              the gloss says which of `premium` and `frontier` is dearer. */}
          <span className="ai-lane-name">
            <span>{name}</span>
            {laneGloss(name, t) && (
              <span className="t-sub">{laneGloss(name, t)}</span>
            )}
          </span>
          <span className="ai-lane-binding">
            <Badge>{binding.provider}</Badge>
            <span className="ai-lane-model">{binding.model}</span>
            {/* WHERE the OpenAI-wire adapter is pointed: `openai_compatible`
                names a protocol, and every broker on it reads identically on
                this row without the host. */}
            {binding.base_url ? <span>{hostOf(binding.base_url)}</span> : null}
            {chip}
            {unkeyed?.has(binding.provider) && (
              <Badge tone="warning">{t("aiRouting.noKey")}</Badge>
            )}
            {isUnpriced(catalogue, binding.provider, binding.model, lane) ? (
              <Badge tone="warning">{t("aiRouting.unpriced")}</Badge>
            ) : (
              <span className="ai-lane-price t-sub">
                {priceLabel(
                  catalogue,
                  binding.provider,
                  binding.model,
                  lane,
                  locale,
                  t,
                )}
              </span>
            )}
          </span>
          {/* Never refused, even to a reader who may not save: the dialog
              shows the rest of the binding, and its Save carries the refusal. */}
          <span className="ai-lane-open">
            <Button onClick={onEdit}>{t("aiRouting.edit")}</Button>
          </span>
        </div>
        {facts}
      </div>
    </PanelRow>
  );
}

// The decision lane: the one lane a routing document may leave out. Absent, the
// row says so and offers to add one; bound, it is the same row as every other
// lane, with where the model processes text beside the vendor.
export function DecisionLaneRow({
  binding,
  catalogue,
  unkeyed,
  canManage,
  onEdit,
  facts,
}: Readonly<{
  binding: DecisionsBinding | undefined;
  catalogue: ModelCatalogue;
  unkeyed: ReadonlySet<string> | null;
  canManage: boolean;
  onEdit: () => void;
  facts?: ReactNode;
}>) {
  const t = useT();
  const processing = useDecisionProcessing(binding);
  if (!binding) {
    return (
      <PanelRow>
        <div data-testid="ai-routing-decisions" className="ai-lane">
          <span className="ai-lane-name">
            <span>{DECISIONS}</span>
            <span className="t-sub">{t("aiRouting.lane.decisions")}</span>
          </span>
          <span className="ai-lane-binding t-sub">
            {t("aiRouting.decisions.absent")}
          </span>
          <span className="ai-lane-open">
            <Button
              onClick={onEdit}
              reason={canManage ? undefined : t("aiRouting.adminOnly")}
            >
              {t("aiRouting.decisions.add")}
            </Button>
          </span>
        </div>
      </PanelRow>
    );
  }
  return (
    <LaneRow
      lane="decisions"
      name={DECISIONS}
      testId="ai-routing-decisions"
      binding={binding}
      catalogue={catalogue}
      unkeyed={unkeyed}
      onEdit={onEdit}
      chip={processing ? <Badge>{processing}</Badge> : null}
      facts={facts}
    />
  );
}

// Where the bound decision model processes text, as the server classified it.
//
// Read off the live status rather than derived here: which adapters are local is
// the server's registry, and a second copy on this side would be free to drift
// from it.
function useDecisionProcessing(
  binding: DecisionsBinding | undefined,
): string | null {
  const t = useT();
  const canDiagnose = useCan("ai_diagnostics", "read");
  const canBudget = useCan("ai_budget", "read");
  const status = useAiStatus(canDiagnose && canBudget);
  const candidate = status.data?.features.find(
    (f) => f.decision_candidate,
  )?.decision_candidate;
  if (
    !binding ||
    !candidate ||
    candidate.provider !== binding.provider ||
    candidate.model !== binding.model
  ) {
    return null;
  }
  return processingLabel(candidate.processing, t);
}

// Whether the price sheet can cost a call on this binding.
//
// An EMPTY sheet answers no. The reader who cannot read `ai_model_rate` gets an
// empty list from the catalogue hook by design, and marking every lane unpriced
// on the strength of that would report a fault in the installation where the
// truth is only that the sheet is not theirs.
//
// A row that EXISTS but carries a price nothing can parse counts as unpriced
// too. It is the same fact to a reader — this call cannot be costed — and
// treating it as priced left the row showing neither a figure nor the pill,
// which says nothing at all.
function isUnpriced(
  catalogue: ModelCatalogue,
  provider: string,
  model: string,
  lane: ModelLane,
): boolean {
  if (!catalogue || catalogue.length === 0) {
    return false;
  }
  const rate = catalogue.find(
    (r) => r.provider === provider && r.model_id === model && r.lane === lane,
  );
  if (!rate) {
    return true;
  }
  if (unreadablePrice(rate.input_per_mtok)) {
    return true;
  }
  return !inputOnlyLane(lane) && unreadablePrice(rate.output_per_mtok);
}

// The host part of a base URL, for a row that has room for the address but not
// for the whole endpoint. Falls back to the string as given: a value an
// operator typed that does not parse is still what this lane is pointed at, and
// hiding it would leave the row claiming a vendor with no address at all.
function hostOf(baseUrl: string): string {
  try {
    return new URL(baseUrl).host;
  } catch {
    return baseUrl;
  }
}

// What each lane in the ladder is FOR, in words rather than in its id.
//
// An explicit switch rather than a key built from the tier name: the message
// catalog's type is a closed union, and a runtime-composed key would compile as
// any old string and ship a typo. It also means a tier the task contract grows
// later renders with no gloss — correct, because nobody has written one, and a
// missing sentence is better than a guessed one.
function laneGloss(name: string, t: ReturnType<typeof useT>): string | null {
  switch (name) {
    case "local_small":
      return t("aiRouting.lane.local_small");
    case "cheap_cloud":
      return t("aiRouting.lane.cheap_cloud");
    case "premium":
      return t("aiRouting.lane.premium");
    case "frontier":
      return t("aiRouting.lane.frontier");
    case "local_large":
      return t("aiRouting.lane.local_large");
    case "embeddings":
      return t("aiRouting.lane.embeddings");
    case "decisions":
      return t("aiRouting.lane.decisions");
    default:
      return null;
  }
}

// This binding's price, short enough to sit on the row: what goes in, what comes
// out, per million tokens. Empty where the sheet cannot say — the `unpriced`
// pill is what a reader sees instead, and printing a zero here would be the one
// thing this product is careful never to say by accident.
function priceLabel(
  catalogue: ModelCatalogue,
  provider: string,
  model: string,
  lane: ModelLane,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  const rate = (catalogue ?? []).find(
    (r) => r.provider === provider && r.model_id === model && r.lane === lane,
  );
  if (!rate) {
    return "";
  }
  // A row the sheet cannot state a price for prints NOTHING rather than
  // reaching the formatter. `formatUsdPerMTok` hands the parsed number to
  // `Intl.NumberFormat`'s `minimumFractionDigits`, and NaN there throws a
  // RangeError — during render, on a card the whole settings page is composed
  // from. The picker's own hint guards the same way for the same reason.
  //
  // The output side is only asked about where it MEANS something: an
  // input-only lane's price is a single figure.
  if (unreadablePrice(rate.input_per_mtok)) {
    return "";
  }
  const input = formatUsdPerMTok(rate.input_per_mtok, locale);
  if (inputOnlyLane(lane)) {
    return t("aiAdmin.inputRate", { input });
  }
  if (unreadablePrice(rate.output_per_mtok)) {
    return "";
  }
  return t("aiAdmin.rates", {
    input,
    output: formatUsdPerMTok(rate.output_per_mtok, locale),
  });
}
