// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { formatUsdPerMTok } from "../format/format";
import { type Locale, useT } from "../i18n";
import {
  type ModelCatalogue,
  type ModelLane,
  type VendorCatalogue,
  vendorModel,
} from "./ai-models";

type VendorModel = components["schemas"]["AiModelCatalogueEntry"];

/**
 * What the models this installation is about to bind will cost, said out loud.
 *
 * The price sheet is already the list a model is CHOSEN from (`ai-models.ts`),
 * and until now its prices reached the reader only as a hint inside a dropdown
 * row — visible while the list is open, gone the moment it closes. So the
 * binding a person actually confirms was the one screen in the product that
 * showed a price nowhere. This is that number, on the surface where the choice
 * is made and after it has been made.
 *
 * A STRIP RATHER THAN TWO CARDS. The two lanes are one decision: what a call
 * costs here is what thinking costs plus what remembering costs, and they are
 * read against each other — an embedder priced like a chat model is the mistake
 * this row is for. Cards would be read one at a time.
 *
 * IT NEVER INVENTS A PRICE. `ai-models.ts` states the rule and this holds the
 * same line for the same reason: a model the sheet cannot price says so, in
 * words, because `Number("")` is 0 and free is the one thing this product must
 * never say by accident. An unpriced model still serves calls — it reports
 * UNPRICED, and a usage report with a week missing is what the warning tone is
 * there to pre-empt.
 */

type Rate = NonNullable<ModelCatalogue>[number];

/** The sheet's row for one binding, or nothing if it has none. */
export function rateFor(
  catalogue: ModelCatalogue,
  provider: string,
  modelId: string,
  lane: ModelLane,
): Rate | undefined {
  return (catalogue ?? []).find(
    (r) => r.provider === provider && r.model_id === modelId && r.lane === lane,
  );
}

// Blank counts as unreadable, not as zero — the same test `ai-models.ts`
// applies before it offers a price beside a model id, kept identical here
// because two spellings of "this price cannot be shown" is how one of them
// ends up rendering US$0.00.
function unreadable(price: string): boolean {
  return price.trim() === "" || !Number.isFinite(Number(price));
}

/**
 * The figure for one lane. A chat model is priced on both sides and the arrow
 * is the whole explanation — what goes in, what comes out. An embedder has no
 * output at all, so a second figure there would be a zero meaning "not
 * applicable" dressed as a price.
 */
function priceOf(rate: Rate, locale: Locale): string | undefined {
  if (unreadable(rate.input_per_mtok)) {
    return undefined;
  }
  const input = formatUsdPerMTok(rate.input_per_mtok, locale);
  if (rate.lane === "embeddings") {
    return input;
  }
  if (unreadable(rate.output_per_mtok)) {
    return undefined;
  }
  return `${input} → ${formatUsdPerMTok(rate.output_per_mtok, locale)}`;
}

/**
 * The vendor's asking price for one model, in the same shape the sheet's is.
 *
 * Both sides, always: the vendor publishes an input and an output price for
 * every chat model, and this is only ever reached for a chat lane, because the
 * one vendor with a public catalogue publishes no embedding models at all.
 */
function proposedPrice(
  model: VendorModel | undefined,
  locale: Locale,
): string | undefined {
  if (
    model === undefined ||
    unreadable(model.input_per_mtok) ||
    unreadable(model.output_per_mtok)
  ) {
    return undefined;
  }
  const input = formatUsdPerMTok(model.input_per_mtok, locale);
  return `${input} → ${formatUsdPerMTok(model.output_per_mtok, locale)}`;
}

/**
 * One lane's slot. Absent from the strip entirely when no model is chosen yet:
 * an empty slot on a plate reads as a reading that failed to load, and before a
 * model id is typed there is nothing this lane costs.
 */
