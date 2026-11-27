// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { useViewerZone } from "../lib/viewerzone";

type Health = components["schemas"]["ChannelSyncHealth"];

export function ChannelSyncHealth({
  health,
  reading,
}: Readonly<{ health: Health; reading: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <>
      {reading && (
        <>
          {health.notifications_delayed && (
            <Callout
              tone="warning"
              title={t("channelLink.notificationsDelayed")}
            >
              {t("channelLink.notificationsPolling")}
            </Callout>
          )}
          <p>
            {health.coverage_at
              ? t("channelLink.coverageThrough", {
                  at: formatDateTime(health.coverage_at, locale, zone),
                })
              : t("channelLink.coverageWaiting")}
          </p>
          {(health.pending_conversations > 0 ||
            health.failed_conversations > 0) && (
            <p>
              {t("channelLink.conversationQueue", {
                pending: formatNumber(health.pending_conversations, locale),
                failed: formatNumber(health.failed_conversations, locale),
              })}
            </p>
          )}
        </>
      )}
      {health.delivery_error && (
        <Callout tone="warning" title={t("channelLink.outboundNeedsAttention")}>
          <SyncFailure reason={health.delivery_error} />
        </Callout>
      )}
      {health.last_error && (
        <Callout tone="warning" title={t("channelLink.syncNeedsAttention")}>
          <SyncFailure reason={health.last_error} />
        </Callout>
      )}
      {health.pending_crm_updates > 0 && (
        <p>
          {plural("channelLink.outboundQueue", health.pending_crm_updates, {
            count: formatNumber(health.pending_crm_updates, locale),
          })}
        </p>
      )}
      {health.unconfirmed_deliveries > 0 && (
        <Callout tone="warning" title={t("channelLink.deliveryUnconfirmed")}>
          {t("channelLink.deliveryUnconfirmedHelp")}
        </Callout>
      )}
    </>
  );
}

function SyncFailure({
  reason,
}: Readonly<{
  reason: NonNullable<Health["last_error"] | Health["delivery_error"]>;
}>) {
  const t = useT();
  switch (reason) {
    case "history_profile":
      return t("channelProfile.reconnect");
    case "audience_changed":
      return t("channelLink.audienceNeedsReview");
    case "delivery_unknown":
      return t("channelLink.deliveryUnconfirmedHelp");
    case "source_incomplete":
      return t("channelLink.sourceIncomplete");
    case "source_too_long":
      return t("channelLink.sourceTooLong");
    case "model_unavailable":
      return t("channelLink.summaryWaiting");
    case "rate_limited":
      return t("channelLink.providerWaiting");
    case "access_lost":
      return t("channelLink.sourceAccessLost");
    case "source_changed":
      return t("channelLink.sourceChanged");
    default:
      return t("channelLink.sourceUnavailable");
  }
}
