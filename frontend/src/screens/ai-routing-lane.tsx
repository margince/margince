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
import {
  AdapterFields,
  DECISION_PROVIDERS,
  OPENROUTER_DECISION_PRESET,
} from "./ai-routing-fields";

// The routing card's lane rows: one per tier, the embedder, and the optional
// decision model. Apart from the form that holds them, because a row is a
// reading of one binding and knows nothing of the document around it.

type Routing = components["schemas"]["AiRouting"];
type DecisionsBinding = components["schemas"]["AiDecisionsBinding"];

// The decision lane's name, as the routing document spells its key. Shown raw,
// like every tier name down the same column.
const DECISIONS = "decisions";

// One lane, read as a row and edited in place.
//
// The row is the READING — which lane, which vendor, which model, and what is
// wrong with that pairing — and the fields open under it only when a reader asks
// to change one. The card was six lanes' worth of fields opened at once, which is
// three screens of controls to answer the question "where does premium go".
//
// The two pills are the whole reason a reader can be shown a binding without also
// being shown the key card and the price sheet. Both are joins, and both stay
// silent rather than guessing: a key list that has not arrived claims nothing, and
// an empty price sheet means the reader cannot read it rather than that nothing on
// this installation is priced.
export function LaneRow<
  B extends { provider: string; model: string; base_url?: string },
