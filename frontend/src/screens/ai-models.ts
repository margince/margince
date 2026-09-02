// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import type { ComboBoxSuggestion } from "../design-system/combobox";
import { stable } from "../format/collate";
import { formatUsdPerMTok } from "../format/format";
import type { Locale } from "../i18n";

/**
 * Which models this installation can bind, for the two screens that bind one.
 *
 * IT IS THE PRICE SHEET. `ai_model_rate` already holds every (provider, model)
 * pair this installation can cost a call on — a model outside it serves calls
 * and reports UNPRICED, which nobody notices until the first usage report comes
 * back with a week missing. So the list an admin is offered is exactly the list
 * the server can price, and the two cannot drift because they are one thing.
 * `lane` is what the sheet adds for this: a chat model where a chat tier binds,
 * an embedder where the embeddings lane does.
 *
 * WHY IT NEVER THROWS. Suggestions are help, and the screens that show them do
 * real work without them: the routing card re-points a lane, and cold start
 * cannot be blocked at all. A seat holding `ai_routing` but not `ai_model_rate`
 * gets a 403 here, and the honest answer to that is a text box with nothing in
 * its list — which is what the field was yesterday — rather than an error that
 * reads as a broken installation.
 */

type ModelRate = components["schemas"]["AiModelRate"];

/** One vendor's answer about what it serves, as the wire carries it. */
export type AvailableModels = components["schemas"]["AvailableModelList"];

/** One model in that answer — priced and ranked only where the vendor states it. */
export type VendorModel = AvailableModels["models"][number];

/** The lane a field binds: what a tier picker asks for, what the embed row does. */
export type ModelLane = ModelRate["lane"];

/** The sheet as a caller holds it, before the read has landed. */
export type ModelCatalogue = readonly ModelRate[] | undefined;

export function useAiModelCatalogue() {
  return useQuery({
    queryKey: ["ai-model-rates"],
    // The sheet changes when an operator adds a price, which is not something
    // that happens while somebody is filling in this form.
    staleTime: 5 * 60 * 1000,
    // A refused or failed read is an empty list, never a rejection: see above.
    // Retrying a 403 would only spend requests on an answer that will not
    // change within the session.
    retry: false,
    queryFn: async (): Promise<readonly ModelRate[]> => {
      const { data, error } = await api.GET("/ai-model-rates");
      if (error || !data) {
        return [];
      }
      return data.data;
    },
  });
}

/**
 * The vendor's live catalogue as a caller holds it. `rankedBy` travels with the
 * models because a screen that prints a top ten has to be able to say what
 * "top" measured, and `unavailable` because an empty list on its own cannot
 * tell a vendor that is down from a vendor that serves nothing.
 */
export type VendorCatalogue = Readonly<{
  models: readonly VendorModel[];
  rankedBy: string;
  unavailable: boolean;
}>;

/**
 * The shared read behind BOTH "what does this vendor serve" hooks:
 * `/ai/available-models/{provider}` answers a routing form's full pick list
 * and a first-run shortlist alike, `top` being the only thing that tells them
 * apart. One queryFn, so the two never drift into two answers for one vendor.
 *
 * IT NEVER THROWS. First run and the routing form must not be blocked by
 * another company's uptime, so a failed transport is the same Unavailable
 * state the server already answers a vendor it could not reach with.
 */
function availableModelsQueryOptions(
  provider: string,
  tier: string,
  top?: number,
) {
  return {
    queryKey: ["ai-available-models", provider, tier, top ?? null],
    staleTime: 5 * 60 * 1000,
    retry: false,
    queryFn: async (): Promise<AvailableModels> => {
      try {
        const { data, error } = await api.GET(
          "/ai/available-models/{provider}",
          { params: { path: { provider }, query: { tier, top } } },
        );
        // A 200 is not by itself an answer: an intermediary can return one with
        // a body this cannot read, and a list that is missing rather than empty
        // would reach `select`/the render as undefined and take the screen
        // down. An unreadable body is the same thing as an unreachable vendor.
        if (error || !data || !Array.isArray(data.models)) {
          return { provider, models: [], unavailable: "unreachable" };
        }
        return data;
      } catch {
        return { provider, models: [], unavailable: "unreachable" };
      }
    },
  } as const;
}

