// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Callout } from "../../design-system/callout";
import { jurisdictionName } from "../../format/jurisdiction";
import { languageName } from "../../format/languagename";
import { useLocale, useT } from "../../i18n";

// Two things the composer says about the draft in front of the reader, rather
// than about the message they are writing.
//
// Their own file because each is a claim with a reason behind it, and both
// reasons were three lines of comment sitting in the middle of a form seven
// hundred lines from the other. Neither takes a callback or reads a query:
// given the fact, each is the whole notice.

/**
 * The draft is not in the sender's voice.
 *
 * The one loss a sender cannot see in the text — their own register is what
 * nobody proofreads for — so it is said rather than left to be noticed. An
 * `outcome`: it answers the Draft they pressed, and a `warning` outcome is
 * mentioned rather than allowed to interrupt.
 */
export function VoiceDegradedNotice({
  degraded,
}: Readonly<{
  /** Absent is the provenance saying nothing, which is not a degraded voice. */
  degraded?: boolean;
}>) {
  const t = useT();
  if (!degraded) {
    return null;
  }
  return (
    <Callout
      tone="warning"
      kind="outcome"
      title={t("compose.voiceDegradedTitle")}
    >
      {t("compose.voiceDegraded")}
    </Callout>
  );
}

/**
 * The conversation the caller named cannot be answered any more.
 *
 * Said rather than silently fallen back from: the reader asked to answer one
 * thread and the composer is open on another, which they will otherwise learn
 * from the recipient. An `event` — it arrived from elsewhere, so it is
 * mentioned quietly.
 */
export function StaleThreadNotice({ stale }: Readonly<{ stale: boolean }>) {
  const t = useT();
  if (!stale) {
    return null;
  }
  return (
    <Callout tone="warning" kind="event" title={t("compose.threadGoneTitle")}>
      {t("compose.threadGone")}
    </Callout>
  );
}

export function ChannelDraftNotice({
  requested,
  channel,
}: Readonly<{ requested?: boolean; channel: boolean }>) {
  const t = useT();
  if (!requested || !channel) return null;
  return (
    <Callout
      tone="warning"
      kind="outcome"
      title={t("compose.channelDraftUnavailable")}
    />
  );
}

type DraftContentFinding = components["schemas"]["DraftContentFinding"];

/**
 * What the advertising draft does not meet among the content rules of its
 * recipients' laws. Advice: the send still goes.
 */
export function DraftContentNotice({
  findings,
}: Readonly<{ findings?: readonly DraftContentFinding[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const lines = (findings ?? []).flatMap((finding) => {
    if (finding.requirement === "claim_without_proof") {
      return [
        t("compose.contentFinding.claim", {
          words: (finding.words ?? []).join(", "),
        }),
      ];
    }
    if (finding.requirement === "advertising_language_first") {
      return [
        t("compose.contentFinding.language", {
          language: languageName(finding.language ?? "", locale),
        }),
      ];
    }
    if (finding.requirement === "foreign_advertiser") {
      return [
        t("compose.contentFinding.foreignAdvertiser", {
          country: jurisdictionName(finding.country ?? "", locale),
        }),
      ];
    }
    return [finding.detail];
  });
  if (lines.length === 0) {
    return null;
  }
  return (
    <Callout
      tone="warning"
      kind="event"
      title={t("compose.contentFinding.title")}
    >
      {lines.map((line) => (
        <p key={line}>{line}</p>
      ))}
    </Callout>
  );
}
