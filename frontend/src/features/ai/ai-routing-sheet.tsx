// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { ModelCatalogue } from "../../lib/ai-models";
import { useViewerZone } from "../../lib/viewerzone";

// What the model lists in the editor are, and how to move them on. The sheet is
// a SNAPSHOT somebody took on a day; undated it reads as "these are the models",
// and the refresh is the way past it.
//
// The profile is a reading, never a control here: it decides which vendors a
// lane may name, and a Save it refuses says so.
export function SheetFooter({
  catalogue,
  profile,
}: Readonly<{ catalogue: ModelCatalogue; profile: string }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const asOf = sheetAsOf(catalogue);
  return (
    <div className="ai-sheet-age">
      <span className="t-caption">
        {asOf
          ? t("aiRouting.sheetAsOf", {
              date: formatDate(asOf, locale, readerZone),
            })
          : t("aiRouting.sheetUnknown")}
      </span>
      <span className="t-caption" data-testid="ai-routing-profile">
        {t("aiRouting.profileLine", { profile })}
      </span>
    </div>
  );
}

// The day the price sheet was last written, which is the day its model list was
// last true. A sheet is re-priced row by row, so that day is its newest
// effective date.
function sheetAsOf(catalogue: ModelCatalogue): string | null {
  return (catalogue ?? []).reduce<string | null>(
    (latest, rate) =>
      latest === null || rate.effective_date > latest
        ? rate.effective_date
        : latest,
    null,
  );
}