/**
 * What the vendor is serving TODAY, which the price sheet cannot know.
 *
 * The sheet is what this installation can put a number on, and it is right that
 * the sheet governs cost. It is not a catalogue of what exists: a model the
 * vendor shipped last week is absent from it, and an admin binding one has no
 * way to find out what the options even are. This read answers only that
 * question, for the ONE vendor whose catalogue is public and unauthenticated,
 * and it is asked only once that vendor has been chosen.
 *
 * `top: 10` is the only thing that makes this a different request from
 * `useAvailableModels`'s: a first run wants ten names to try, not four hundred.
 */
export function useVendorCatalogue(provider: "openrouter" | undefined) {
  return useQuery({
    ...availableModelsQueryOptions(provider ?? "", "", 10),
    // Nothing is asked until a vendor with a public catalogue is chosen. The
    // admin is about to hand that vendor their key and their text, so reading
    // its catalogue is not an escalation, but reading it before they have
    // chosen would be this installation talking to a vendor it may never use.
    enabled: provider !== undefined,
    select: (data): VendorCatalogue => ({
      models: data.models,
      rankedBy: data.ranked_by ?? "",
      unavailable: data.unavailable !== undefined,
    }),
  });
}

/**
 * The vendor's own price for one model, or undefined if it does not serve it.
 *
 * Callers use this only where the SHEET has nothing to say: a recorded price
 * always outranks a proposed one, because the recorded number is the one this
 * installation has agreed to bill against.
 */
export function vendorModel(
  catalogue: VendorCatalogue | undefined,
  modelId: string,
): VendorModel | undefined {
  return catalogue?.models.find((m) => m.id === modelId);
}

/**
 * What one model costs, short enough to sit beside its id in a dropdown row.
 *
 * A chat model is priced on BOTH sides and the output rate is the larger one —
 * five times the input across most of this sheet — so showing input alone would
 * rank the list by the number that matters least. The arrow is the whole
 * explanation: what goes in, what comes out. An embedding lane has no output at
 * all, so a second figure there would be a zero that means "not applicable"
 * rendered as if it were a price.
 */
// A price the sheet cannot state. Blank counts, and it is the case worth
// spelling out: `Number("")` is 0, not NaN, so an absent price would otherwise
// render as US$0.00 — and free is the one thing this product is careful never
// to say by accident.
export function unreadablePrice(price: string): boolean {
  return price.trim() === "" || !Number.isFinite(Number(price));
}

function priceHint(rate: ModelRate, locale: Locale): string | undefined {
  // The hint is decoration, so an unreadable sheet offers the model without a
  // price — never a NaN in the list, and never a throw inside a render, which
  // takes the whole settings page down with it.
  if (
    unreadablePrice(rate.input_per_mtok) ||
    unreadablePrice(rate.output_per_mtok)
  ) {
    return undefined;
  }
  const shown = formatUsdPerMTok(rate.input_per_mtok, locale);
  if (rate.lane === "embeddings") {
    return shown;
  }
  return `${shown} → ${formatUsdPerMTok(rate.output_per_mtok, locale)}`;
}

/**
 * The models to offer for one field: this provider's, in this lane, by id.
 *
 * Sorted rather than left in the server's order because the server's order is
 * the price sheet's — provider then model — and a reader scanning one provider's
 * models wants them alphabetical, which for a namespaced id also groups a
 * vendor's own family together.
 *
 * `stable`, not the reader's collation: a model id is a KEY the vendor minted,
 * not a name in anybody's language, and two colleagues comparing the same list
 * must see it in the same order.
 */
export function suggestionsFor(
  catalogue: ModelCatalogue,
  provider: string,
  lane: ModelLane,
  locale: Locale,
): readonly ComboBoxSuggestion[] {
  return (catalogue ?? [])
    .filter((r) => r.provider === provider && r.lane === lane)
    .map((r) => ({ value: r.model_id, hint: priceHint(r, locale) }))
    .sort((a, b) => stable(a.value, b.value));
}

/**
 * The vendor's ranked models, for a field the sheet cannot fill on its own.
 *
 * IN THE VENDOR'S ORDER, not alphabetical, which is the one place this differs
 * from `suggestionsFor` and the whole reason the list is worth fetching: the
 * order IS the ranking, and sorting it by id would throw away the only thing
 * that makes ten rows more useful than four hundred.
 *
 * A model the sheet already prices is dropped here rather than listed twice.
 * The sheet's own row is the better offer: it carries a date and a price this
 * installation has agreed to, where the vendor's is a number nobody has
 * confirmed yet.
 */
