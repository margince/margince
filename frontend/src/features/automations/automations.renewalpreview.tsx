// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatDate, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

export function RenewalPreviewDetails({
  renewal,
}: Readonly<{
  renewal: components["schemas"]["AutomationRenewalPreview"];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <div>
      <p className="t-caption">
        {t(renewal.recurs_yearly ? "auto.renewal.annual" : "auto.renewal.once")}
      </p>
      <p className="t-caption">
        {t("auto.renewal.missingDates", {
          n: formatNumber(renewal.missing_dates, locale),
        })}
      </p>
      {renewal.samples.map((sample) => (
        <p className="t-caption" key={sample.entity_id}>
          {t("auto.renewal.dates", {
            reminder: formatDate(sample.reminder_date, locale, zone),
            renewal: formatDate(sample.occurrence_date, locale, zone),
          })}
        </p>
      ))}
    </div>
  );
}
