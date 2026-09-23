// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronRight } from "lucide-react";
import {
  type CSSProperties,
  type MouseEvent,
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { Badge } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import {
  MarginceCoreScene,
  type MarginceCoreState,
} from "../design-system/margince-core";
import { usePrefersReducedMotion } from "../design-system/motion";
import { formatMoney, formatNumber, INTL_LOCALE } from "../format/format";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import { settingsHref } from "../screens/settingsrouting";
import {
  type AgentEdgeRegister,
  clearAgentEdge,
  publishAgentEdge,
} from "./agent-edge-signal";
import { type AgentFault, useAgentFault } from "./agent-fault";
import { RUNNING } from "./agentrail-copy";
import { EdgeLightSetting } from "./agentrail-edgelight";
import { RailLine } from "./agentrail-line";
import type {
  AiActivityItem,
  AiCall,
  AiPosture,
  LicensePosture,
  Signals,
  Spend,
} from "./agentrail-reads";
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
import { type AiActivity, useAiActivity } from "./ai-activity";
import { PANEL_HEADING } from "./ai-activity-lines";
import { laneFor } from "./ai-activity-orb";
import { plain, type SpokenLine, speak, spokenText } from "./ai-activity-speak";
import { useAgentTierMap } from "./autonomy";
import { usePopoverDismiss } from "./popover";
import type { Route } from "./router";
import { routeHash } from "./router";
import { usePhoneViewport } from "./viewport";
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

/** How many of the agent's last actions the panel recaps. */
const RECAP_ROWS = 5;

/**
 * Where the whole trace lives, and where a model gets bound. Same page.
 *
 * The catalog splits that page into four, and this link belongs on
 * `model-calls` — the rail's "full log" is the call list. It moves there in the
 * change that splits the cards; pointing at it now would land a reader on
 * Account, because the screen still renders the combined entry.
 */
const AI_SETTINGS_HREF = routeHash(settingsHref("usage"));
/** Where a licence key is entered: the seats section of settings. */
const LICENSE_SETTINGS_HREF = routeHash(settingsHref("seats"));

/**
 * The state in a word, under the agent's name.
 *
 * The Core's own vocabulary is five machine words, and the head used to print
 * whichever one it was in — so a panel opened on a broken installation said
 * "error" at a reader in the product's own voice. This is that vocabulary in
 * the reader's language, TOTAL over the states the Core knows, so a sixth one
 * cannot arrive unnamed.
 */
const STATE_WORD: Readonly<Record<MarginceCoreState, MessageKey>> = {
  idle: "agent.state.idle",
  ingest: "agent.state.ingest",
  working: "agent.state.working",
  warning: "agent.state.warning",
  error: "agent.state.error",
};

/**
 * What a recap row's mark says, which is how that occurrence WENT.
 *
 * It used to carry the orb's CURRENT tone at a fading opacity, which drew a
 * brief that failed at four in the morning in the green of a quiet afternoon.
 * Position and the stamp at the row's end already say how old a row is, so the
 * mark is free to say the one thing nothing else on the row does.
 */
const ROW_TONE: Readonly<
  Record<AiActivityItem["state"], "info" | "success" | "warning" | "danger">
> = {
  queued: "info",
  running: "info",
  stalled: "warning",
  done: "success",
  degraded: "warning",
  failed: "danger",
};

/**
 * When it happened, as a contact would say it. A wall-clock stamp answers "at
 * what time", and the question a recap answers is "how long ago".
 */
function agoFor(
  iso: string,
  locale: Locale,
  now: number,
  t: Translator,
): string {
  const seconds = Math.round((now - Date.parse(iso)) / 1000);
  if (Number.isNaN(seconds)) {
    return t("agent.line.justNow");
  }
  const format = new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unitDisplay: "narrow",
    maximumFractionDigits: 0,
    unit:
      seconds < 60
        ? "second"
        : seconds < 3600
          ? "minute"
          : seconds < 86_400
            ? "hour"
            : "day",
  });
  const size =
    seconds < 60 ? 1 : seconds < 3600 ? 60 : seconds < 86_400 ? 3600 : 86_400;
  return format.format(Math.max(0, Math.floor(seconds / size)));
}

