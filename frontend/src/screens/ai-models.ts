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
function unreadablePrice(price: string): boolean {
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
