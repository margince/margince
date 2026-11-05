// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useDealLines } from "./deal360/deallines";
import type { Offer } from "./offers";
import { OfferLineEditor } from "./offers.lines";

/**
 * The lines part of an offer. An offer typed by hand is edited here while it
 * is a draft. An offer made from the deal's products changes only through the
 * deal: it says which revision it copied, says when the deal moved on, and a
 * draft can take the deal's lines again.
 */
export function OfferLinesPanel({ offer }: Readonly<{ offer: Offer }>) {
  if (offer.source_commercial_revision == null) {
    return offer.status === "draft" ? <OfferLineEditor offer={offer} /> : null;
  }
  return (
    <DealSourcedLines
      offer={offer}
      revision={offer.source_commercial_revision}
    />
  );
}

function DealSourcedLines({
  offer,
  revision,
}: Readonly<{ offer: Offer; revision: number }>) {
  const t = useT();
  const client = useQueryClient();
  const deal = useDealLines(offer.deal_id).data;
  const refresh = useMutation({
    mutationFn: async (expected: number) =>
      unwrap(
        await api.POST("/offers/{id}/refresh-from-deal", {
          params: { path: { id: offer.id } },
          body: { expected_revision: expected },
        }),
        t,
      ),
    onSuccess: (updated: Offer) => {
      client.setQueryData(["offer", offer.id], updated);
    },
  });
  const stale = deal !== undefined && deal.commercial_revision !== revision;
  return (
    <Callout
      tone={stale ? "warning" : "info"}
      title={stale ? t("offer.fromDeal.stale") : t("offer.fromDeal.title")}
      actions={
        <>
          <Button
            variant="link"
            onClick={() => navigate({ screen: "deals", id: offer.deal_id })}
          >
            {t("offer.fromDeal.editDeal")}
          </Button>
          {stale && offer.status === "draft" && deal && (
            <Button
              pending={refresh.isPending}
              onClick={() => refresh.mutate(deal.commercial_revision)}
            >
              {t("offer.fromDeal.refresh")}
            </Button>
          )}
        </>
      }
    >
      {stale && offer.status !== "draft"
        ? t("offer.fromDeal.revise")
        : t("offer.fromDeal.body")}
      {refresh.error && <ErrorLine error={refresh.error} />}
    </Callout>
  );
}
