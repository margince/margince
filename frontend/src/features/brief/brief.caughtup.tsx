// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatTimeOfDay } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

// A clear morning, said once and warmly. Only a complete read may draw it:
// the caller keeps a partial or failed read on its own sentences.
export function CaughtUp({ asOf }: Readonly<{ asOf: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <div className="card brief-caughtup">
      <span className="brief-caughtup-mark" aria-hidden="true">
        <svg
          aria-hidden="true"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
        >
          <path pathLength={1} d="M20 6 9 17l-5-5" />
        </svg>
      </span>
      <h3 className="brief-caughtup-title">{t("brief.feed.caughtUp")}</h3>
      <p className="brief-caughtup-line">
        {t("brief.feed.caughtUpDetail", {
          time: formatTimeOfDay(asOf, locale, zone),
        })}
      </p>
    </div>
  );
}