/**
 * What the model row prints: the model the last call was SERVED by — not the
 * configured one, because a fallback ladder makes those differ exactly when it
 * matters — or the reason there is none.
 */
function modelText(
  read: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>,
  t: Translator,
): string {
  const latest = read.calls[0];
  if (latest) {
    return `${latest.provider}/${latest.served_model}`;
  }
  return t(read.allowed ? "agent.fact.noCalls" : "agent.fact.hidden");
}

/**
 * One titled part of the report: a labelled region with its own heading, so the
 * panel is four named passages rather than one column of text. `action` rides
 * the heading's line — a verb belonging to the section, at the far end of the
 * title it belongs to.
 */
function PanelSection({
  title,
  action,
  className,
  children,
}: Readonly<{
  title: string;
  action?: ReactNode;
  className?: string;
  children: ReactNode;
}>) {
  const titleId = useId();
  return (
    <section
      className={["arsect", className ?? ""].filter(Boolean).join(" ")}
      aria-labelledby={titleId}
    >
      <div className="arsecthead">
        <Heading size="xsmall" as="h3" className="arsectname" id={titleId}>
          {title}
        </Heading>
        {action}
      </div>
      {children}
    </section>
  );
}

/**
 * The recap: what the agent has done lately, and the door to the whole trace.
 *
 * Five rows at most — a sixth turns the panel into a log viewer, which already
 * exists and is better at it.
 *
 * It is drawn from the AI-activity feed rather than from the model-call trace,
 * and the difference is what a row can SAY. The trace is telemetry and carries
 * no record — a call's subject travels to the occurrence and never to
 * `ai_call` — so a recap read from it could only ever say that something
 * happened five times. The occurrence knows what the work was ABOUT, which is
 * how these rows name the account and link to it, in the same vocabulary the
 * running section above speaks.
 */
function Recap({
  settled,
}: Readonly<{
  /** Today's settled occurrences, or undefined while no read has answered. */
  settled: readonly AiActivityItem[] | undefined;
}>) {
  const { locale } = useLocale();
  const t = useT();
  // Read once per open, so five rows share one reading of the clock and cannot
  // disagree about what "now" is.
  const now = Date.now();
  if (settled === undefined) {
    // Nothing, not a sentence: the panel cannot tell a read still in flight
    // from one that failed, and both would be libelled by "nothing has
    // finished today".
    return null;
  }
  const said = settled
    .flatMap((item) => {
      const line = speak(item, t);
      return line === null ? [] : [{ item, line }];
    })
    .slice(0, RECAP_ROWS);
  if (said.length === 0) {
    return <p className="arempty t-caption">{t("agent.panel.nothingToday")}</p>;
  }
  return (
    <>
      {said.map(({ item, line }) => (
        <p className="aritem" key={item.id}>
          {/* How that one went, in the state families: a failed run is the row
              a reader is looking for, and it was the only thing on this list
              that could not be told from the four beside it. */}
          <span
            className="armark"
            aria-hidden="true"
            data-tone={ROW_TONE[item.state]}
          />
          <span className="arsaid">
            <RailLine line={line} />
          </span>
          {/* A settled occurrence has a finish — the projection's own CHECK
              says so — and `started_at` is what is left if a server ever sends
              one that does not. */}
          <span className="armuted t-caption t-num">
            {agoFor(item.finished_at ?? item.started_at, locale, now, t)}
          </span>
        </p>
      ))}
    </>
  );
}

/**
 * What the agent is standing on: the provider, the model it last ran on, how
 * many tools it holds and which sources it cannot reach.
 *
 * A definition list, because every one of them is a term and its value. They
 * were one wrapped line of `<b>`-and-word fragments, which is the shape of a
 * sentence and read as one.
 *
 * The FAULTS lead, above the list and as badges: an unbound model and a refused
 * licence decide whether anything under them means anything, and neither is a
 * fact with a value — it is a repair, with somewhere to go.
 */
