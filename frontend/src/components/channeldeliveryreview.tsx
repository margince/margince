// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { QueryStates } from "../design-system/querystates";
import { formatDateTime, formatNumber } from "../format/format";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, unwrap } from "../lib/problem";
import { useViewerZone } from "../lib/viewerzone";

import { ChannelProviderCopies } from "./channelprovidercopies";

type Link = components["schemas"]["ChannelLink"];
type Kind = components["schemas"]["ChannelLinkRecordType"];
type Decision = components["schemas"]["ReviewChannelDeliveryRequest"];
type Review = components["schemas"]["ChannelDeliveryReview"];
type Intent = "review" | "restart";

export function ChannelDeliveryRecovery({
  channel,
  recordType,
  recordId,
  busy = false,
}: Readonly<{
  channel: Link;
  recordType: Kind;
  recordId: string;
  busy?: boolean;
}>) {
  const t = useT();
  const client = useQueryClient();
  const [intent, setIntent] = useState<Intent | null>(null);
  const query = useQuery({
    queryKey: [
      "channel-link",
      "delivery-review",
      recordType,
      recordId,
      channel.id,
    ],
    enabled: intent !== null,
    queryFn: async () =>
      unwrap(
        await api.GET("/channel-links/{record_type}/{id}/delivery-review", {
          params: { path: { record_type: recordType, id: recordId } },
        }),
      ),
  });
  const action = useMutation({
    mutationFn: async (
      instruction: Readonly<{ kind: Kind; id: string; body: Decision }>,
    ) =>
      unwrap(
        await api.POST("/channel-links/{record_type}/{id}/delivery-review", {
          params: {
            path: { record_type: instruction.kind, id: instruction.id },
          },
          body: instruction.body,
        }),
      ),
    onSuccess: async () => {
      setIntent(null);
      await client.invalidateQueries({ queryKey: ["channel-link"] });
    },
  });
  const review = query.data;
  const sameLink = review?.link_id === channel.id;
  const ready =
    review &&
    !busy &&
    sameLink &&
    !query.isError &&
    !query.isFetching &&
    (intent === "review"
      ? review.deliveries.length > 0
      : review.overview_card_paused && review.deliveries.length === 0);
  const open = (next: Intent) => {
    action.reset();
    setIntent(next);
  };
  return (
    <>
      <Button
        disabled={busy || action.isPending}
        onClick={() => open("review")}
      >
        {t("channelDelivery.review")}
      </Button>
      {channel.overview_card_paused && (
        <Callout tone="warning" title={t("channelDelivery.cardPaused")}>
          <p>{t("channelDelivery.cardPausedHelp")}</p>
          <Button
            disabled={busy || action.isPending}
            onClick={() => open("restart")}
          >
            {t("channelDelivery.restart")}
          </Button>
        </Callout>
      )}
      <ConfirmModal
        open={intent !== null}
        onClose={() => setIntent(null)}
        title={t(
          intent === "restart"
            ? "channelDelivery.restartTitle"
            : "channelDelivery.title",
        )}
        confirmLabel={t(
          intent === "restart"
            ? "channelDelivery.restart"
            : "channelDelivery.skip",
        )}
        confirmDisabled={!ready}
        confirmReason={reviewRefusal(t, Boolean(ready), query.isFetching)}
        pending={action.isPending}
        error={action.error ? problemMessageOf(action.error, t) : null}
        onConfirm={() => {
          if (!ready || !review || !intent) return;
          action.mutate({
            kind: recordType,
            id: recordId,
            body: {
              link_id: review.link_id,
              action: intent === "restart" ? "restart_card" : "skip_unknown",
              acknowledged: true,
              delivery_ids:
                intent === "restart"
                  ? []
                  : review.deliveries.map((item) => item.id),
            },
          });
        }}
      >
        <p>{channel.channel_name}</p>
        <p>
          {t(
            intent === "restart"
              ? "channelDelivery.restartWarning"
              : "channelDelivery.help",
          )}
        </p>
        <QueryStates query={query} pendingLabel={t("messenger.loading")}>
          {review && !sameLink && (
            <ErrorLine>{t("channelDelivery.linkChanged")}</ErrorLine>
          )}
          {review && sameLink && (
            <DeliveryReviewBody review={review} intent={intent ?? "review"} />
          )}
        </QueryStates>
      </ConfirmModal>
    </>
  );
}

function DeliveryReviewBody({
  review,
  intent,
}: Readonly<{ review: Review; intent: Intent }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  if (intent === "restart")
    return review.deliveries.length > 0 ? (
      <p>{t("channelDelivery.reviewFirst")}</p>
    ) : null;
  if (review.deliveries.length === 0)
    return (
      <>
        {review.provider_copies && (
          <ChannelProviderCopies copies={review.provider_copies} />
        )}
        <p>{t("channelDelivery.none")}</p>
      </>
    );
  return (
    <>
      {review.provider_copies && (
        <ChannelProviderCopies copies={review.provider_copies} />
      )}
      <ul aria-label={t("channelDelivery.list")}>
        {review.deliveries.map((item) => (
          <li key={item.id}>
            {item.overview_card
              ? t("channelDelivery.card")
              : plural("channelDelivery.updates", item.update_count, {
                  count: formatNumber(item.update_count, locale),
                })}
            {" · "}
            {formatDateTime(item.created_at, locale, zone)}
          </li>
        ))}
      </ul>
    </>
  );
}

function reviewRefusal(t: Translator, ready: boolean, loading: boolean) {
  if (ready) return undefined;
  if (loading) return t("messenger.loading");
  return t("channelDelivery.reviewFirst");
}
