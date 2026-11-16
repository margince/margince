// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { useViewerZone } from "../lib/viewerzone";
import { Badge } from "./atoms";
import { OffsiteLink } from "./offsitelink";
import type { TimelineEntry } from "./timelineentry";

type ChatRow = Pick<TimelineEntry, "chatEpisode" | "withheld">;

// The chat a summary note came from. Dispact is the one chat a channel links
// to today.
const CHAT_PROVIDER = "Dispact";

// ChatSummaryFlag marks a note the AI wrote from a chat channel's talk.
export function ChatSummaryFlag({ entry }: Readonly<{ entry: ChatRow }>) {
  const t = useT();
  if (!entry.chatEpisode || entry.withheld) {
    return null;
  }
  return (
    <>
      <Badge tone="ai">{t("timeline.chatSummary.badge")}</Badge>
      <Badge>
        {t("timeline.chatSummary.from", { provider: CHAT_PROVIDER })}
      </Badge>
    </>
  );
}

// ChatEpisodeLine says how much talk the summary covers, when it ended, and
// opens it in the chat: the talk itself is not kept in Margince.
export function ChatEpisodeLine({ entry }: Readonly<{ entry: ChatRow }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const episode = entry.chatEpisode;
  if (!episode || entry.withheld) {
    return null;
  }
  return (
    <span className="tl-meta">
      {plural("timeline.chatSummary.meta", episode.message_count, {
        count: formatNumber(episode.message_count, locale),
        ended: formatDateTime(episode.ended_at, locale, zone),
      })}{" "}
      <OffsiteLink href={episode.url}>
        {t("timeline.chatSummary.open", { provider: CHAT_PROVIDER })}
      </OffsiteLink>
    </span>
  );
}
