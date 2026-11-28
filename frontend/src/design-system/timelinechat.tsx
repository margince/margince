// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { useViewerZone } from "../lib/viewerzone";
import { Badge } from "./atoms";
import { ChatSourceLink } from "./chatsourcelink";
import { ProviderMark, providerBrandName } from "./provider-mark";
import type { TimelineEntry } from "./timelineentry";

type ChatRow = Pick<
  TimelineEntry,
  "id" | "chatEpisode" | "chatUpdatedAt" | "withheld"
>;

// ChatSummaryFlag marks a note the AI wrote from a chat channel's talk.
export function ChatSummaryFlag({
  entry,
}: Readonly<{ entry: Omit<ChatRow, "id"> }>) {
  const t = useT();
  if (!entry.chatEpisode || entry.withheld) {
    return null;
  }
  return (
    <>
      {entry.chatEpisode.source_changed && (
        <Badge tone="warning">{t("timeline.chatSummary.sourceChanged")}</Badge>
      )}
      <Badge tone="ai">{t("timeline.chatSummary.badge")}</Badge>
      <Badge
        mark={
          <ProviderMark providerKey={entry.chatEpisode.provider ?? "dispact"} />
        }
      >
        {t("timeline.chatSummary.from", {
          provider:
            providerBrandName(entry.chatEpisode.provider ?? "dispact") ??
            entry.chatEpisode.provider ??
            "Dispact",
        })}
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
      {episode.channel_name && <span>{episode.channel_name} · </span>}
      {plural("timeline.chatSummary.meta", episode.message_count, {
        count: formatNumber(episode.message_count, locale),
        ended: formatDateTime(episode.ended_at, locale, zone),
      })}{" "}
      {entry.chatUpdatedAt && (
        <span>
          {t("timeline.chatSummary.updated", {
            at: formatDateTime(entry.chatUpdatedAt, locale, zone),
          })}{" "}
          ·{" "}
        </span>
      )}
      <ChatSourceLink
        activityId={entry.id}
        provider={
          providerBrandName(episode.provider ?? "dispact") ??
          episode.provider ??
          "Dispact"
        }
      />
    </span>
  );
}