export function vendorSuggestions(
  vendor: VendorCatalogue | undefined,
  priced: ModelCatalogue,
  provider: string,
  locale: Locale,
): readonly ComboBoxSuggestion[] {
  const onSheet = new Set(
    (priced ?? [])
      .filter((r) => r.provider === provider)
      .map((r) => r.model_id),
  );
  return (vendor?.models ?? [])
    .filter((m) => !onSheet.has(m.id))
    .map((m) => ({ value: m.id, hint: vendorHint(m, locale) }));
}

function vendorHint(model: VendorModel, locale: Locale): string | undefined {
  const { input_per_mtok: input, output_per_mtok: output } = model;
  // Absent is absent, not a decoration to invent — the same guard as the
  // sheet's hint, widened for a price that may not be there at all: a hint is
  // decoration, and a model priced on only one side (or neither) offers the
  // field without one rather than printing NaN or half a number.
  if (
    input === undefined ||
    output === undefined ||
    unreadablePrice(input) ||
    unreadablePrice(output)
  ) {
    return undefined;
  }
  const shown = formatUsdPerMTok(input, locale);
  return `${shown} → ${formatUsdPerMTok(output, locale)}`;
}

/**
 * What the VENDOR says it serves, for the field that binds a lane to it.
 *
 * The sheet below answers what this installation can PRICE, which is a
 * different and older question: it is a table somebody maintains, so a model
 * released after the last edit was simply absent and a reader concluded the
 * product could not reach it. This asks the vendor.
 *
 * Enabled only while a field is open, and keyed on the provider: it is a real
 * round-trip to a vendor, and every lane on the page firing one at mount would
 * spend an installation's credentials on screens nobody is looking at.
 *
 * NEVER throws, for the same reason the sheet does not: the field binds any id
 * the reader types, and a vendor being down must not take the form with it. The
 * server already says so — a vendor it could not ask is a 200 carrying the
 * reason — and this only has to survive the transport failing too.
 */
export function useAvailableModels(
  provider: string,
  tier: string,
  enabled: boolean,
) {
  // The lane is part of the key, not just the request: two lanes on one vendor
  // may be reached at two hosts, so their answers are two different lists and
  // must not share a cache entry. No `top`: a routing form binds an id its
  // reader already knows, so it wants the vendor's whole list, not a shortlist.
  return useQuery({
    ...availableModelsQueryOptions(provider, tier),
    enabled: enabled && provider !== "",
  });
}

/**
 * The suggestions one field offers: what the vendor serves, priced from the
 * sheet where the sheet knows it.
 *
 * The union, not one or the other. The vendor is the authority on what EXISTS
 * and the sheet on what it COSTS, and a reader needs both — an id the sheet
 * still lists but the vendor has retired is worth keeping visible (it may be
 * what this lane is bound to today), and one the vendor serves that nothing has
 * priced is the case the UNPRICED pill was built for.
 *
 * Vendor order first, because a vendor that dates its models returns them
 * newest first and that is the order somebody looking for "the new one" wants;
 * the sheet's leftovers follow, sorted, so the tail is stable.
 */
export function offeredModels(
  available: AvailableModels | undefined,
  catalogue: ModelCatalogue,
  provider: string,
  lane: ModelLane,
  locale: Locale,
): readonly ComboBoxSuggestion[] {
  const priced = new Map(
    (catalogue ?? [])
      .filter((r) => r.provider === provider && r.lane === lane)
      .map((r) => [r.model_id, priceHint(r, locale)]),
  );
  const fromVendor = (available?.models ?? [])
    // A vendor that says what a model is FOR is taken at its word; one that
    // says nothing is offered on every lane, because guessing would either hide
    // a usable model or offer an embedder to a chat tier.
    .filter((m) => m.lane === undefined || m.lane === lane)
    .map((m) => ({ value: m.id, hint: priced.get(m.id) }));
  const seen = new Set(fromVendor.map((s) => s.value));
  const fromSheet = [...priced.entries()]
    .filter(([id]) => !seen.has(id))
    .map(([value, hint]) => ({ value, hint }))
    .sort((a, b) => stable(a.value, b.value));
  return [...fromVendor, ...fromSheet];
}
