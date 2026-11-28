// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useViewerZone } from "../lib/viewerzone";

type Profile = components["schemas"]["SlackHistoryProfile"];

export function ChannelHistoryProfile({
  profile,
}: Readonly<{ profile?: Profile }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  if (!profile) return <p>{t("channelProfile.missing")}</p>;
  const label =
    profile.distribution === "internal"
      ? "channelProfile.internal"
      : profile.distribution === "marketplace"
        ? "channelProfile.marketplace"
        : "channelProfile.nonMarketplace";
  return (
    <>
      <p>{t(label)}</p>
      <p>
        {t("channelProfile.limits", {
          history: formatNumber(profile.history_per_minute, locale),
          replies: formatNumber(profile.replies_per_minute, locale),
          historyPage: formatNumber(profile.history_page_size, locale),
          repliesPage: formatNumber(profile.replies_page_size, locale),
        })}
      </p>
      <p>
        {t("channelProfile.provenance", {
          at: formatDateTime(profile.recorded_at, locale, zone),
        })}
      </p>
      {!profile.canonical_supported && <p>{t("channelProfile.unsupported")}</p>}
    </>
  );
}
