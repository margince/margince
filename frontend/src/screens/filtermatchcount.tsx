// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { type PluralBase, useLocale, usePlural, useT } from "../i18n";

/**
 * The count, and whether it is behind.
 *
 * Four readings, and keeping them apart is the point. A count the server has
 * answered reads plainly. A count being recomputed reads as the LAST answer,
 * marked stale — not as a spinner, because a number that vanishes on every
 * keystroke is harder to read than one that lags a moment. A tree with no
 * complete clause has no count at all, which is different from a count of zero:
 * zero means "nothing matches", and this means "you have not asked yet". And a
 * count the server was asked for and refused says exactly that.
 *
 * The refusal outranks the other three. It is read first because the previous
 * answer survives a failed refetch, so a stale number would otherwise be
 * presented as current, and because "you have not asked yet" over a finished
 * clause blames the reader for the server's refusal.
 */
export function MatchCount({
  label,
  count,
  stale,
  failed,
}: Readonly<{
  label: PluralBase;
  count: number | undefined;
  stale: boolean;
  failed: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (failed) {
    // Silent: the results card below carries the reason in an assertive live
    // region, and announcing the same failure twice fragments it.
    return (
      <span className="filters-count">
        <ErrorLine inline standing>
          {t("filters.countUnavailable")}
        </ErrorLine>
      </span>
    );
  }
  if (count === undefined) {
    return <span className="filters-count">{t("filters.noFilterYet")}</span>;
  }
  return (
    <span
      className="filters-count"
      // Spoken, because the count changing is the feedback for every edit — a
      // sighted reader sees the number move and a screen-reader user would
      // otherwise get nothing back from adding a clause.
      role="status"
      aria-busy={stale}
      data-stale={stale ? "true" : undefined}
    >
      {plural(label, count, { count: formatNumber(count, locale) })}
    </span>
  );
}
