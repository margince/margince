// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowDown, ArrowUp, ChevronRight, Sparkles } from "lucide-react";
import {
  Badge,
  Button,
  EmptyState,
  Kbd,
  SegmentedControl,
} from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import type { Translator } from "../i18n";
import { useT } from "../i18n";
import {
  type CoreSearchHitType,
  SEARCH_FILTER_KEY,
  SEARCH_HIT_ORDER,
} from "../lib/searchkinds";

type KindOption = CoreSearchHitType | "all";

function kindLabels(t: Translator): Record<KindOption, string> {
  return {
    all: t("palette.filterAll"),
    contact: t(SEARCH_FILTER_KEY.contact),
    company: t(SEARCH_FILTER_KEY.company),
    deal: t(SEARCH_FILTER_KEY.deal),
    deal_room: t(SEARCH_FILTER_KEY.deal_room),
    lead: t(SEARCH_FILTER_KEY.lead),
    project: t(SEARCH_FILTER_KEY.project),
    product: t(SEARCH_FILTER_KEY.product),
    offer_template: t(SEARCH_FILTER_KEY.offer_template),
    activity: t(SEARCH_FILTER_KEY.activity),
    tag: t(SEARCH_FILTER_KEY.tag),
  };
}

/**
 * The strip that narrows the palette to one type of record, each type with
 * the count of hits it has. Only the types the words found are offered, and
 * the one picked stays while the words change.
 */
export function PaletteFilter({
  value,
  counts,
  floors,
  onChange,
}: Readonly<{
  value: CoreSearchHitType | null;
  counts: Partial<Record<CoreSearchHitType, number>>;
  floors: Partial<Record<CoreSearchHitType, boolean>>;
  onChange: (next: CoreSearchHitType | null) => void;
}>) {
  const t = useT();
  const types = SEARCH_HIT_ORDER.filter(
    (type) => (counts[type] ?? 0) > 0 || type === value,
  );
  const options: KindOption[] = ["all", ...types];
  const total = types.reduce((sum, type) => sum + (counts[type] ?? 0), 0);
  return (
    <div className="palette-filter">
      <SegmentedControl
        options={options}
        value={value ?? "all"}
        onChange={(next) => onChange(next === "all" ? null : next)}
        labels={kindLabels(t)}
        counts={{ ...counts, all: total }}
        floors={{ ...floors, all: types.some((type) => floors[type]) }}
        label={t("palette.filter")}
      />
    </div>
  );
}

/**
 * The heading over the recent rows, with the button that forgets them; none
 * without an `id`. It stands above the listbox, which holds options only, and
 * names their group by that `id`.
 */
export function PaletteRecentHeading({
  id,
  onClear,
}: Readonly<{ id: string | undefined; onClear: () => void }>) {
  const t = useT();
  if (id === undefined) {
    return null;
  }
  return (
    <Eyebrow
      as="h2"
      id={id}
      size="xxsmall"
      className="palette-group"
      action={
        <Button variant="link" onClick={onClear}>
          {t("palette.clearRecent")}
        </Button>
      }
    >
      {t("palette.recent")}
    </Eyebrow>
  );
}

/**
 * The drawer that asks one set of the reader's choosing, outside the rows so
 * Enter on a screen name never opens it. It carries what is typed into the
 * box rather than asking it. It closes the Ask Margince group, and heads it
 * when no question row above already did.
 */
export function PaletteAskDocuments({
  heading,
  onAsk,
}: Readonly<{ heading: string | null; onAsk: () => void }>) {
  const t = useT();
  return (
    <>
      {heading && (
        <Eyebrow as="h2" size="xxsmall" className="palette-group">
          {heading}
        </Eyebrow>
      )}
      <button
        type="button"
        className="palette-row palette-ask t-body"
        onClick={onAsk}
      >
        <span className="palette-tile palette-tile-ai">
          <Sparkles aria-hidden />
        </span>
        <span className="palette-text">
          <span className="label">{t("corpusAsk.title")}</span>
        </span>
        <Badge tone="ai">{t("palette.ask")}</Badge>
        <ChevronRight aria-hidden className="palette-chevron" />
      </button>
    </>
  );
}

/** The keys the palette answers to, under the list. */
export function PaletteKeys() {
  const t = useT();
  return (
    <div className="palette-foot t-caption" aria-hidden>
      <span>
        <Kbd>
          <ArrowUp />
        </Kbd>
        <Kbd>
          <ArrowDown />
        </Kbd>
        {t("palette.keyMove")}
      </span>
      <span>
        <Kbd>{"Enter"}</Kbd>
        {t("palette.keyOpen")}
      </span>
      <span>
        <Kbd>{"Esc"}</Kbd>
        {t("palette.keyClose")}
      </span>
    </div>
  );
}

// Words the search answered with no record are named. A list with nothing in
// it says so only once no search is still out or has failed.
export function PaletteEmpty({
  query,
  nothingFound,
  listed,
  search,
}: Readonly<{
  query: string;
  nothingFound: boolean;
  listed: number;
  search: Readonly<{
    pending: boolean;
    failed: boolean;
    timedOut: readonly string[];
  }>;
}>) {
  const t = useT();
  if (nothingFound) {
    return <EmptyState>{t("search.empty", { q: query })}</EmptyState>;
  }
  if (
    listed > 0 ||
    search.pending ||
    search.failed ||
    search.timedOut.length > 0
  ) {
    return null;
  }
  return <EmptyState>{t("palette.empty")}</EmptyState>;
}
