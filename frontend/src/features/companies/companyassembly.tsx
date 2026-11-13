// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two speeds of the dossier and the growth fit. A stored reading answers
// at once, and a missing one is written on the request. The panel says which
// of the two the reader is waiting for.

import { CACHE_ANSWER_GRACE_MS } from "../../api/client";
import type { components } from "../../api/schema";
import { AiPending } from "../../design-system/aipending";
import { PendingBody } from "../../design-system/atoms";
import { useWaited } from "../../design-system/waited";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

type ServedFrom = components["schemas"]["AssemblyServedFrom"];

/**
 * AssemblyWait is the wait for a stored reading. A stored one answers inside
 * CACHE_ANSWER_GRACE_MS, so until then this is a plain load. Past it the
 * reading is being written, and `writing` says so.
 */
export function AssemblyWait({
  writing,
  lines,
}: Readonly<{ writing: string; lines: number }>) {
  const t = useT();
  const beingWritten = useWaited(CACHE_ANSWER_GRACE_MS);
  return beingWritten ? (
    <AiPending label={writing} lines={lines} />
  ) : (
    <PendingBody label={t("state.loading")} lines={lines} />
  );
}

/**
 * AssemblyStamp says when the reading on screen was written: just now for one
 * written on this request, its age for one the server had stored.
 */
export function AssemblyStamp({
  at,
  servedFrom,
}: Readonly<{ at: string; servedFrom?: ServedFrom }>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  return (
    <span className="t-caption">
      {servedFrom === "generated"
        ? t("co.assembly.writtenNow")
        : t("co.brief.generatedAt", {
            when: formatDateTime(at, locale, readerZone),
          })}
    </span>
  );
}