>({
  name,
  lane,
  binding,
  catalogue,
  unkeyed,
  disabled,
  open,
  onOpen,
  onChange,
  extra,
  testId,
  providers,
  chip,
}: Readonly<{
  name: string;
  lane: ModelLane;
  binding: B;
  catalogue: ModelCatalogue;
  unkeyed: ReadonlySet<string> | null;
  disabled: boolean;
  open: boolean;
  onOpen: () => void;
  onChange: (next: B) => void;
  // A control this lane has and the others do not — the embedding width, the
  // decision model's Remove. It rides in the opened body rather than in
  // `AdapterFields`, which asks the one question every lane answers.
  extra?: ReactNode;
  testId?: string;
  // The adapters this lane may name, where they are not the chat list.
  providers?: readonly string[];
  // A fact about the binding that only this lane states, beside the vendor.
  chip?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <PanelRow>
      {/* The lane's addressable region: the summary line AND the fields it
          opens, so "the premium lane" names one thing whether it is folded or
          not. Inside the row rather than on it, because `PanelRow` owns the
          row's own geometry and takes no attributes of its own. */}
      <div data-testid={testId ?? `ai-routing-tier-${name}`}>
        <div className="ai-lane">
          {/* The lane's own id, and under it what the lane is FOR.
              The id stays because it is the routing document's vocabulary and
              what an operator greps for; the gloss is there because `premium`
              and `frontier` do not say which is dearer, and `local_small` says
              nothing at all to somebody meeting this page for the first time.
              A lane this build does not know gets no gloss rather than an
              invented one. */}
          <span className="ai-lane-name">
            <span>{name}</span>
            {laneGloss(name, t) && (
              <span className="t-sub">{laneGloss(name, t)}</span>
            )}
          </span>
          {/* The binding itself, as ONE flex item. Grouped rather than laid
              out beside the name as five siblings, because a row that wraps
              wraps at whatever item runs out of room — which put the Change
              button alone on a second line while the model beside it still had
              space. Wrapping now happens INSIDE this group, and the two things
              that anchor the row keep their edges. */}
          <span className="ai-lane-binding">
            {/* Filled, not `quiet`. Quiet draws a status DOT, and the vendor is
                not a status — the pills that follow it are, and a dot in front
                of the vendor would put three status marks on a row carrying one
                fact and two warnings. */}
            <Badge>{binding.provider}</Badge>
            <span className="ai-lane-model">{binding.model}</span>
            {/* WHERE the OpenAI-wire adapter is pointed. It is not a detail of
                the binding, it IS the vendor: `openai_compatible` names a
                protocol, and every broker on it — OpenRouter, Together, a
                self-hosted gateway — reads identically on this row without the
                host. Only this adapter has one, so nothing else grows it. */}
            {binding.base_url ? <span>{hostOf(binding.base_url)}</span> : null}
            {chip}
            {unkeyed?.has(binding.provider) && (
              <Badge tone="warning">{t("aiRouting.noKey")}</Badge>
            )}
            {isUnpriced(catalogue, binding.provider, binding.model, lane) ? (
              <Badge tone="warning">{t("aiRouting.unpriced")}</Badge>
            ) : (
              // What this lane costs to call, where the sheet can say. It is
              // the reason the ladder is ordered the way it is, and reading it
              // used to mean leaving for the price table and finding this model
              // in a list of two hundred.
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
          {/* Never refused, even to a reader who may not save. The row is a
            summary and the body under it is the rest of the binding — the host
            an OpenAI-wire vendor is reached at, the width the embedder asks for
            — so refusing to OPEN it would hide facts from somebody whose job is
            to report them. The fields inside carry the refusal, and so does
            Save. */}
          <span className="ai-lane-open">
            <Button onClick={onOpen} aria-expanded={open}>
              {open ? t("aiRouting.done") : t("aiRouting.change")}
            </Button>
          </span>
        </div>
        {open && (
          <div className="form-row">
            <AdapterFields
              label={t("aiRouting.provider.label")}
              lane={lane}
              laneName={name}
              binding={binding}
              catalogue={catalogue}
              disabled={disabled}
              onChange={onChange}
              providers={providers}
            />
            {extra}
          </div>
        )}
      </div>
    </PanelRow>
  );
}

// The decision lane: the one lane a routing document may leave out. Absent means
// no task asks a decision model first, so the row offers to add one rather than
// drawing an empty binding the server would refuse, and Remove takes the key off
// the document again instead of saving a blank. Once bound it is the same row
// as every other lane, offering only the adapters that answer a decision.
export function DecisionLaneRow({
  binding,
  catalogue,
  unkeyed,
  disabled,
  open,
  onOpen,
  onChange,
}: Readonly<{
  binding: DecisionsBinding | undefined;
  catalogue: ModelCatalogue;
  unkeyed: ReadonlySet<string> | null;
  disabled: boolean;
  open: boolean;
  onOpen: (open: boolean) => void;
  onChange: (next: DecisionsBinding | undefined) => void;
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
              disabled={disabled}
              onClick={() => {
                onChange({ provider: DECISION_PROVIDERS[0], model: "" });
                onOpen(true);
              }}
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
      disabled={disabled}
      open={open}
      onOpen={() => onOpen(!open)}
      onChange={(next) => onChange(reboundDecision(binding, next))}
      providers={DECISION_PROVIDERS}
      chip={processing ? <Badge>{processing}</Badge> : null}
      extra={
        <>
          {binding.provider === OPENROUTER_DECISION_PRESET.provider && (
            // A jev_compatible endpoint is a full URL nobody remembers, and
            // OpenRouter's is the one most installations want. The preset
            // fills the endpoint and the model certified there; the key is
            // the one thing it cannot fill, so the sentence beside it says
            // which one.
            <div>
              <Button
                disabled={disabled}
                onClick={() =>
                  onChange({
                    ...binding,
                    base_url: OPENROUTER_DECISION_PRESET.base_url,
                    model: OPENROUTER_DECISION_PRESET.model,
                  })
                }
              >
                {t("aiRouting.decisions.preset.openrouter")}
              </Button>
              <p className="t-sub">
                {t("aiRouting.decisions.preset.openrouterKey")}
              </p>
            </div>
          )}
          <div>
            <Button
              disabled={disabled}
              onClick={() => {
                onChange(undefined);
                onOpen(false);
              }}
            >
              {t("aiRouting.decisions.remove")}
            </Button>
          </div>
        </>
      }
    />
  );
}

// A decision model and its host name one adapter's endpoint: OpenRouter's Jev
// slug means nothing to TypeSafe's own API or to a self-hosted checkpoint. A provider switch
// therefore starts the binding over rather than pointing the new adapter at the
// old one's address.
function reboundDecision(
  previous: DecisionsBinding,
  next: DecisionsBinding,
): DecisionsBinding {
  return next.provider === previous.provider
    ? next
    : { provider: next.provider, model: "" };
}

// Where the bound decision model processes text, as the server classified it.
//
// Read off the live status rather than derived here: which adapters are local is
// the server's registry, and a second copy on this side would be free to drift
// from it. The status describes the STORED lane, so a draft pointing anywhere
// else says nothing until it is saved, rather than borrowing the old answer.
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

// The document with its decision lane set, or with the key gone. Deleted rather
// than left holding undefined: absent is what the server reads as "no decision
// model", and the draft should be that document, not one that merely serializes
// like it.
export function withDecisions(
  routing: Routing,
  decisions: DecisionsBinding | undefined,
): Routing {
  const next: Routing = { ...routing, decisions };
  if (decisions === undefined) {
    delete next.decisions;
  }
  return next;
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
