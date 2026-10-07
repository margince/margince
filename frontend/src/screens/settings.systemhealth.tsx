// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CaptureHealthCard } from "./capturehealth";
import { EmbedReindexCard } from "./embedreindex";
import { ExtensionIngestHealthCard } from "./extingesthealth";
import { JobHealthCard } from "./jobhealth";
import { BackgroundSchedulesCard, SendPacingCard } from "./operationsettings";
import { ProviderHealthCard } from "./providerhealth";

/**
 * The system health page's cards. Extracted from settings.tsx, which is frozen
 * at its length, for the reason the privacy lanes were.
 */
export function SystemHealthPage() {
  return (
    <>
      {/* A reindex that costs tokens, then a read of what the background
          system is holding: they hid beside the custom-field editor. */}
      <EmbedReindexCard />
      <JobHealthCard />
      {/* How often that work is scheduled and how fast mail leaves, under the
          reading of what it is holding. */}
      <BackgroundSchedulesCard />
      <SendPacingCard />
      {/* Beside the queue reading, not under Capture or Extensions: each
          answers "is something broken in the background". */}
      <CaptureHealthCard />
      <ExtensionIngestHealthCard />
      <ProviderHealthCard />
    </>
  );
}
