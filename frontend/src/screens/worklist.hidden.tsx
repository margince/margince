// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the queue is NOT showing, and which rule is holding it back.
//
// The Worklist is built to look finite, which is what makes it worth working:
// a rep reaches the bottom and the day is done. The cost of that design is that
// a queue hiding real work looks exactly like a queue with none, so the page
// cannot report its own worst failure. This is the surface that can.
//
// Behind a disclosure, closed by default, and that is the honest placement
// rather than a hedge. On a healthy installation every figure is zero, and a
// row of zeros drawn above the queue every morning would train a reader to
// scroll past the one morning it is not — while a rep whose job is to work the
// queue is not the person who acts on a horizon that is set wrong.

import { Disclosure } from "../design-system/atoms";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { type HiddenBacklog, useHiddenBacklog } from "./worklist.queries";
import "./worklist.css";

/**
 * The guardrail, drawn for a reader who can act on it.
 *
 * `enabled` carries the same tier the team board's does: this is a lead's
 * reading, not a rep's, and a seat with no route to it should not fire the
 * request.
 *
 * The SERVER refuses below that tier too, which is what makes this a courtesy
 * rather than the control. It was the control until the endpoint gained its own
 * gate, and a permission held only in the browser is one an unmodified client
 * enforces and nothing else does.
 */
export function HiddenBacklogPanel({
  enabled,
}: Readonly<{ enabled: boolean }>) {
  const t = useT();
  const hidden = useHiddenBacklog(enabled);
  if (!enabled) {
    return null;
  }
  // A guardrail that could not be read says so rather than drawing zeros. Zeros
  // are its healthy answer, so a failed read rendered as zeros would report
  // perfect health at the moment the check stopped working — the same failure
  // in the client that `truncated` guards against in the server.
  const state = hidden.isPending
    ? "loading"
    : hidden.isError
      ? "unavailable"
      : // A clear backlog IS this surface's empty state, so it is drawn by the
        // same component that draws every other empty section rather than by a
        // paragraph of its own. `clear` is the server's own flag and already
        // accounts for a truncated read, so an empty page here cannot mean
        // "nothing found" over a scan that stopped early.
        hidden.data?.clear
        ? "empty"
        : "ready";
  return (
    <Disclosure summary={t("worklist.hidden.title")}>
      <SurfaceState
        state={state}
        loadingLabel={t("worklist.hidden.loading")}
        emptyLabel={t("worklist.hidden.clear")}
      >
        {hidden.data && !hidden.data.clear && (
          <HiddenFigures backlog={hidden.data} />
        )}
      </SurfaceState>
    </Disclosure>
  );
}

// The four figures.
//
// Exported for its story: the panel above fetches, so a story that mounted it
// would draw a loading skeleton and never the readings it exists to show.
export function HiddenFigures({
  backlog,
}: Readonly<{ backlog: HiddenBacklog }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className="worklist-hidden">
      {backlog.truncated && (
        // FIRST, and stated before any number. Every figure below is a floor
        // when the read was cut, so a reader who saw the counts before the
        // caveat would already have drawn a conclusion from them.
        <p className="t-body worklist-hidden-truncated">
          {t("worklist.hidden.truncated")}
        </p>
      )}
      <ul className="worklist-hidden-list">
        {/* Ordered by whose fault it is, not by size. The two nobody chose come
            FIRST: a wait that fell off on a date with no rep having judged
            anything is the failure this reading exists to surface, and sorting
            by count would bury it under an ordinary week of snoozes. */}
        <Reading
          count={backlog.past_horizon}
          label={t("worklist.hidden.pastHorizon")}
          detail={t("worklist.hidden.pastHorizon.detail")}
          locale={locale}
          t={t}
        />
        <Reading
          count={backlog.unlinked}
          label={t("worklist.hidden.unlinked")}
          detail={t("worklist.hidden.unlinked.detail")}
          locale={locale}
          t={t}
        />
        <Reading
          count={backlog.not_sales}
          label={t("worklist.hidden.notSales")}
          detail={t("worklist.hidden.notSales.detail")}
          locale={locale}
          t={t}
        />
        <Reading
          count={backlog.set_aside}
          label={t("worklist.hidden.setAside")}
          detail={t("worklist.hidden.setAside.detail")}
          locale={locale}
          t={t}
        />
      </ul>
      <p className="t-caption worklist-hidden-shown">
        {t("worklist.hidden.shown", {
          count: formatNumber(backlog.shown, locale),
        })}
      </p>
    </div>
  );
}

// One figure. Drawn only when it found something: a list of zeros says nothing
// a reader can act on, and the clear case above already covers "all of them".
function Reading({
  count,
  label,
  detail,
  locale,
  t,
}: Readonly<{
  count: number;
  label: string;
  detail: string;
  locale: Locale;
  t: Translator;
}>) {
  if (count === 0) {
    return null;
  }
  return (
    <li className="worklist-hidden-row">
      <span className="worklist-hidden-count">
        {t("worklist.hidden.count", { count: formatNumber(count, locale) })}
      </span>
      <span className="worklist-hidden-label">{label}</span>
      <span className="t-caption worklist-hidden-detail">{detail}</span>
    </li>
  );
}
