// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronLeft, ChevronRight, type LucideIcon } from "lucide-react";
import {
  formatNumber,
  identifierNumber,
  ordinalNumber,
} from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  DESKTOP_MIN_WIDTH,
  usePhoneViewport,
  useViewportUnder,
} from "../lib/viewport";
import { type PageRange, rangeText } from "./listsurface";
import { PAGE_SIZES, PAGER_SLOTS, pagerSlots } from "./listtable.paging";
import { Select } from "./select";

/**
 * `Pager` is exported for a table of another kind that holds all its rows and
 * pages them in memory, the way a list's own footer does.
 *
 * A known end is numbered as the last page, and Next stops there. While the end
 * is not known a gap follows the pages in hand, and Next goes on.
 */
export function Pager({
  current,
  lastPage,
  hasMore,
  endKnown = !hasMore,
  asked,
  perPage,
  range,
  onGoto,
  onPerPage,
}: Readonly<{
  current: number;
  lastPage: number;
  hasMore: boolean;
  /** `lastPage` is the list's real end, so it is numbered. Default: no more rows. */
  endKnown?: boolean;
  /** The page being fetched; its number shows as busy until it is drawn. */
  asked?: number;
  perPage: number;
  /** The rows on screen, said beside the pages ("26-50 of 1,024"). */
  range: PageRange;
  onGoto: (to: number) => void;
  onPerPage: (next: number) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const phone = usePhoneViewport();
  const short = useViewportUnder(DESKTOP_MIN_WIDTH - 1);
  const slots = short ? PAGER_SLOTS.short : PAGER_SLOTS.wide;
  return (
    <div className={`lt-foot${lastPage === 1 && endKnown ? " single" : ""}`}>
      <div className="lt-footrow">
        {range.total > 0 && (
          <span className="lt-range">{rangeText(t, locale, range)}</span>
        )}
        {/* A landmark, because this is navigation and a reader who jumps by region
            should find it as one. The numbers carry "Page 3" rather than a bare
            "3": out of the row's context a digit names nothing, and the row's
            context is exactly what a screen reader does not have. */}
        <nav className="lt-pager" aria-label={t("table.pagination")}>
          <PageStep
            label={t("table.prev")}
            icon={short ? ChevronLeft : undefined}
            disabled={current === 1}
            onClick={() => onGoto(current - 1)}
          />
          {pagerSlots(current, lastPage, !endKnown, slots).map((slot, index) =>
            typeof slot === "number" ? (
              <button
                type="button"
                key={slot}
                className={slot === current ? "on" : undefined}
                aria-current={slot === current ? "page" : undefined}
                aria-busy={slot === asked ? true : undefined}
                // "page 1.234" would read as a fraction of a page rather than
                // the 1234th of them.
                aria-label={t("table.page", { number: ordinalNumber(slot) })}
                onClick={() => onGoto(slot)}
              >
                {ordinalNumber(slot)}
              </button>
            ) : (
              <span
                // A strip can hold two gaps, and neither has an identity of its own.
                // biome-ignore lint/suspicious/noArrayIndexKey: the position IS the identity
                key={`gap-${index}`}
                className="lt-gap"
                aria-hidden="true"
              >
                …
              </span>
            ),
          )}
          <PageStep
            label={t("table.next")}
            icon={short ? ChevronRight : undefined}
            disabled={endKnown && current >= lastPage}
            onClick={() => onGoto(current + 1)}
          />
        </nav>
        <span className="lt-perpage">
          <Select
            aria-label={t("table.rowsPerPage")}
            value={identifierNumber(perPage)}
            onChange={(next) => onPerPage(Number(next))}
            options={PAGE_SIZES.map((size) => ({
              value: identifierNumber(size),
              label: t("table.perPage", { count: formatNumber(size, locale) }),
              // The box's name says what the number counts.
              faceLabel: phone ? formatNumber(size, locale) : undefined,
            }))}
          />
        </span>
      </div>
    </div>
  );
}

/** Previous or Next: its word, or below a desktop its chevron named by the word. */
function PageStep({
  label,
  icon: Icon,
  disabled,
  onClick,
}: Readonly<{
  label: string;
  icon?: LucideIcon;
  disabled: boolean;
  onClick: () => void;
}>) {
  return Icon ? (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
    >
      <Icon size={16} aria-hidden="true" />
    </button>
  ) : (
    <button type="button" disabled={disabled} onClick={onClick}>
      {label}
    </button>
  );
}