function LaneSlot({
  label,
  modelId,
  rate,
  proposed,
  locale,
}: Readonly<{
  label: string;
  modelId: string;
  rate: Rate | undefined;
  /** The vendor's live entry for this model, where the sheet has no price. */
  proposed: VendorModel | undefined;
  locale: Locale;
}>) {
  const t = useT();
  const price = rate ? priceOf(rate, locale) : undefined;
  // The vendor's own asking price, for a model the sheet has never seen. It is
  // shown ONLY here, under the sheet: a recorded price outranks a proposed one,
  // because the recorded number is the one this installation has agreed to bill
  // against and the proposed one is a number a machine went and read.
  const asked =
    price === undefined ? proposedPrice(proposed, locale) : undefined;
  if (asked !== undefined) {
    return (
      <StatCard
        label={label}
        value={asked}
        // `source` rather than a tone: this is not a worse price, it is a price
        // from somewhere else, and the card already owns a slot for saying
        // where a figure came from. Indigo, because indigo means a machine
        // proposed it and nobody has confirmed it yet.
        source={
          <span className="ai-rate-proposed">{t("aiRates.proposed")}</span>
        }
        detail={
          <>
            <span className="t-mono">{modelId}</span>
            <span>{t("aiRates.perMTokInOut")}</span>
            <span>{t("aiRates.proposedDetail")}</span>
          </>
        }
      />
    );
  }
  // Both halves of the same case: a model the sheet has never seen, and one it
  // has a row for that it cannot state. They read the same to the person
  // binding it, because the consequence is the same — the call runs, and the
  // usage report cannot say what it cost.
  if (rate === undefined || price === undefined) {
    return (
      <StatCard
        label={label}
        value={t("aiRates.unpriced")}
        tone="warn"
        detail={
          <>
            <span className="t-mono">{modelId}</span>
            <span>{t("aiRates.unpricedDetail")}</span>
          </>
        }
      />
    );
  }
  return (
    <StatCard
      label={label}
      value={price}
      detail={
        <>
          <span className="t-mono">{modelId}</span>
          <span>
            {rate.lane === "embeddings"
              ? t("aiRates.perMTok")
              : t("aiRates.perMTokInOut")}
          </span>
          {/* The date the sheet's own row carries. A price with no date is a
              price somebody has to go and re-verify against the vendor, which
              is the trip this saves them.

              UNDER the figure rather than in `source`, which the slot draws
              above it: a second grey line before the number turns provenance
              into a second label and the price into the third thing read.

              Written as the sheet writes it — the ISO day, the same string the
              price-sheet table shows — because a date-only value put through a
              zone-aware formatter lands on the day before for half the world. */}
          <span>{t("aiRates.priced", { date: rate.effective_date })}</span>
        </>
      }
    />
  );
}

/**
 * What this binding costs, for the two lanes a binding fills.
 *
 * Both ids are the caller's current values rather than the sheet's, because
 * the field takes anything the vendor serves: a typed id the sheet has never
 * seen is a legitimate binding and its slot says so, which is the case that
 * matters most.
 */
export function ModelRatePlate({
  catalogue,
  vendor,
  provider,
  chatModel,
  embedModel,
  locale,
}: Readonly<{
  catalogue: ModelCatalogue;
  /**
   * The vendor's live catalogue, where one was fetched. Only the chat lane can
   * ever be answered from it: the one vendor with a public catalogue publishes
   * no embedding models, so the embed lane is the sheet's alone.
   */
  vendor?: VendorCatalogue;
  provider: string;
  chatModel: string;
  embedModel: string;
  locale: Locale;
}>) {
  const t = useT();
  const chat = chatModel.trim();
  const embed = embedModel.trim();
  if (chat === "" && embed === "") {
    return null;
  }
  return (
    <StatStrip className="ai-rate-plate" testId="ai-rate-plate">
      {chat === "" ? null : (
        <LaneSlot
          label={t("aiRates.chatLane")}
          modelId={chat}
          rate={rateFor(catalogue, provider, chat, "chat")}
          proposed={vendorModel(vendor, chat)}
          locale={locale}
        />
      )}
      {embed === "" ? null : (
        <LaneSlot
          label={t("aiRates.embedLane")}
          modelId={embed}
          rate={rateFor(catalogue, provider, embed, "embeddings")}
          // Never a proposed price: no vendor with a public catalogue publishes
          // embedding models, so anything shown here would be invented.
          proposed={undefined}
          locale={locale}
        />
      )}
    </StatStrip>
  );
}
