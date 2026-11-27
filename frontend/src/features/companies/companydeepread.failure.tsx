// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatDateTimeAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useViewerZone } from "../../lib/viewerzone";

type SiteReadReport = components["schemas"]["SiteReadReport"];

// Why a read failed, by the diagnosis the worker recorded. A code this build
// does not know reads the server's own sentence instead.
const SITE_READ_FAILURE_LABELS: Partial<
  Record<NonNullable<SiteReadReport["status_code"]>, MessageKey>
> = {
  bot_blocked: "deepread.failure.bot_blocked",
  http_server_error: "deepread.failure.http_server_error",
  timeout: "deepread.failure.timeout",
  http_client_error: "deepread.failure.http_client_error",
  dns: "deepread.failure.dns",
  tls: "deepread.failure.tls",
  robots_disallowed: "deepread.failure.robots_disallowed",
  unreadable: "deepread.failure.unreadable",
  internal: "deepread.failure.internal",
  stale_reclaim: "deepread.failure.stale_reclaim",
};

export function SiteReadFailure({
  report,
}: Readonly<{ report: SiteReadReport }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  if (report.status !== "failed") {
    return null;
  }
  const key = report.status_code
    ? SITE_READ_FAILURE_LABELS[report.status_code]
    : undefined;
  const reason = key ? t(key) : report.status_detail;
  if (!reason) {
    return null;
  }
  return (
    <p className="t-caption companydeepread-note">
      {reason}
      {report.next_attempt_at && (
        <>
          {" "}
          {t("deepread.resumesAt", {
            when: formatDateTimeAbbrev(
              report.next_attempt_at,
              locale,
              readerZone,
            ),
          })}
        </>
      )}
    </p>
  );
}
