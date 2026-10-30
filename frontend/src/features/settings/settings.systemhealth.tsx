// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { useProviderHealth } from "../../lib/ai-provider-health";
import { useCan } from "../../lib/capability";
import { routeHash } from "../../lib/router";
import { useMe } from "../../lib/session";
import { settingsHref } from "../../lib/settingsrouting";
import { ProviderHealthCard } from "../capture/providerhealth";
import { AutomaticWorkHealthCard } from "./automaticworkhealth";
import { CaptureHealthCard, useCaptureHealth } from "./capturehealth";
import { DataTasksHealthCard } from "./datataskshealth";
import { EmbedReindexCard } from "./embedreindex";
import {
  useEmbedReindexAvailable,
  useEmbedReindexStatus,
} from "./embedreindex.queries";
import {
  ExtensionIngestHealthCard,
  useExtensionIngestHealth,
} from "./extingesthealth";
import { HealthStrip } from "./healthcard";
import { JobHealthCard, useJobHealth } from "./jobhealth";
import { BackgroundSchedulesCard, SendPacingCard } from "./operationsettings";
import { RecoveryHealthCard, useRecoveryHealth } from "./recoveryhealth";
import {
  type HealthReading,
  intakeReading,
  jobsReading,
  oldestStamp,
  providersReading,
  readHealth,
  searchIndexReading,
} from "./systemhealthreadings";

// The four readings that are mostly "all clear", one answer each, and when the
// reports were read. The queries are the cards' own, so a card drawn below
// shares its read with its slot.
function useReadings() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const words = { t, plural, locale };
  const probing = useMe().isPending;
  const modelsHref = routeHash(settingsHref("models"));

  const canReadIndex = useCan("embedding_reindex", "read");
  const bound = useEmbedReindexAvailable();
  const index = useEmbedReindexStatus(canReadIndex && bound);
  const jobs = useJobHealth();
  const intake = useExtensionIngestHealth();
  const canSeeProviders = useCan("ai_diagnostics", "read");
  const providers = useProviderHealth(canSeeProviders);

  // An installation with no embedding model has no index to report on, which
  // is a fact about its set-up rather than a fault.
  const search: HealthReading =
    canReadIndex && !bound && !probing
      ? {
          card: {
            value: t("systemHealth.notSetUp"),
            href: modelsHref,
          },
          panel: false,
        }
      : readHealth({ canSee: canReadIndex, probing, query: index }, t, (data) =>
          searchIndexReading(data, words),
        );
  const capture = useCaptureHealth();
  const recovery = useRecoveryHealth();
  // The search index and provider reports carry no stamp of their own, so the
  // time of their read stands in for it.
  const stamp = useStamp([
    generatedAt(jobs),
    generatedAt(intake),
    generatedAt(capture),
    generatedAt(recovery),
    fetchedAt(canReadIndex && bound, index),
    fetchedAt(canSeeProviders, providers),
  ]);
  return {
    stamp,
    search,
    jobs: readHealth({ ...jobs, probing }, t, (data) =>
      jobsReading(data, words),
    ),
    intake: readHealth({ ...intake, probing }, t, (data) =>
      intakeReading(data, words),
    ),
    providers: readHealth(
      { canSee: canSeeProviders, probing, query: providers },
      t,
      (data) => providersReading(data, words, modelsHref),
    ),
  };
}

function generatedAt(
  report: Readonly<{
    canSee: boolean;
    query: Readonly<{ data?: Readonly<{ generated_at: string }> }>;
  }>,
): string | undefined {
  return report.canSee ? report.query.data?.generated_at : undefined;
}

function fetchedAt(
  canSee: boolean,
  query: Readonly<{ dataUpdatedAt: number }>,
): string | undefined {
  return canSee && query.dataUpdatedAt > 0
    ? new Date(query.dataUpdatedAt).toISOString()
    : undefined;
}

// When the reports on this page were read, said once for all of them.
function useStamp(stamps: readonly (string | undefined)[]): string | undefined {
  const { locale } = useLocale();
  const oldest = oldestStamp(stamps);
  return oldest && formatDateTime(oldest, locale, viewerZone());
}

/**
 * The system health page, in a `settings.<page>.tsx` companion as the privacy
 * lanes are, so settings.tsx stays the dispatch.
 */
export function SystemHealthPage() {
  const t = useT();
  const readings = useReadings();
  return (
    <>
      <HealthStrip
        stamp={readings.stamp}
        slots={[
          {
            key: "search",
            label: t("embedreindex.title"),
            reading: readings.search,
          },
          { key: "jobs", label: t("settings.jobs"), reading: readings.jobs },
          {
            key: "intake",
            label: t("systemHealth.intake.label"),
            reading: readings.intake,
          },
          {
            key: "providers",
            label: t("settings.providerHealth"),
            reading: readings.providers,
          },
        ]}
      />
      {/* A slot's full card stands under the row only when there is more to
          read than its answer: a finding, a withheld grant, or rows. */}
      {readings.search.panel && <EmbedReindexCard />}
      {readings.jobs.panel && <JobHealthCard />}
      {readings.intake.panel && <ExtensionIngestHealthCard />}
      {readings.providers.panel && <ProviderHealthCard />}
      <CaptureHealthCard />
      <AutomaticWorkHealthCard />
      <RecoveryHealthCard />
      <DataTasksHealthCard />
      {/* The values the background work runs on, under the readings of what
          it is doing. */}
      <BackgroundSchedulesCard />
      <SendPacingCard />
    </>
  );
}
