// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the queue is NOT showing, and which rule is holding it back.
//
// The Worklist is built to look finite, which is what makes it worth working:
// a rep reaches the bottom and the day is done. The cost of that design is that
// a queue hiding real work looks exactly like a queue with none, so the page
// cannot report its own worst failure. This is the surface that can.
//
// A titled panel among the LEAD's panels, in the team board's own chrome, and
// that is the honest placement rather than a hedge. A rep whose job is to work
// the queue is not the contact who acts on a horizon that is set wrong, so this
// sits with the rest of a lead's read of the team rather than over the day; and
// it stands open there, because a guardrail folded shut is one nobody checks.
// On a healthy installation the whole panel is one sentence saying nothing is
// held back, which is a cheap thing to draw and the only thing worth reading.

import { useState } from "react";
import { Disclosure } from "../design-system/atoms";
import { EmailEntry } from "../design-system/emailentry";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import {
  type HiddenBacklog,
  type HiddenRule,
  useHiddenBacklog,
  useHiddenBacklogRows,
} from "./worklist.hidden.queries";
import { AFTER_THE_DAY } from "./worklist.layout";
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
  onOpenEmail,
}: Readonly<{
  enabled: boolean;
  /** Opens a held-back message in the page's own email drawer. */
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
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
    <Panel
      className={AFTER_THE_DAY}
      title={t("worklist.hidden.title")}
      // What the queue itself carries, against what is held back above it. It
      // belongs to the whole panel rather than to any one rule's row, which is
      // the footer band's own job — and it is a fact about the queue rather
      // than a caveat about this read, so it stays out of the body where the
      // truncation sentence has to come FIRST.
      //
      // Only over a read that ANSWERED. The cached backlog outlives the refetch
      // that failed, so a footer taken off the payload alone reported the last
      // good figure in this band while the body said the guardrail could not be
      // read — and a number standing beside an unreadable result is read as the
      // answer, which is the one failure this panel exists to prevent.
      footer={
        state === "ready" && hidden.data && !hidden.data.clear
          ? t("worklist.hidden.shown", {
              count: formatNumber(hidden.data.shown, locale),
            })
          : undefined
      }
    >
      <PanelBody>
        <SurfaceState
          state={state}
          loadingLabel={t("worklist.hidden.loading")}
          emptyLabel={t("worklist.hidden.clear")}
        >
          {hidden.data && !hidden.data.clear && (
            <HiddenFigures backlog={hidden.data} onOpenEmail={onOpenEmail} />
          )}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

// One figure per hiding rule.
//
// Exported for its story: the panel above fetches, so a story that mounted it
// would draw a loading skeleton and never the readings it exists to show.
//
// What the QUEUE carries is not here. That figure describes the whole panel
// rather than any one rule, so it rides the panel's own footer band — the one
// place a reader learns to look for a section's total.
export function HiddenFigures({
  backlog,
  onOpenEmail,
}: Readonly<{
  backlog: HiddenBacklog;
  onOpenEmail: (activityId: string) => void;
}>) {
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
          rule="past_horizon"
          onOpenEmail={onOpenEmail}
          label={t("worklist.hidden.pastHorizon")}
          detail={t("worklist.hidden.pastHorizon.detail")}
          locale={locale}
          measured={!backlog.truncated}
          t={t}
        />
        <Reading
          count={backlog.unlinked}
          rule="unlinked"
          onOpenEmail={onOpenEmail}
          label={t("worklist.hidden.unlinked")}
          detail={t("worklist.hidden.unlinked.detail")}
          locale={locale}
          measured={!backlog.truncated}
          t={t}
        />
        <Reading
          count={backlog.colleagues}
          rule="colleagues"
          onOpenEmail={onOpenEmail}
          label={t("worklist.hidden.colleagues")}
          detail={t("worklist.hidden.colleagues.detail")}
          locale={locale}
          measured={!backlog.truncated}
          t={t}
        />
        {/* A model's judgement sits between the two: nobody chose it, and
            it can still be wrong about a customer. */}
        <Reading
          count={backlog.informs_us}
          rule="informs_us"
          onOpenEmail={onOpenEmail}
          label={t("worklist.hidden.informsUs")}
          detail={t("worklist.hidden.informsUs.detail")}
          locale={locale}
          measured={!backlog.truncated}
          t={t}
        />
        <Reading
          count={backlog.not_sales}
          rule="not_sales"
          onOpenEmail={onOpenEmail}
          label={t("worklist.hidden.notSales")}
          detail={t("worklist.hidden.notSales.detail")}
          locale={locale}
          measured={!backlog.truncated}
          t={t}
        />
        <Reading
          count={backlog.set_aside}
          rule="set_aside"
          onOpenEmail={onOpenEmail}
          label={t("worklist.hidden.setAside")}
          detail={t("worklist.hidden.setAside.detail")}
          locale={locale}
          measured={!backlog.truncated}
          t={t}
        />
      </ul>
    </div>
  );
}

// One figure. Drawn only when it found something: a list of zeros says nothing
// a reader can act on, and the clear case above already covers "all of them".
//
// A figure opens onto the messages behind it. They are read only once it is
// opened, so a lead who never asks costs nothing.
function Reading({
  rule,
  count,
  label,
  detail,
  locale,
  measured,
  onOpenEmail,
  t,
}: Readonly<{
  rule: HiddenRule;
  count: number;
  label: string;
  detail: string;
  locale: Locale;
  // False when the read hit its limit: every figure is then a difference of
  // two capped reads, so a zero means "not counted" rather than "none".
  measured: boolean;
  onOpenEmail: (activityId: string) => void;
  t: Translator;
}>) {
  const [open, setOpen] = useState(false);
  if (count === 0 && measured) {
    return null;
  }
  return (
    <li>
      <Disclosure
        open={open}
        onToggle={setOpen}
        summary={
          <span className="worklist-hidden-row">
            <span className="worklist-hidden-count">
              {count === 0
                ? t("worklist.hidden.notCounted")
                : t("worklist.hidden.count", {
                    count: formatNumber(count, locale),
                  })}
            </span>
            <span>{label}</span>
            <span className="t-caption worklist-hidden-detail">{detail}</span>
          </span>
        }
      >
        <HiddenRows rule={rule} open={open} onOpenEmail={onOpenEmail} />
      </Disclosure>
    </li>
  );
}

// The messages one rule holds back. An email is the canonical email row and
// opens in the page's drawer; a channel message has no email shape and is
// named by its subject.
function HiddenRows({
  rule,
  open,
  onOpenEmail,
}: Readonly<{
  rule: HiddenRule;
  open: boolean;
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = useHiddenBacklogRows(rule, open);
  const state = rows.isPending
    ? "loading"
    : rows.isError
      ? "unavailable"
      : rows.data.rows.length === 0
        ? "empty"
        : "ready";
  return (
    <SurfaceState
      state={state}
      loadingLabel={t("worklist.hidden.rows.loading")}
      emptyLabel={t("worklist.hidden.rows.empty")}
    >
      <ul className="worklist-hidden-rows">
        {rows.data?.rows.map((row) => (
          <li key={row.activity_id}>
            {row.email_summary ? (
              <EmailEntry
                summary={row.email_summary}
                timestamp={formatDateTime(row.since, locale, viewerZone())}
                onOpen={() => onOpenEmail(row.activity_id)}
              />
            ) : (
              <span className="t-body">
                {row.subject} ·{" "}
                {formatDateTime(row.since, locale, viewerZone())}
              </span>
            )}
          </li>
        ))}
      </ul>
    </SurfaceState>
  );
}