function RuntimeFacts({
  offline,
  model,
  ai,
  license,
  licenseLine,
}: Readonly<{
  offline: readonly string[];
  model: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>;
  ai: AiPosture;
  license: LicensePosture | undefined;
  licenseLine: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const tools = Object.values(useAgentTierMap()).length;
  const licenceFault = license === "none" || license === "refused";
  return (
    <div className="armeta">
      {(ai === "unconfigured" || licenceFault) && (
        <div className="arflags">
          {ai === "unconfigured" && (
            <Badge tone="warning">{t("agent.fact.noModel")}</Badge>
          )}
          {/* The badge names a fault the reader can repair only on the seats
              page, so a link around it takes them there: the badge stays the
              label nobody presses, and the anchor carries the press. */}
          {licenceFault && (
            <a className="arwarning" href={LICENSE_SETTINGS_HREF}>
              <Badge tone="warning">{licenseLine}</Badge>
            </a>
          )}
        </div>
      )}
      <dl className="arfacts">
        {ai === "development" && (
          <>
            <dt>{t("auth.coreDevelopment")}</dt>
            <dd>{t("auth.coreModeDevelopment")}</dd>
          </>
        )}
        <dt>{t("agent.fact.model")}</dt>
        {/* Italic where there is no model to name: the words are the reason
            there is none, not a value. */}
        <dd>
          {model.calls.length > 0 ? (
            modelText(model, t)
          ) : (
            <i>{modelText(model, t)}</i>
          )}
        </dd>
        {tools > 0 && (
          <>
            <dt>{t("agent.fact.tools")}</dt>
            <dd>{formatNumber(tools, locale)}</dd>
          </>
        )}
        {offline.length > 0 && (
          <>
            <dt>{t("agent.fact.sources")}</dt>
            <dd>
              {offline.map((source) => (
                <span className="arconn" key={source}>
                  <i aria-hidden="true" />
                  {`${source} ${t("agent.fact.offline")}`}
                </span>
              ))}
            </dd>
          </>
        )}
      </dl>
    </div>
  );
}

/**
 * The runs live now, in the reader's words, under the section's heading.
 *
 * A kind or state the copy map has no line for draws NOTHING — not a fallback
 * sentence, not the message key. A surface that answers an unknown run with an
 * invented sentence is one a reader cannot trust about the runs it DOES name.
 * When that empties the section it is absent, unless the live total holds work
 * beyond the rows: one caption then admits it, offering nothing to read.
 */
function RunSection({
  items,
  liveTotal,
}: Readonly<{ items: readonly AiActivityItem[]; liveTotal: number | null }>) {
  const heading = PANEL_HEADING.running;
  const t = useT();
  // flatMap rather than map+filter: the empty array drops the run AND narrows
  // the line to a string, where a filtered predicate would only have claimed it.
  const said = items.flatMap((item) => {
    const line = speak(item, t);
    return line === null ? [] : [{ item, line }];
  });
  if (said.length === 0) {
    return (liveTotal ?? 0) > items.length ? (
      <PanelSection title={t(heading)}>
        <p className="arempty t-caption">{t("agent.panel.unnamedLive")}</p>
      </PanelSection>
    ) : null;
  }
  return (
    <PanelSection title={t(heading)}>
      <ul className="arruns">
        {said.map(({ item, line }) => (
          <li className="arbox arrun" key={item.id}>
            <span className="arrunline">
              <RailLine line={line} />
            </span>
          </li>
        ))}
      </ul>
    </PanelSection>
  );
}

/**
 * Who is reporting, how it is, and what it has cost.
 *
 * ONE title, and it is the agent's name: the one thing on this surface that
 * does not change every few seconds is whose report it is. The live sentence
 * stands under it as the STATUS rather than as the title — it is the caption of
 * the state (a fault, a run in flight, a resting reading) and not an entry in
 * the log below, so as a heading it would have renamed the region on every poll.
 *
 * The month's figure is meta beside the state and is said HERE and nowhere else
 * on the panel: it is one fact, and a surface that prints it twice invites the
 * reader to check whether the two agree.
 */
function PanelHead({
  state,
  name,
  line,
  spend,
}: Readonly<{
  state: MarginceCoreState;
  name: string;
  line: SpokenLine;
  spend: Spend;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <header className="arphead">
      {/* No orb here. The card in the rail already carries one, and a second
          Core a few pixels away is the same object drawn at another size
          against another ground: the two never quite agree, and a reader who
          sees them disagree stops trusting either. The state's own word and
          tone carry it instead. */}
      <p className="arpstate t-caption">
        <i aria-hidden="true" />
        {t(STATE_WORD[state])}
      </p>
      {spend.allowed && spend.minor !== undefined && (
        <p className="arpmoney t-caption t-num">
          <b>{formatMoney(spend.minor, spend.currency, locale)}</b>{" "}
          {t("agent.thisMonth")}
        </p>
      )}
      <Heading size="medium" as="h2" className="arptitle">
        {name}
      </Heading>
      <p className="arpsaying">
        <RailLine line={line} />
      </p>
    </header>
  );
}

/**
 * The panel: ONE report, in the order a reader asks for it.
 *
 * The head says who is reporting and how it is. Under it four titled sections
 * answer what is running, what needs the reader, what has been done and what it
 * is all standing on — in that order, which is severity and then scope. The one
 * control and the one promise close it, under everything that reports.
 *
 * Every part of it is ABSENT rather than empty when its read has nothing to
 * say: this surface reports what was answered, and a section drawn empty claims
 * an answer nobody got.
 */
function AgentPanel({
  state,
  line,
  running,
  liveTotal,
  settled,
  signals,
  model,
  spend,
  panel,
  frame,
}: Readonly<{
  state: MarginceCoreState;
  /** The same line the card carries, so the two never disagree. */
  line: SpokenLine;
  /** The scheduled runs the server reports as live. */
  running: readonly AiActivityItem[];
  liveTotal: number | null;
  /** What settled today, or undefined while no read of the feed has answered. */
  settled: readonly AiActivityItem[] | undefined;
  signals: Signals;
  model: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>;
  spend: Spend;
  panel: RefObject<HTMLElement | null>;
  frame: PanelFrame;
}>) {
  const { locale } = useLocale();
  const t = useT();
  return (
    <section
      className="arpanel"
      ref={panel}
      aria-label={t("agent.panel.label")}
      style={{
        left: frame.left,
        right: frame.right,
        bottom: frame.bottom,
        width: frame.width,
        maxHeight: frame.maxHeight,
      }}
    >
      <PanelHead state={state} name={signals.name} line={line} spend={spend} />

      {/* Above the counts: a run happening this second outranks a queue that
          has been waiting since yesterday. Only live work is listed here — what
          settled belongs to the recap further down, so an occurrence is in one
          section or the other and never both. The section is absent when its
          list is, rather than drawn empty. */}
      <RunSection items={running} liveTotal={liveTotal} />

      {/* THREE cases, not two, and the difference is the whole doctrine of this
          surface: a count nobody has read is not a count of zero.

          Absent when the read has not answered or this seat may not make it —
          the panel cannot tell those two apart, and both would be misreported
          by either sentence available: "nothing needs you" claims an all-clear
          nobody read, and "not readable on this seat" accuses a seat whose read
          is merely still in flight. Silence is the one honest answer to a
          question that was never answered.

          A read that ANSWERED zero is different, and it earns the sentence:
          the agent looked, and there is nothing waiting. */}
      {signals.waiting !== undefined && (
        <PanelSection title={t("agent.panel.needsYou")}>
          {signals.waiting === 0 ? (
            // A quiet line rather than a dashed plate. The plate said "a tile
            // failed to load" to every reader who met it before they read the
            // words in it, which is the opposite of what an all-clear is for.
            //
            // Its own sentence, not the resting line's: that line says "Nothing
            // needs you" in the head of this very panel, and one surface saying
            // the same four words twice reads as assembled rather than written.
            <p className="arnone t-caption">
              {t("agent.panel.nothingWaiting")}
            </p>
          ) : (
            <div className="artiles">
              {/* The tile leads with the number a reader scans for; its NAME
                  leads with the label, because "10" alone names nothing. */}
              <a
                className="arbox artile"
                href="#/worklist"
                aria-label={`${t("agent.panel.decisions")} ${formatNumber(signals.waiting, locale)}`}
              >
                <b>{formatNumber(signals.waiting, locale)}</b>
                <span>{t("agent.panel.decisions")}</span>
              </a>
            </div>
          )}
        </PanelSection>
      )}

      <PanelSection
        title={t("agent.panel.recent")}
        action={
          // A verb, beside the title rather than inside it: worn as part of the
          // heading it took the heading's weight and read as a second title.
          <a className="link-button" href={AI_SETTINGS_HREF}>
            {t("agent.panel.fullLog")}
          </a>
        }
      >
        <Recap settled={settled} />
      </PanelSection>

      <PanelSection title={t("agent.panel.runtime")} className="arstrip">
        <RuntimeFacts
          offline={signals.offline}
          model={model}
          ai={signals.ai}
          license={signals.license}
          licenseLine={signals.licenseLine}
        />
      </PanelSection>

      {/* The one thing on this panel that CHANGES anything, under everything
          that reports: a control above the report it is about would be read as
          part of the report. */}
      <EdgeLightSetting />

      {/* Last, and the only line here no read produced: the agent reaches no
          further than this reader does, whatever the rows above managed to
          answer. A footnote because it is a standing promise rather than news.
          Held by AC-shell-8. */}
      <p className="arclaim t-caption">{t("shell.agent.scope")}</p>
    </section>
  );
}

/** The air between the rail and the panel it opens. */
const PANEL_GAP = 8;

/**
 * How wide the panel is beside the rail, and the least air it leaves.
 *
 * Wide enough that a count tile holds its label on one line: "Duplicate pairs
 * open" wrapping under its own number is the difference between a figure with a
 * name and two stacked fragments.
 */
const PANEL_WIDTH = 408;
const PANEL_MARGIN = 12;

type PanelFrame = Readonly<{
  left: number;
  right?: number;
  bottom: number;
  width?: number;
  maxHeight: number;
  /**
   * Where the notch under the panel points, in viewport coordinates.
   *
   * Only the phone frame carries one: there the panel stands OVER the anchor
   * rather than beside it, and a full-width sheet with nothing pointing at what
   * opened it is a panel that could have come from anywhere. Measured from the
   * anchor rather than assumed to be the middle of the screen, so the notch
   * still lands on the orb if the bar's cells ever stop being symmetric.
   */
  caret?: number;
}>;

/**
 * The two custom properties the portalled wrapper is placed by.
 *
 * Declared rather than asserted onto `CSSProperties`: React's own type carries
 * the CSS properties it knows, and a cast to it would say these two are among
 * them. The intersection says what is true — the wrapper takes a style object
 * that is one of those PLUS the two this file mints.
 */
type NotchPlacement = CSSProperties &
  Readonly<{
    "--arCaretX"?: string;
    "--arPanelBottom": string;
  }>;

/**
 * The notch's place, handed to the stylesheet rather than drawn here.
 *
 * Where it points is a MEASUREMENT and how it is drawn is the sheet's business,
 * so the frame travels as custom properties on the portalled wrapper. Beside a
 * sidebar there is no notch and no `--arCaretX`: the panel and the card that
 * opened it are already touching, and a tail would have no gap to cross.
 */
function looseStyle(frame: PanelFrame): NotchPlacement {
  return {
    "--arCaretX": frame.caret === undefined ? undefined : `${frame.caret}px`,
    "--arPanelBottom": `${frame.bottom}px`,
  };
}

/**
 * The frame over the phone bar: the bar's own span, clear of the well.
 *
 * Edge to edge with the BAR rather than inset by a margin of its own. The panel
 * hangs off one of the bar's cells, so the two are one object seen from two
 * distances — a panel inset by a different amount reads as a sheet that happened
 * to arrive over the bar. Falls back to its own margin only where no bar was
 * handed in, which is a caller that has none.
 */
function overTheBar(well: DOMRect, bar: DOMRect | undefined): PanelFrame {
  return {
    left: bar ? bar.left : PANEL_MARGIN,
    right: bar ? globalThis.innerWidth - bar.right : PANEL_MARGIN,
    bottom: globalThis.innerHeight - well.top + PANEL_GAP,
    maxHeight: well.top - PANEL_GAP * 2,
    caret: well.left + well.width / 2,
  };
}

/**
 * The frame beside the sidebar: bottom-aligned to the card, so opening the panel
 * does not move the thing that opened it.
 */
function besideTheCard(card: DOMRect): PanelFrame {
  const height = globalThis.innerHeight;
  return {
    left: card.right + PANEL_GAP,
    bottom: Math.max(PANEL_MARGIN, height - card.bottom),
    width: PANEL_WIDTH,
    maxHeight: height - PANEL_MARGIN * 2,
  };
}

/**
 * Where the panel goes, in viewport coordinates.
 *
 * It is FIXED and portalled to the body rather than positioned inside the block
 * it belongs to, and the reason is not preference: the rail scrolls its
 * destinations, so it carries `overflow-x: hidden`, and anything absolutely
 * positioned beside the rail is clipped by it.
 *
 * Beside the rail on a desktop, bottom-aligned to the block, so opening it does
 * not move the thing that opened it. On a phone the rail is the bottom bar and
 * there is no beside: the panel takes the width of the screen and sits above the
 * block instead.
 */
function usePanelFrame(
  card: RefObject<HTMLElement | null>,
  well: RefObject<HTMLElement | null>,
  bar: RefObject<HTMLElement | null> | undefined,
  open: boolean,
  phone: boolean,
): PanelFrame | null {
  const [frame, setFrame] = useState<PanelFrame | null>(null);
  // Before paint, so the panel is never shown at the wrong place for one frame.
  useLayoutEffect(() => {
    if (!open) {
      setFrame(null);
      return;
    }
    const place = () => {
      // Two anchors, because the panel stands in two places. Beside the whole
      // CARD on the sidebar, where the card is what the reader is looking at;
      // above the round WELL on the phone bar, which rises out of the bar's top
      // edge — a frame measured from the cell behind it would open the panel
      // across the orb it belongs to.
      const box = (phone ? well : card).current?.getBoundingClientRect();
      if (!box) {
        return;
      }
      setFrame(
        phone
          ? overTheBar(box, bar?.current?.getBoundingClientRect())
          : besideTheCard(box),
      );
    };
    place();
    // The anchor moves when the rail collapses, when the window changes size and
    // when anything it sits in scrolls, so the frame follows all three rather
    // than being measured once at open.
    globalThis.addEventListener("resize", place);
    globalThis.addEventListener("scroll", place, true);
    return () => {
      globalThis.removeEventListener("resize", place);
      globalThis.removeEventListener("scroll", place, true);
    };
  }, [bar, card, well, open, phone]);
  return frame;
}

/**
 * The state the section shows when nobody has overridden it.
 *
 * ONE SUBJECT: the agent. The rail used to derive `ingest` from a query being in
 * flight and `working` from any mutation, which meant the Core spoke the agent's
 * vocabulary while reporting the reader's own clicks: the orb went to `ingest`
 * because a list was loading, and `ingest` was therefore the one state the agent
 * could never cause. What the TOOL is doing is still reported, in its own
 * quieter line under this one, and it no longer borrows the agent's voice.
 *
 * THREE observers watch the same work, and they rank. The server's projection
 * knows what a run IS and is the only thing that may name it. This tab's own
 * count of requests held open to a model route (`asking`, api/model-inflight.ts)
 * closes the window between a contact pressing "Draft with AI" and the next
 * poll, in which the orb used to sit at rest through the whole of the work it
 * exists to show; it ranks below every occurrence the feed carries, and answers
 * `working` because a request in flight cannot say which half of the lifecycle
 * it is in. A mailbox import is the third, and the one the feed cannot replace:
 * it reaches the feed as a trickle of settled classifications and never as work
 * in flight, so the import's own status row (capture-progress.ts) reports it,
 * as `ingest` by definition — above `asking`, because it knows its half.
 *
 * The order is severity, and it starts with the faults that stop the agent
 * running AT ALL, because an agent with no model bound is not a broken run, it
 * is no runs. Under those, a run that actually broke, then one that stalled.
 * Only then the agent's own live work, and at the bottom, rest.
 *
 * The licence is not in the order. The line answers what the agent is doing
 * right now; a licence posture is a standing condition of the installation,
 * still true in an hour, and letting it take the live line would hide every
 * run behind it. It has its own persistent chrome (`LicenseBanner`).
 *
 * It answers the CAUSE alongside the state, and the two travel together for one
 * reason: the sentence the block carries is the cause's own, so a caller that
 * re-derived the cause by its own reading could caption a colour with a run that
 * did not produce it: an unread failure over a second run still in flight
 * must read as the failure, not the run.
 */
type Reading = Readonly<{
  state: MarginceCoreState;
  /** The occurrence the state is ABOUT, or null when no occurrence caused it. */
  cause: AiActivityItem | null;
  /**
   * Which register the margins light in. The import's only when the import is
   * the ONLY work in flight: a named run or a call this tab holds open is the
   * agent's own work whatever the orb is showing, and the margin says so.
   */
  register: AgentEdgeRegister;
}>;

function derive(
  signals: Signals,
  server: AiActivity,
  fault: AgentFault | null,
): Reading {
  if (signals.ai === "unconfigured" || signals.offline.length > 0) {
    return { state: "error", cause: null, register: "agent" };
  }
  // A run that broke, and that this reader has not been shown yet. It clears by
  // being read rather than by being repaired.
  if (fault !== null) {
    return { state: fault.severity, cause: fault.item, register: "agent" };
  }
  // A live run past the lease its own source declared. The server derives it, so
  // a worker that died without saying so cannot go on being displayed as busy,
  // and amber is right for it: the work may yet land. Where a kind has a way
  // out — opening the account or the document again re-arms its read — the
  // kind's own line says so, because a warning with no next step is only a
  // worry.
  const stalled = server.running.find((item) => item.state === "stalled");
  if (stalled) {
    return { state: "warning", cause: stalled, register: "agent" };
  }
  // The agent's own live work, and which half of the lifecycle it is in comes
  // from the KIND of work rather than from how far along it is: evidence
  // arriving is `ingest`, reasoning over evidence already held is `working`
  // (ai-activity-orb.ts). A queued occurrence counts as live for this, because
  // the agent has taken the work up and the reader has no use for the
  // difference; the line under the orb carries that precision, in copy the feed
  // already has for every state.
  const live = server.running.find((item) => item.state !== "stalled");
  if (live) {
    return { state: laneFor(live), cause: live, register: "agent" };
  }
  // Mail being imported. No cause travels with it — it is not an occurrence the
  // feed can name — so the line is the import's own, from the signals. The
  // margins take the import's register unless this tab has a call open under
  // it: a model call in flight is the agent's own work, and the margin lights
  // for that.
  if (signals.capture !== null) {
    return {
      state: "ingest",
      cause: null,
      register: server.asking ? "agent" : "capture",
    };
  }
  // Work the feed cannot name: this tab's own ask, before the feed catches up,
  // or live work of a kind the rail does not narrate. `working` rather than a
  // lane read from the kind, because the kind is exactly what is not known
  // here. No cause travels with either, so the line falls back to the generic
  // word rather than borrowing a sentence about some other run, until the
  // feed carries an occurrence it can name.
  if (server.asking || (server.liveTotal ?? 0) > server.running.length) {
    return { state: "working", cause: null, register: "agent" };
  }
  // A request that failed a moment ago does NOT colour the orb. One dropped
  // request on a flaky connection would otherwise flash the corner of every
  // screen red and green and red again, and a light that does that is a light
  // nobody reads. What the orb reports is standing state; the screen that made
  // the request reports the request.
  return { state: "idle", cause: null, register: "agent" };
}

/**
 * What that occurrence is called, in the reader's words.
 *
 * A kind the copy map does not narrate answers with the plainest true thing
 * instead of nothing, but ONLY where the state is a fault: an unnamed failure
 * still has to say a failure happened, while an unnamed run in flight can fall
 * through to the state's own generic word without losing anything.
 */
function causeLine(
  cause: AiActivityItem | null,
  t: (key: MessageKey) => string,
): SpokenLine | null {
  if (cause === null) {
    return null;
  }
  const said = speak(cause, t);
  if (said !== null) {
    return said;
  }
  if (cause.state === "failed") {
    return plain(t("agent.line.runFailed"));
  }
  return cause.state === "degraded" || cause.state === "stalled"
    ? plain(t("agent.line.runStopped"))
    : null;
}

/**
 * The one line the block carries, for whichever state is showing.
 *
 * `agentLine` is the sentence belonging to the occurrence that PUT the orb in
 * this state, in the reader's own locale: the run that broke, the one past its
 * lease, or the one running now. It leads wherever it exists, because a state is
 * a colour and a named run is an answer. It is null only for a state no
 * occurrence caused (an unbound model, an unreachable source) or for a kind this
 * build writes no sentence for.
 */
function barLine(
  state: MarginceCoreState,
  signals: Signals,
  devLine: string,
  agentLine: SpokenLine | null,
  said: RailWords,
): SpokenLine {
  if (state === "error" || state === "warning") {
    return faultLine(state, signals, agentLine, said.t);
  }
  if (state === "working") {
    // The named run outranks the generic word: "Working" is true of an overnight
    // brief and of a one-line summary, and only one of them is news.
    return agentLine ?? plain(said.t("agent.state.working"));
  }
  if (state === "ingest") {
    return (
      agentLine ?? plain(signals.capture?.line ?? said.t("agent.state.ingest"))
    );
  }
  if (signals.waiting !== undefined && signals.waiting > 0) {
    return plain(said.waiting(signals.waiting));
  }
  // A deployment on the development path is not disconnected — it answers — but
  // every answer it gives is invented, and a reader who does not know that is
  // being misled by a product that looks like it works. A standing fact, so the
  // resting line states it calmly rather than raising it as a fault, in the same
  // words the sign-in screen already uses for it.
  if (signals.ai === "development") {
    return plain(devLine);
  }
  return plain(said.t("agent.state.idle"));
}

/**
 * The line for a fault, red or amber.
 *
 * Its own function because the two colours rank their causes differently, and
 * the ranking is the whole content: a deployment with no model bound and a
 * source that stopped answering are different repairs, and both outrank a run
 * that broke — an agent that cannot run at all is not a failed run, it is no
 * runs. Amber always has an occurrence behind it, a broken run or a stalled
 * one, and causeLine gives every kind a sentence, so amber's fallback is the
 * same last resort the red branch keeps.
 */
function faultLine(
  state: "error" | "warning",
  signals: Signals,
  agentLine: SpokenLine | null,
  t: Translator,
): SpokenLine {
  if (state === "warning") {
    return agentLine ?? plain(t("agent.line.runStopped"));
  }
  if (signals.ai === "unconfigured") {
    return plain(t("agent.fact.noModel"));
  }
  if (signals.offline.length > 0) {
    return plain(
      t("agent.line.cannotReach", { sources: signals.offline.join(", ") }),
    );
  }
  return agentLine ?? plain(t("agent.line.runFailed"));
}

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
  // second. Read once per render, so every line in one pass of the rotation
  // shares one reading of the clock and they cannot disagree about which runs
  // are still news.
  const settled = stillNews(server.recent, Date.now()).flatMap((item) => {
    const said = speak(item, t);
    return said === null ? [] : [said];
  });
  const resting = useRestingLine(
    restingReadings(
      {
        waiting: signals.waiting,
        developmentLine:
          signals.ai === "development" ? t("auth.coreDevelopment") : null,
        settled,
      },
      said,
    ),
    restingTips(route.screen, t),
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
  // something it can name, that sentence is the orb's line — "Reading Acme" is
  // the status a reader is waiting on at that moment — and the agent's own line
  // has the slot back the instant the read settles. Two stacked lines read as
  // two statuses, and a reader asked which one was the status.
  //
  // A fault keeps the slot regardless: the colour and the caption are always
  // about the same thing, and an amber orb captioned "Reading the pipeline"
  // tells a reader the pipeline is the fault.
  const holdsFault = state === "warning" || state === "error";
  const shown = ticker.length > 0 && !holdsFault ? plain(ticker[0].said) : line;
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
              liveTotal={server.liveTotal}
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
