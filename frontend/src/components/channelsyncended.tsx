// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useViewerZone } from "../lib/viewerzone";

import { ChannelProviderCopies } from "./channelprovidercopies";

type Ended = components["schemas"]["ChannelEndedSync"];

export function ChannelSyncEnded({
  sync,
  canEdit,
}: Readonly<{ sync: Ended; canEdit: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <Callout
      tone="info"
      title={t("channelEnded.title", {
        at: formatDateTime(sync.ended_at, locale, zone),
      })}
    >
      <p>{t("channelEnded.retention")}</p>
      <p>
        {sync.read_through
          ? t("channelEnded.read", {
              at: formatDateTime(sync.read_through, locale, zone),
            })
          : t("channelEnded.noRead")}
      </p>
      <p>
        {sync.last_posted_at
          ? t("channelEnded.posted", {
              at: formatDateTime(sync.last_posted_at, locale, zone),
            })
          : t("channelEnded.noPost")}
      </p>
      {canEdit && <ChannelProviderCopies copies={sync} />}
    </Callout>
  );
}
