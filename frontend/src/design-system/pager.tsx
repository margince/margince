// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  formatNumber,
  identifierNumber,
  ordinalNumber,
} from "../format/format";
import { useLocale, useT } from "../i18n";
import { PAGE_SIZES, pagerSlots } from "./listtable.paging";
import { Select } from "./select";

/**
 * `Pager` is exported for a table of another kind that holds all its rows and
 * pages them in memory, the way a list's own footer does.
 *
 * The pager: the pages in hand as numbers, prev/next either side, and the page
 * size on the right. Next stays enabled on the last loaded page while the
 * cursor still has rows to give, which is how the set grows without a total.
 */
export function Pager({
  current,
  lastPage,
  hasMore,
  perPage,
  onGoto,
  onPerPage,
}: Readonly<{
  current: number;
  lastPage: number;
  hasMore: boolean;
  perPage: number;
  onGoto: (to: number) => void;
  onPerPage: (next: number) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className={`lt-foot${lastPage === 1 && !hasMore ? " single" : ""}`}>
      {/* A landmark, because this is navigation and a reader who jumps by region
          should find it as one. The numbers carry "Page 3" rather than a bare
          "3": out of the row's context a digit names nothing, and the row's
          context is exactly what a screen reader does not have. */}
      <nav className="lt-pager" aria-label={t("table.pagination")}>
        <button
          type="button"
          disabled={current === 1}
          onClick={() => onGoto(current - 1)}
        >
          {t("table.prev")}
        </button>
        {pagerSlots(current, lastPage).map((slot, index) =>
          typeof slot === "number" ? (
            <button
              type="button"
              key={slot}
              className={slot === current ? "on" : undefined}
              aria-current={slot === current ? "page" : undefined}
              // "page 1.234" would read as a fraction of a page rather than
              // the 1234th of them.
              aria-label={t("table.page", { number: ordinalNumber(slot) })}
              onClick={() => onGoto(slot)}
            >
              {ordinalNumber(slot)}
            </button>
          ) : (
            <span
              // Slots are positional: two of them can hold the same kind of
              // nothing, and neither carries an identity of its own.
              // biome-ignore lint/suspicious/noArrayIndexKey: the position IS the identity
              key={`${slot}-${index}`}
              className="lt-gap"
              aria-hidden="true"
            >
              {slot === "gap" ? "…" : ""}
            </span>
          ),
        )}
        <button
          type="button"
          disabled={current === lastPage && !hasMore}
          onClick={() => onGoto(current + 1)}
        >
          {t("table.next")}
        </button>
      </nav>
      <span className="lt-perpage">
        <Select
          aria-label={t("table.rowsPerPage")}
          value={identifierNumber(perPage)}
          onChange={(next) => onPerPage(Number(next))}
          options={PAGE_SIZES.map((size) => ({
            value: identifierNumber(size),
            label: t("table.perPage", { count: formatNumber(size, locale) }),
          }))}
        />
      </span>
    </div>
  );
}
