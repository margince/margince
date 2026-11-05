// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronRight } from "lucide-react";
import {
  type MouseEvent,
  type RefObject,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import {
  MarginceCoreScene,
  type MarginceCoreState,
} from "../design-system/margince-core";
import { usePrefersReducedMotion } from "../design-system/motion";
import { formatMoney } from "../format/format";
import { useNow } from "../format/now";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import type { Route } from "../lib/router";
import { usePhoneViewport } from "../lib/viewport";
import { clearAgentEdge, publishAgentEdge } from "./agent-edge-signal";
import { useAgentFault } from "./agent-fault";
import { RUNNING } from "./agentrail-copy";
import { RailLine } from "./agentrail-line";
import type { Signals } from "./agentrail-reads";
import { useAiSpend, useLastCall, useSignals } from "./agentrail-reads";
import {
  type RailWords,
  railWords,
  restingReadings,
  restingTips,
  stillNews,
  useRestingLine,
} from "./agentrail-resting";
import { useAgentTicker } from "./agentrail-ticker";
import { useAiActivity } from "./ai-activity";
import { plain, type SpokenLine, speak, spokenText } from "./ai-activity-speak";
import { usePopoverDismiss } from "./popover";
import "./agentrail.css";

// The agent's place in the app: the foot of the workspace rail, under the
// destinations, carrying the Core as its status light.
//
// This is the ONE agent surface. Two others were built and judged first, a dock
// beside the page title and a bar across the foot of the viewport, and both put
// the agent in the content column, which is the column the reader is working in.
// Both then had to answer a question every other always-present thing answers
// with the rail: an agent that is always there belongs in the chrome that is
// always there.
//
// The rail also settles the SPLIT the bar was built to argue for. What the agent
// has on the page you are standing on, and what it is doing everywhere else, are
// two readings of different scope, and a horizontal bar spent its whole width
// keeping them apart. Stacked in a column they are simply two lines, and the
// panel that opens beside them carries the detail of both.
//
// EVERYTHING IT REPORTS IS READ FROM THE API: approvals waiting, which sources
// are unreachable, the model the last call actually ran on, this account's own
// suggestions. Nothing is a zero standing in for a read that has not answered:
// a row whose read is pending, or that this seat may not make, is absent
// instead. Nothing here can be put into a state by hand: the section reaches a
// state because something was read, or it does not reach it at all.
import { AgentPanel } from "./agentpanel";
import { looseStyle, usePanelFrame } from "./agentpanelframe";
import { barLine, causeLine, derive } from "./agentreading";

/**
 * How far the import has got, drawn as the ring around the orb.
 *
 * The ring is the IMPORT's and not the state's: it is drawn whenever mail is
 * being taken in and the orb is live, whichever occurrence the feed named for
 * the colour, because a morning brief starting mid-import does not stop the
 * import. It goes with a fault, though — a red or amber orb wearing a
 * progress ring would say the work is going well and badly at once — and it is
 * absent without a denominator, since a ring at a guessed share is a ring drawn
 * wrong. `undefined` is what the Core reads as "no ring".
 */
function importRing(
  state: MarginceCoreState,
  capture: Signals["capture"],
): number | undefined {
  if (capture === null || !RUNNING.has(state)) {
    return undefined;
  }
  return capture.progress.fraction ?? undefined;
}

/**
 * The block's one click, wherever on it a pointer lands.
 *
 * The words stand beside the button rather than inside it, because the record a
 * line names is a link and a link inside a button is a control inside a control.
 * So the toggle listens on the whole hit area, and the one exception is the
 * record link itself — following it is the reader leaving for the record.
 *
 * Opening the panel is also what acknowledges a broken run: it is the reader
 * turning to the agent's report. Until then the orb holds it.
 */
function useHit(
  open: boolean,
  setOpen: (next: (current: boolean) => boolean) => void,
  acknowledge: () => void,
): (event: MouseEvent<HTMLElement>) => void {
  return useCallback(
    (event: MouseEvent<HTMLElement>) => {
      if (
        event.target instanceof Element &&
        event.target.closest("a") !== null
      ) {
        return;
      }
      if (!open) {
        acknowledge();
      }
      setOpen((current) => !current);
    },
    [open, setOpen, acknowledge],
  );
}

/**
 * What the one button on this rail is called, spend included.
 *
 * A function rather than an expression in the component because the component
 * is already at the complexity ceiling, and because the two conditions that
 * decide whether there IS a figure — this seat may read it, and something in
 * the month carried a price — have to be asked the same way here as they are
 * for the box below. Two spellings of "is there a spend" is how the name and
 * the contents come to disagree.
 */
function railHitLabel(
  open: boolean,
  spend: Readonly<{ allowed: boolean; minor?: number }>,
  money: string,
  t: Translator,
): string {
  const name = t(open ? "agent.rail.close" : "agent.rail.open");
  return spend.allowed && spend.minor !== undefined
    ? `${name}. ${t("agent.rail.spend")}: ${money}`
    : name;
}

/**
 * The one line under the orb, changing without a cut.
 *
 * This slot changes on its own — one reading every `IDLE_HOLD_MS` at rest,
 * several a second while the agent works — and a sentence replaced in place is
 * a hard cut in the corner of somebody's eye all day. So the two sentences
 * overlap: the outgoing one stays for a `--dur-enter`, fading, over the
 * incoming one fading in. Opacity only, and the outgoing layer is out of the
 * flow, so the two-line room holds still and nothing under it moves.
 *
 * A sentence is the SAME sentence when its words are, not when its object is.
 * The block re-renders on every read this tab makes, and identity by object
 * would fade the same four words at a reader several times a second.
 *
 * At most one outgoing layer, ever. A change arriving mid-fade drops whatever
 * was leaving and gives the slot to the sentence that was on screen: the
 * working state ticks faster than the fade is long, and a queue of them would
 * still be playing this second's news a minute from now.
 *
 * Reduced motion gets the END state, which for a crossfade is the new sentence
 * by itself: no outgoing layer is mounted at all, so there is none to retire.
 */
export function RailSaying({ line }: Readonly<{ line: SpokenLine }>) {
  const said = spokenText(line);
  const reduced = usePrefersReducedMotion();
  const [held, setHeld] = useState({ said, line });
  const [leaving, setLeaving] = useState<SpokenLine | null>(null);
  if (held.said !== said) {
    // Adjusted while rendering rather than in an effect: both layers have to be
    // in the DOM in the SAME commit, and an effect would paint one frame of the
    // new sentence alone first — which is the cut this exists to remove.
    setLeaving(reduced ? null : held.line);
    setHeld({ said, line });
  } else if (reduced && leaving !== null) {
    // The preference turned on WHILE a fade was running, which is a re-render
    // the words did not change in. What retires the outgoing layer is its own
    // animation ending, and `@media (prefers-reduced-motion: reduce)` in
    // agentrail.css has just set `animation: none` on it — so no
    // `animationend` is ever delivered and the old sentence would stand over
    // the new one for the rest of the session. Snapping is this preference's
    // end state, so drop the layer here rather than wait for an event the
    // stylesheet has cancelled.
    setLeaving(null);
  }
  return (
    <span className="arswap">
      {/* Keyed on the words: a new element is what replays the fade, the same
          way an inserted block arrives in design-system/enter.css. */}
      <span className="arline" key={said}>
        <RailLine line={line} />
      </span>
      {leaving !== null && (
        <span
          className="argone"
          key={spokenText(leaving)}
          // Announced once, by the layer underneath. `inert` as well as
          // `aria-hidden`, because the sentence on its way out can carry the
          // record's link and a hidden control that still takes Tab is a trap.
          aria-hidden="true"
          inert
          // The fade ending is what retires it, so the duration lives in the
          // stylesheet alone and no timer here can disagree with it.
          onAnimationEnd={() => setLeaving(null)}
        >
          <RailLine line={leaving} />
        </span>
      )}
    </span>
  );
}

export function AgentRail({
  route,
  bar,
}: Readonly<{
  route: Route;
  /**
   * The bottom bar this block is a cell of, at phone width.
   *
   * Only the panel needs it, and only there: it spans the bar rather than
   * insetting itself, so the two read as one object. Absent on the sidebar,
   * where the panel stands beside the card and the bar does not exist.
   */
  bar?: RefObject<HTMLElement | null>;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  // The hit area around the button: on the phone bar it is the round well the
  // panel is measured from, where the button inside it is only the ball.
  const well = useRef<HTMLDivElement>(null);
  const panel = useRef<HTMLElement>(null);
  const block = useRef<HTMLElement>(null);
  const phone = usePhoneViewport();
  const signals = useSignals();
  const model = useLastCall();
  const server = useAiActivity();
  const ticker = useAgentTicker();
  const spend = useAiSpend();
  const { locale } = useLocale();
  // The rail's own words, bundled once per render: the queue's line is a COUNT
  // and goes through the reader's plural rule rather than a pasted number.
  const plural = usePlural();
  const said: RailWords = railWords(t, plural, locale);
  const { fault, acknowledge } = useAgentFault(server.faults);
  const { state, cause, register } = derive(signals, server, fault);
  const hit = useHit(open, setOpen, acknowledge);

  // What the screen's margins draw, published rather than re-derived: the reads
  // above are all local to this component, so a second consumer calling the same
  // hooks would get a second set of them and report on those.
  // The unanswered queue is NOT part of it: it reaches a reader through this
  // panel's own line, with its count, rather than as a ring around the window
  // that stands for as long as the queue does.
  const reading = RUNNING.has(state);
  useEffect(() => {
    publishAgentEdge({ reading, register });
  }, [reading, register]);
  // The last word belongs to the unmount: a reading left behind would outlive the
  // session that made it, and the signed-out screen would inherit a lit margin.
  useEffect(() => clearAgentEdge, []);

  // Put focus back on the block only when the panel actually HELD it: an outside
  // click usually lands on something focusable of its own, and pulling focus
  // back after it would undo what the click just did.
  const dismiss = useCallback(() => {
    const held = panel.current?.contains(document.activeElement) ?? false;
    setOpen(false);
    if (held) {
      trigger.current?.focus();
    }
  }, []);
  usePopoverDismiss(open, panel, dismiss);

  const frame = usePanelFrame(block, well, bar, open, phone);
  const money =
    spend.minor === undefined
      ? ""
      : formatMoney(spend.minor, spend.currency, locale);
  const hitLabel = railHitLabel(open, spend, money, t);
  // Above the early return with every other hook: a screen this section draws
  // nothing on is still a render it has to make the same calls in.
  //
  // The runs that settled recently, for the rotation; the newest live one, for
  // the bar. The bar keeps the live run because that is what is true this
  // second. Every line in one pass of the rotation shares one reading of the
  // clock, so they cannot disagree about which runs are still news.
  const now = useNow(60_000);
  const settled = stillNews(server.recent, now).flatMap((item) => {
    const said = speak(item, t);
    return said === null ? [] : [said];
  });
  const resting = useRestingLine(
    restingReadings(
      {
        waiting: signals.waiting,
        sourcesUnknown: signals.sourcesUnknown,
        developmentLine:
          signals.ai === "development" ? t("auth.coreDevelopment") : null,
        settled,
      },
      said,
    ),
    restingTips(route.screen, t, navigator.platform),
  );

  // Two things can hold the line, and this is their order: whatever the state
  // itself has to say, because a fault outranks small talk, and at rest the
  // rotation of true readings.
  //
  // The named occurrence is the one that put the orb where it is, so the colour
  // and the sentence can never be about two different runs.
  const agentLine = causeLine(cause, t);
  const line =
    state === "idle"
      ? resting
      : barLine(state, signals, t("auth.coreDevelopment"), agentLine, said);
  // ONE line under the orb, whoever is talking. While this tab is fetching
  // something it can name, that sentence is the orb's line — "Loading Acme" is
  // the status a reader is waiting on at that moment — and the agent's own line
  // has the slot back the instant the read settles. Two stacked lines read as
  // two statuses, and a reader asked which one was the status.
  //
  // A fault keeps the slot regardless: the colour and the caption are always
  // about the same thing, and an amber orb captioned "Loading pipeline"
  // tells a reader the pipeline is the fault.
  const holdsFault = state === "warning" || state === "error";
  const [latest] = ticker;
  const shown = latest !== undefined && !holdsFault ? plain(latest.said) : line;
  const ring = importRing(state, signals.capture);
  return (
    <section
      className="arblock"
      data-core-state={state}
      aria-label={t("agent.rail.region")}
      ref={block}
    >
      {/* Out of the rail and into the body: the rail clips what hangs beside it
          (usePanelFrame). The tone the panel is dressed in travels with it,
          because a portalled element inherits nothing from where it came from. */}
      {open &&
        frame &&
        createPortal(
          <div
            className="arloose"
            data-core-state={state}
            style={looseStyle(frame)}
          >
            <AgentPanel
              state={state}
              signals={signals}
              model={model}
              panel={panel}
              frame={frame}
              line={line}
              running={server.running}
              unnamed={server.unnamed}
              settled={server.answered ? server.recent : undefined}
              spend={spend}
            />
          </div>,
          document.body,
        )}
      {/* One hit area carries the whole block, and the orb inside it is the
          button: the accessible name and the expanded state. The words stand
          BESIDE the button rather than in it, because the record a line names
          is a link, and a link inside a button is a control inside a control
          (the same reason the CTA underneath keeps its own row). The pointer
          still gets the whole block through `useHit`; the keyboard gets the
          button. */}
      {/* biome-ignore lint/a11y/noStaticElementInteractions: the button inside is the interactive element; this only widens its pointer target to the words and chevron beside it */}
      {/* biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard's toggle is the button's, whose Enter and Space arrive here as the same click */}
      <div className="arhit" onClick={hit} ref={well}>
        <button
          type="button"
          className="artoggle"
          ref={trigger}
          aria-expanded={open}
          // The spend joins the NAME, not only the box.
          //
          // An `aria-label` replaces everything inside the button, and on the
          // collapsed rail the stylesheet hides `.arwords` too, so there the
          // figure and its scope reach a screen reader from nowhere else. A
          // figure somebody is accountable for cannot be the half of this
          // control that only some readers get.
          aria-label={hitLabel}
        >
          <MarginceCoreScene
            state={state}
            feed={false}
            progress={ring}
            size="md"
            className="arorb"
          />
        </button>
        {/* Hidden by the stylesheet on the collapsed rail, where the orb and the
            count are the whole report and the button's name carries the rest. */}
        <span className="arwords">
          {/* The one line: the tool's named read while one is in flight, the
              agent's own sentence otherwise. The panel keeps the agent's line
              regardless, because it is the agent's report. */}
          <RailSaying line={shown} />
        </span>
        {/* The block's last line, and it exists whether or not there is a figure
            on it: the chevron reports whether the panel is open, which is a fact
            about the block rather than about the spend, so a row that came and
            went with the money would take the disclosure with it. */}
        {/* The same reading as the panel's head, in the width a rail has:
            small, secondary and tabular, with the figure in the primary ink.
            The classes are the type; this file only places the row. */}
        <span className="arlast t-caption t-num">
          {/* The spend sits in the bar and not only in the panel: it is the one
              figure somebody is accountable for, and a number nobody opens a
              panel to see is a number nobody sees. Absent when this seat may not
              read it, and absent again when nothing in the month carried a
              price. */}
          {spend.allowed && spend.minor !== undefined && (
            <span className="arspend">
              <b>{money}</b>
              {/* The scope beside the figure, in the slot the stylesheet
                  reserved for it (`.arspend > .arscope`, "the money, and the
                  scope it was spent in, on one line"): a bare currency amount
                  names nothing. The expanded panel says "Cost this month";
                  this is the same fact in the space a rail has, from the same
                  string. */}
              <span className="arscope">{t("agent.thisMonth")}</span>
            </span>
          )}
          <ChevronRight
            size={15}
            className={open ? "archev open" : "archev"}
            aria-hidden="true"
          />
        </span>
      </div>
    </section>
  );
}
