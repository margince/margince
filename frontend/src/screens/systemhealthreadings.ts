// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { STAT_CARD_TONES } from "../design-system/statcard";
import { formatNumber } from "../format/format";
import type { Locale, PluralTranslator, Translator } from "../i18n";
import {
  answersFromStandIn,
  answersOnlyFromStandIn,
} from "./ai-provider-health";

// Each System health report read down to one answer. It says what the slot in
// the page's row shows, and whether the full card stands under the row.

type Tone = (typeof STAT_CARD_TONES)[number];

export type HealthReading = Readonly<{
  // Null where the reader lacks the grant: the card below says so instead.
  card: Readonly<{
    value: string;
    tone?: Tone;
    detail?: string;
    href?: string;
  }> | null;
  // Whether the full card stands under the row: a finding, or a report with
  // rows to read.
  panel: boolean;
}>;

type Words = Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
}>;

export type HealthProbe<Data> = Readonly<{
  canSee: boolean;
  // The session is still loading, so `canSee` is not an answer yet.
  probing: boolean;
  query: Readonly<{
    data: Data | undefined;
    isError: boolean;
    errorUpdateCount: number;
  }>;
}>;

const checking = (t: Translator): HealthReading => ({
  card: { value: t("systemHealth.checking") },
  panel: false,
});

// The steps every reading shares before its own report decides it. A failed
// read keeps the card, which carries the server's words and the retry.
export function readHealth<Data>(
  probe: HealthProbe<Data>,
  t: Translator,
  read: (data: Data) => HealthReading,
): HealthReading {
  if (!probe.canSee) {
    return probe.probing ? checking(t) : { card: null, panel: true };
  }
  // Failed until a read lands: a retry puts the query back to pending, and
  // hiding the card then would remount it and ask again, forever.
  if (
    probe.query.isError ||
    (probe.query.data === undefined && probe.query.errorUpdateCount > 0)
  ) {
    return {
      card: { value: t("systemHealth.unread"), tone: "danger" },
      panel: true,
    };
  }
  if (probe.query.data === undefined) {
    return checking(t);
  }
  return read(probe.query.data);
}

const total = <Row>(rows: readonly Row[], count: (row: Row) => number) =>
  rows.reduce((sum, row) => sum + count(row), 0);

type ReindexStatus = components["schemas"]["EmbedReindexStatus"];

export function searchIndexReading(
  status: ReindexStatus,
  { t, locale }: Words,
): HealthReading {
  if (status.status === "reembedding") {
    return {
      card: { value: t("embedreindex.statusReembedding"), tone: "info" },
      panel: true,
    };
  }
  if (status.reindex_needed) {
    return {
      card: {
        value: t("embedreindex.statusNeeded"),
        tone: "warning",
        detail: t("embedreindex.entitiesPending", {
          count: formatNumber(status.entities_pending, locale),
        }),
      },
      panel: true,
    };
  }
  return {
    card: { value: t("embedreindex.statusIdle"), tone: "success" },
    panel: false,
  };
}

// Only the recent dead and the live retries set the tone: the week-long
// failure list would keep a finished outage red for days.
export function jobsReading(
  health: components["schemas"]["JobHealth"],
  { t, plural, locale }: Words,
): HealthReading {
  const dead = total(health.kinds, (kind) => kind.dead_recent);
  const retrying = total(health.kinds, (kind) => kind.retrying);
  const busy = health.kinds.length > 0 || health.recent_failures.length > 0;
  if (dead > 0) {
    return {
      card: {
        value: plural("systemHealth.jobs.dead", dead, {
          count: formatNumber(dead, locale),
        }),
        tone: "danger",
      },
      panel: true,
    };
  }
  if (retrying > 0) {
    return {
      card: {
        value: plural("systemHealth.jobs.retrying", retrying, {
          count: formatNumber(retrying, locale),
        }),
        tone: "warning",
      },
      panel: true,
    };
  }
  return {
    card: {
      value: t(busy ? "systemHealth.jobs.keepingUp" : "systemHealth.jobs.idle"),
      tone: "success",
    },
    panel: busy,
  };
}

export function intakeReading(
  health: components["schemas"]["ExtensionIngestHealth"],
  { t, plural, locale }: Words,
): HealthReading {
  const refused = total(health.units, (unit) => unit.refused);
  if (health.units.length === 0) {
    return {
      card: { value: t("systemHealth.intake.none"), tone: "success" },
      panel: false,
    };
  }
  return {
    card: {
      value: plural("systemHealth.intake.refused", refused, {
        count: formatNumber(refused, locale),
      }),
      tone: "warning",
    },
    panel: true,
  };
}

export function providersReading(
  health: components["schemas"]["AiProviderHealth"],
  { t, plural, locale }: Words,
  setUpHref: string,
): HealthReading {
  const affected = health.providers.length;
  if (affected > 0) {
    // `degraded` still takes calls; every other state refuses them.
    const blocked = health.providers.some(
      (entry) => entry.health !== "degraded",
    );
    return {
      card: {
        value: plural("systemHealth.providers.affected", affected, {
          count: formatNumber(affected, locale),
        }),
        tone: blocked ? "danger" : "warning",
      },
      panel: true,
    };
  }
  if (health.configured.length === 0) {
    return {
      card: { value: t("systemHealth.notSetUp"), href: setUpHref },
      panel: false,
    };
  }
  if (answersFromStandIn(health.configured)) {
    return {
      card: {
        value: t("systemHealth.providers.standIn"),
        tone: "info",
        detail: t(
          answersOnlyFromStandIn(health.configured)
            ? "systemHealth.providers.standInDetail"
            : "systemHealth.providers.standInPartialDetail",
        ),
        href: setUpHref,
      },
      panel: false,
    };
  }
  return {
    card: { value: t("systemHealth.providers.answering"), tone: "success" },
    panel: false,
  };
}

// The oldest stamp among the reports read, so "as of" never claims a reading
// fresher than the stalest one on the page.
export function oldestStamp(
  stamps: readonly (string | undefined)[],
): string | undefined {
  return stamps
    .filter((stamp): stamp is string => stamp !== undefined)
    .reduce<string | undefined>(
      (oldest, stamp) =>
        oldest === undefined || Date.parse(stamp) < Date.parse(oldest)
          ? stamp
          : oldest,
      undefined,
    );
}
