// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { ChevronRight } from "lucide-react";
import {
  type CSSProperties,
  type RefObject,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  MarginceCoreScene,
  type MarginceCoreState,
} from "../design-system/margince-core";
import { formatMoney, formatNumber, INTL_LOCALE } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { usePendingApprovals } from "../screens/approvals.queries";
import { useConnectors } from "../screens/connectors";
import { useLicenseEntitlement } from "../screens/license";
import { clearAgentEdge, publishAgentEdge } from "./agent-edge-signal";
import { type AgentFault, useAgentFault } from "./agent-fault";
import {
  IDLE_ORDER,
  type IdleKind,
  LABELS,
  RUNNING,
  TASK_SAID,
} from "./agentrail-copy";
import { useAgentTicker } from "./agentrail-ticker";
import { type AiActivity, useAiActivity } from "./ai-activity";
import { lineFor, PANEL_HEADING } from "./ai-activity-lines";
import { laneFor } from "./ai-activity-orb";
import { useAgentTierMap } from "./autonomy";
import { useCan } from "./capability";
import { usePopoverDismiss } from "./popover";
import type { Route } from "./router";
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

/** How much dimmer each row's mark is than the one above it. */
const MARK_FADE = 0.16;

/** Where the whole trace lives, and where a model gets bound. Same tab. */
const AI_SETTINGS_HREF = "#/settings/admin/ai";

/** What the installation can actually tell us, and what it cannot. */
type Signals = Readonly<{
  /** Approvals staged for this human; undefined until the read answers.
   *  A true total — usePendingApprovals walks every page. */
  waiting: number | undefined;
  /** Sources the agent cannot reach, named as the reader knows them. */
  offline: readonly string[];
  /** Whether this deployment has a model bound at all. */
  ai: AiPosture;
  /** What the installation is entitled to; undefined when this seat may not
   *  read it, which is not the same as an installation with no licence. */
  license: LicensePosture | undefined;
  /** The licence posture in the reader's words, for the line and the panel. */
  licenseLine: string;
}>;

/**
 * What the installation's entitlement adds up to, for a surface that reports
 * rather than enforces.
 *
 * `none` and `refused` are the two a person has to act on, and they are why the
 * Core carries this at all: an installation with no licence is not a healthy
 * agent with a footnote, it is a standing fault, and the rail used to state it
 * as a grey row at the very bottom that nobody read. `pressing` is the same
 * claim one step softer: over the seat cap, in grace, or renewal due.
 */
type LicensePosture = "ok" | "pressing" | "refused" | "none";

/**
 * What the deployment has bound, as `/assistant/profile` reports it.
 *
 * `configured` says the bindings were CONSTRUCTED at boot — the contract is
 * explicit that it is not a health check, so nothing here may render as online,
 * running or healthy. The negative is the honest half and the one worth showing:
 * a deployment with no provider key has an agent that cannot think, and every
 * other thing this bar reports is beside the point until that is fixed.
 */
type AiPosture = "configured" | "unconfigured" | "development" | "unknown";

/**
 * The reads the bar's right half stands on, and the state they add up to.
 *
 * Order is severity, not convenience: a source the agent cannot reach outranks a
 * queue, because a queue built from half the evidence is the more dangerous of
 * the two to report calmly. Everything else rests at `idle` — proposals
 * WAITING is not a state of its own, it is the agent at rest with a number
 * beside it, and that number is the thing a person acts on.
 */
function useAiPosture(): AiPosture {
  const profile = useQuery({
    queryKey: ["assistant-profile"],
    // Anonymous, cheap and effectively static for the life of the process: the
    // same key the sign-in screen uses, so the two share one answer.
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
    queryFn: async () => {
      const { data, error } = await api.GET("/assistant/profile");
      if (error) {
        return null;
      }
      return data;
    },
  });
  return profile.data?.state ?? "unknown";
}

function useSignals(): Signals {
  const t = useT();
  const approvals = usePendingApprovals();
  const connectors = useConnectors();
  const ai = useAiPosture();
  const license = useLicensePosture();

  const offline = (connectors.data?.data ?? [])
    .filter((connection) => connection.status !== "connected")
    .map((connection) => connection.account_label ?? connection.provider);

  return {
    // Absent `data` means the read has not answered, or was refused. A 0 here
    // would be this surface inventing an all-clear.
    waiting: approvals.data ? approvals.data.data.length : undefined,
    offline,
    ai,
    license,
    licenseLine:
      license === "refused"
        ? t("shell.license.refused")
        : t("shell.license.none"),
  };
}

/**
 * The installation's entitlement, read the way the rail used to read it.
 *
 * Absent for a seat without `license:read`, silently: a read they may not make
 * is not a fact being withheld from them, it is a fact that is none of their
 * work, and an orb that went amber about it on every screen they opened would be
 * a permission boundary drawn as a fault.
 */
function useLicensePosture(): LicensePosture | undefined {
  const mayRead = useCan("license", "read");
  const query = useLicenseEntitlement(mayRead);
  const entitlement = query.data;
  if (!mayRead || !entitlement) {
    return undefined;
  }
  if (entitlement.state === "rejected") {
    return "refused";
  }
  if (entitlement.state !== "valid") {
    return "none";
  }
  return entitlement.over_limit ||
    entitlement.license?.in_grace === true ||
    entitlement.license?.renewal_due === true
    ? "pressing"
    : "ok";
}

/**
 * The model the agent last actually ran on — the SERVED one, not the configured
 * one, because a fallback ladder makes those two differ exactly when it matters.
 *
 * Operator-only: `/ai/calls` sits behind `automation:update`, so a sales seat
 * gets nothing and the runtime row says the seat cannot read it rather than
 * printing a model nobody on that seat could verify. One call, because the panel
 * shows one line.
 */
type AiCall = components["schemas"]["AiCallSummary"];

/**
 * The last few things the agent actually did, newest first.
 *
 * This is the recap the panel owes a reader: not what the agent IS, but what it
 * has been doing — the task, the model it ran on, when. The full trace, with the
 * attempt ladder and the payloads, lives on the AI settings tab, and the panel
 * links to it rather than reproducing it. A recap that grows into a log is a
 * second log to keep correct.
 *
 * Operator-only, because `/ai/calls` sits behind `automation:update`. A seat
 * without it gets no rows and is told why, rather than an empty section that
 * reads as an agent which has never run.
 */
function useRecentCalls(): Readonly<{
  allowed: boolean;
  calls: readonly AiCall[];
}> {
  const allowed = useCan("automation", "update");
  const recent = useQuery({
    queryKey: ["ai-calls", "agentrail-recent"],
    enabled: allowed,
    staleTime: 30_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/calls", {
        params: { query: { limit: RECAP_ROWS } },
      });
      if (error) {
        // Chrome must not take a page down over telemetry: an unreadable log is
        // a state this surface draws, not an error it throws.
        return [];
      }
      return data.data;
    },
  });
  return { allowed, calls: recent.data ?? [] };
}

/**
 * What the agent has cost this month, as the server priced it.
 *
 * `cost_est_minor` is an ESTIMATE the server computes on read from its own rate
 * tables, in minor units of the budget's currency, and it is omitted for a call
 * nothing could be priced against. So the sum is over the lines that HAVE a
 * price, and a month where nothing was priced draws no figure at all rather than
 * a confident zero: the difference between "this cost nothing" and "nobody knows
 * what this cost" is the whole point of the row.
 *
 * Operator-only, on the same grant the spend card uses: the server treats the
 * runtime's cost as operator information, so a sales seat gets nothing and the
 * panel says so rather than printing a number that seat could not verify.
 */
function useAiSpend(): Readonly<{
  allowed: boolean;
  minor: number | undefined;
  currency: string;
  /** One number per day of the month so far, oldest first. */
  daily: readonly number[];
}> {
  const allowed = useCan("automation", "update");
  const usage = useQuery({
    queryKey: ["ai-usage", "agentrail-month"],
    enabled: allowed,
    // The month's spend does not move between two page opens, and this read sits
    // in the chrome, so a short staleness would put a request behind every
    // navigation.
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/usage", {
        params: { query: {} },
      });
      if (error) {
        // Chrome must not take a page down over a reading: an unreadable figure
        // is a state this surface draws, not an error it throws.
        return null;
      }
      return data;
    },
  });
  const days = usage.data?.days ?? [];
  const priced = days
    .flatMap((day) => day.tasks)
    .filter((task) => task.cost_est_minor !== undefined);
  return {
    allowed,
    minor:
      priced.length === 0
        ? undefined
        : priced.reduce((total, task) => total + (task.cost_est_minor ?? 0), 0),
    currency: usage.data?.budget.currency ?? "USD",
    // A day with no priced call is a real zero here, unlike the total: the month
    // HAS that day, and a gap in a series is a lie about its shape.
    daily: days.map((day) =>
      day.tasks.reduce((total, task) => total + (task.cost_est_minor ?? 0), 0),
    ),
  };
}

/**
 * What the runtime row prints, in the three cases it genuinely has: the model
 * the last call was SERVED by — not the configured one, because a fallback
 * ladder makes those differ exactly when it matters — or the reason there is
 * none.
 */
function modelText(
  read: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>,
): string {
  const latest = read.calls[0];
  if (latest) {
    return `${latest.provider}/${latest.served_model}`;
  }
  return read.allowed ? LABELS.noCallsYet : LABELS.unreadable;
}

/**
 * The recap: what the agent has done lately, and the door to the whole trace.
 *
 * Five rows at most. The question a person asks of a background agent is "what
 * have you been doing", and five answers it — a sixth turns the panel into a log
 * viewer, which already exists and is better at it.
 */
/** The task in the reader's words, or the token opened up if it is a new one. */
function saidFor(task: string): string {
  // `task` comes off the wire, and a bare lookup answers `constructor` from the
  // prototype chain with a function that React then tries to render.
  return Object.hasOwn(TASK_SAID, task)
    ? TASK_SAID[task]
    : task.replaceAll("_", " ");
}

/**
 * When it happened, as a person would say it.
 *
 * A wall-clock stamp answers "at what time", and the question a recap answers is
 * "how long ago" — five rows of `19/08/2026, 10:00` make the reader do the
 * subtraction, five times, to learn that everything happened this morning.
 */
function agoFor(iso: string, locale: Locale, now: number): string {
  const seconds = Math.round((now - Date.parse(iso)) / 1000);
  if (Number.isNaN(seconds)) {
    return LABELS.justNow;
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

function Recap({
  recent,
}: Readonly<{
  recent: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>;
}>) {
  const { locale } = useLocale();
  // Read once per open, so five rows share one reading of the clock and cannot
  // disagree about what "now" is.
  const now = Date.now();
  if (!recent.allowed) {
    return <p className="aritem arempty">{LABELS.logUnreadable}</p>;
  }
  if (recent.calls.length === 0) {
    return <p className="aritem arempty">{LABELS.noCallsYet}</p>;
  }
  return (
    <>
      {recent.calls.map((call, index) => (
        <p className="aritem" key={call.id}>
          {/* The mark fades down the list, so the newest thing the agent did is
              the brightest thing in it. Position IS the age here: the rows are
              newest first, and a reader takes the gradient before they read a
              single timestamp. */}
          <span
            className="armark"
            aria-hidden="true"
            style={{ opacity: Math.max(0.3, 1 - index * MARK_FADE) }}
          />
          {saidFor(call.task)}
          <span className="armuted">
            {agoFor(call.occurred_at, locale, now)}
          </span>
        </p>
      ))}
    </>
  );
}

function RuntimeRows({
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
  const spend = useAiSpend();
  const tools = Object.values(useAgentTierMap()).length;
  return (
    <div className="armeta">
      {/* The posture leads, because it decides whether anything below it means
          anything: a model name from last week is not a model bound today. */}
      {ai === "unconfigured" && (
        <span className="arwarn">{LABELS.noModel}</span>
      )}
      {ai === "development" && (
        <span>
          <b>{t("auth.coreDevelopment")}</b> {t("auth.coreModeDevelopment")}
        </span>
      )}
      <span>
        {LABELS.model}{" "}
        {model.calls.length > 0 ? (
          <b>{modelText(model)}</b>
        ) : (
          <i>{modelText(model)}</i>
        )}
      </span>
      {/* Absent when the seat may not read it, and absent again when nothing in
          the month carried a price. A spend row is the one figure on this panel
          somebody will quote at somebody else, so it is drawn only when the
          server actually priced the calls behind it. */}
      {spend.allowed && spend.minor !== undefined && (
        <span>
          {LABELS.spend}{" "}
          <b>{formatMoney(spend.minor, spend.currency, locale)}</b>
        </span>
      )}
      {tools > 0 && (
        <span>
          {LABELS.tools} <b>{formatNumber(tools, locale)}</b>
        </span>
      )}
      {(license === "none" || license === "refused") && (
        <span className="arwarn">{licenseLine}</span>
      )}
      {offline.map((source) => (
        <span className="arconn down" key={source}>
          <i aria-hidden="true" />
          {`${source} ${LABELS.offline}`}
        </span>
      ))}
    </div>
  );
}

/** One AI occurrence, as the server reports it. */
type AiActivityItem = components["schemas"]["AiActivityItem"];

/**
 * One list of scheduled runs, in the reader's words, under its own heading.
 *
 * A kind or state the copy map has no line for draws NOTHING — not a fallback
 * sentence, not the message key. `lineFor` returning null is the map saying it
 * has never heard of this run, and a surface that answers that with an invented
 * sentence is a surface a reader cannot trust about the runs it DOES name. When
 * that empties the section, the section is absent too.
 */
function RunSection({
  heading,
  items,
}: Readonly<{ heading: MessageKey; items: readonly AiActivityItem[] }>) {
  const t = useT();
  // flatMap rather than map+filter: the empty array drops the run AND narrows
  // the line to a string, where a filtered predicate would only have claimed it.
  const said = items.flatMap((item) => {
    const line = lineFor(item, t);
    return line === null ? [] : [{ item, line }];
  });
  if (said.length === 0) {
    return null;
  }
  return (
    <div className="arsect">
      <h4>{t(heading)}</h4>
      <ul className="arruns">
        {said.map(({ item, line }) => (
          <li className="arbox arrun" key={item.id}>
            <span className="arrunline">{line}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function AgentPanel({
  state,
  line,
  running,
  signals,
  model,
  spend,
  panel,
  frame,
}: Readonly<{
  state: MarginceCoreState;
  /** The same line the card carries, so the two never disagree. */
  line: string;
  /** The scheduled runs the server reports as live. */
  running: readonly AiActivityItem[];
  signals: Signals;
  model: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>;
  spend: Readonly<{
    allowed: boolean;
    minor: number | undefined;
    currency: string;
  }>;
  panel: RefObject<HTMLElement | null>;
  frame: PanelFrame;
}>) {
  const { locale } = useLocale();
  return (
    <section
      className="arpanel"
      ref={panel}
      aria-label={LABELS.panel}
      style={{
        left: frame.left,
        right: frame.right,
        bottom: frame.bottom,
        width: frame.width,
        maxHeight: frame.maxHeight,
      }}
    >
      {/* The head restates what the card said, because the panel opens over the
          page and away from it: a reader who came here for the detail should not
          have to look back at the rail to remember what the detail is about. */}
      {/* No orb here. The card in the rail already carries one, and a second
          Core a few pixels away is the same object drawn at another size against
          another ground: the two never quite agree, and a reader who sees them
          disagree stops trusting either. The state's own word and tone carry it
          instead. */}
      <header className="arphead">
        <span className="arpstate">
          <i aria-hidden="true" />
          {state}
        </span>
        <p className="arptitle">{line}</p>
        {spend.allowed && spend.minor !== undefined && (
          <span className="arpmoney">
            <b>{formatMoney(spend.minor, spend.currency, locale)}</b>
            <i>{LABELS.thisMonth}</i>
          </span>
        )}
      </header>

      {/* Above the counts: a run happening this second outranks a queue that
          has been waiting since yesterday. Only live work is listed — a settled
          run reaches the reader through the resting rotation on the card, not
          as a list here. The section is absent when its list is, rather than
          drawn empty. */}
      <RunSection heading={PANEL_HEADING.running} items={running} />

      {/* The one count somebody opens this panel to act on, as a tile rather
          than a row: a number in a list of rows reads as one more line of text.
          The duplicate queue used to stand beside it and does not any more. It
          is not the agent's work — it is a queue the product keeps, repaired on
          the screen that owns it, and every place it appeared here was a second
          telling of a number the worklist already carries. */}
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
        <div className="arsect">
          <h4>{LABELS.acrossWorkspace}</h4>
          {signals.waiting === 0 ? (
            <p className="arnone">{LABELS.allClear}</p>
          ) : (
            <div className="artiles">
              {/* The tile leads with the number because that is what a reader
                  scans for, and its NAME leads with the label because "10"
                  spoken alone is not a sentence about anything. */}
              <a
                className="arbox artile"
                href="#/worklist"
                aria-label={`${LABELS.approvals} ${formatNumber(signals.waiting, locale)}`}
              >
                <b>{formatNumber(signals.waiting, locale)}</b>
                <span>{LABELS.approvals}</span>
              </a>
            </div>
          )}
        </div>
      )}

      <div className="arsect">
        <h4>
          {LABELS.recap}
          <a className="arplain" href={AI_SETTINGS_HREF}>
            {LABELS.fullLog}
          </a>
        </h4>
        <Recap recent={model} />
      </div>

      {/* What it is standing on, in one strip. Not a section of its own: it is
          the small print of the panel, and small print that takes a heading and
          four rows reads as more important than the counts above it. */}
      <div className="arstrip">
        <RuntimeRows
          offline={signals.offline}
          model={model}
          ai={signals.ai}
          license={signals.license}
          licenseLine={signals.licenseLine}
        />
      </div>
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
 * The subject stayed the agent when a second observer of it was added. Two
 * things watch the same work from different ends: the server's projection, which
 * knows what a run IS and is the only thing that may name it, and this tab's own
 * count of requests it is holding open to a route whose handler calls a model
 * and waits (`asking`, from api/model-inflight.ts). The projection arrives on a
 * poll, so between a person pressing "Draft with AI" and the next read there was
 * a live model call nothing on screen reported: the orb sat at rest through the
 * whole of the work it exists to show. `asking` closes that window and claims
 * nothing else: it ranks BELOW every occurrence the feed carries, so it can
 * never outrank a run, and the moment the feed can name the work it does.
 *
 * The correction that ranking allows is real and is the design working, not a
 * defect to hide: `asking` answers `working` because a request in flight cannot
 * say which half of the lifecycle it is in, so a route whose task turns out to
 * be `ingest` (the enrich and cold-start lanes) shows `working` for the
 * moment before the feed arrives and settles into its own lane after. A guess
 * the owner of the fact overrules is the right shape for a bridge.
 *
 * The order is severity, and it starts with the faults that stop the agent
 * running AT ALL, because an agent with no model bound is not a broken run, it
 * is no runs. Under those, a run that actually broke. Under that, the licence.
 * Only then the agent's own live work, and at the bottom, rest.
 *
 * It answers the CAUSE alongside the state, and the two travel together for one
 * reason: the sentence the block carries is the cause's own, so a caller that
 * re-derived the cause by its own reading could caption a colour with a run that
 * did not produce it. A licensing amber over an installation whose agent is
 * mid-brief is exactly that case, and it is not hypothetical: a workspace in
 * grace keeps running its agent.
 */
type Reading = Readonly<{
  state: MarginceCoreState;
  /** The occurrence the state is ABOUT, or null when no occurrence caused it. */
  cause: AiActivityItem | null;
}>;

function derive(
  signals: Signals,
  server: AiActivity,
  fault: AgentFault | null,
): Reading {
  if (signals.ai === "unconfigured" || signals.offline.length > 0) {
    return { state: "error", cause: null };
  }
  // A run that broke, and that this reader has not been shown yet. It outranks
  // the licence because it is a thing that HAPPENED rather than a standing
  // condition, and it clears by being read rather than by being repaired.
  if (fault !== null) {
    return { state: fault.severity, cause: fault.item };
  }
  // A live run past the lease its own source declared. The server derives it, so
  // a worker that died without saying so cannot go on being displayed as busy,
  // and amber is right for it: the work may yet land, and there is nothing for
  // the reader to do but know.
  const stalled = server.running.find((item) => item.state === "stalled");
  if (stalled) {
    return { state: "warning", cause: stalled };
  }
  // REFUSED only, and it stays amber rather than escalating, because escalating
  // would make the chrome a sales surface.
  //
  // An installation that never had a licence is deliberately not a fault here.
  // It used to be, and the result was that every demo and every fresh dev stack
  // wore a permanent amber orb: the state stopped reading as "a fault that can
  // wait" and started reading as "this is a default install", which is the way a
  // signal that is always on stops being a signal at all. Asking and being told
  // no is different, because there is a repair behind it.
  //
  // Nothing in the chrome says the licence is missing, and that is the trade
  // rather than an oversight: the fact is a standing condition, so it is stated
  // once where an operator goes looking for it — the licence card in settings
  // names both absences and says what each one costs — instead of spending the
  // chrome's only ambient warning channel on something that is permanently true.
  if (signals.license === "refused") {
    return { state: "warning", cause: null };
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
    return { state: laneFor(live), cause: live };
  }
  // This tab's own ask, which the feed has not caught up with yet. `working`
  // rather than a lane read from the kind, because the kind is exactly what is
  // not known here: a request in flight says the agent is busy and says nothing
  // about which half of its lifecycle it is in. `working` is the same answer
  // laneFor gives an unnamed kind, and for the same reason: the honest half of
  // what is known. No cause travels with it, so the line under the orb falls
  // back to the generic word instead of borrowing a sentence about some other
  // run, and the moment the feed carries the occurrence the branch above wins
  // and names it.
  if (server.asking) {
    return { state: "working", cause: null };
  }
  // A request that failed a moment ago does NOT colour the orb. One dropped
  // request on a flaky connection would otherwise flash the corner of every
  // screen red and green and red again, and a light that does that is a light
  // nobody reads. What the orb reports is standing state; the screen that made
  // the request reports the request.
  return { state: "idle", cause: null };
}

/**
 * The true things a resting agent can say, in the order it says them.
 *
 * Every one is a reading it already made. A kind with nothing to report is
 * absent rather than reworded into a cheerful nothing, so an installation with a
 * clean queue and no licence rotates through two lines and not five.
 */
function idleLines(
  signals: Signals,
  devLine: string,
  /** The newest run that settled today, or null when there is none to name. */
  settledLine: string | null,
): readonly string[] {
  const said: Partial<Record<IdleKind, string>> = {
    // What the scheduled runner finished while nobody was looking. It rotates
    // rather than pinning the bar: `recent` is bounded to today, so a line
    // pinned to it would still be announcing this morning's brief at six in the
    // evening. In the rotation it is one true thing among the others.
    finished: settledLine ?? undefined,
    waiting:
      signals.waiting !== undefined && signals.waiting > 0
        ? `${signals.waiting} ${LABELS.waiting}`
        : undefined,
    // The development path answers every call with an invention, and a reader who
    // does not know that is being misled by a product that looks like it works.
    model: signals.ai === "development" ? devLine : undefined,
  };
  const lines = IDLE_ORDER.map((kind) => said[kind]).filter(
    (line): line is string => line !== undefined,
  );
  return lines.length === 0 ? [LABELS.allClear] : lines;
}

/**
 * Which of those lines is showing, changing on its own.
 *
 * Slow: this sits at the edge of every screen all day, and a line that changed
 * every second would be movement in the corner of somebody's eye while they were
 * trying to read something else. One line, long enough to read twice.
 */
const IDLE_HOLD_MS = 5200;

function useIdleLine(lines: readonly string[]): string {
  const [at, setAt] = useState(0);
  const count = lines.length;
  useEffect(() => {
    if (count < 2) {
      return;
    }
    const timer = setInterval(
      () => setAt((current) => (current + 1) % count),
      IDLE_HOLD_MS,
    );
    return () => clearInterval(timer);
  }, [count]);
  // The list can shorten under it when a read answers, so the index is clamped
  // rather than trusted.
  return lines[at % count] ?? lines[0];
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
): string | null {
  if (cause === null) {
    return null;
  }
  const said = lineFor(cause, t);
  if (said !== null) {
    return said;
  }
  if (cause.state === "failed") {
    return LABELS.runFailed;
  }
  return cause.state === "degraded" || cause.state === "stalled"
    ? LABELS.runStopped
    : null;
}

/**
 * The one line the block carries, for whichever state is showing.
 *
 * `agentLine` is the sentence belonging to the occurrence that PUT the orb in
 * this state, in the reader's own locale: the run that broke, the one past its
 * lease, or the one running now. It leads wherever it exists, because a state is
 * a colour and a named run is an answer. It is null only for a state no
 * occurrence caused (a licence, an unbound model) or for a kind this build
 * writes no sentence for.
 */
function barLine(
  state: MarginceCoreState,
  signals: Signals,
  devLine: string,
  agentLine: string | null,
): string {
  if (state === "error") {
    // A deployment with no model bound and a source that stopped answering are
    // different repairs, and both outrank a run that broke: an agent that cannot
    // run at all is not a failed run, it is no runs.
    if (signals.ai === "unconfigured") {
      return LABELS.noModel;
    }
    if (signals.offline.length > 0) {
      return `${LABELS.cannotReach} ${signals.offline.join(", ")}`;
    }
    return agentLine ?? LABELS.runFailed;
  }
  if (state === "warning") {
    // Amber with no occurrence behind it is the licence, and it is the only way
    // to reach that: derive() ranks a broken run and a stalled one above it, and
    // both carry a sentence of their own.
    return agentLine ?? signals.licenseLine;
  }
  if (state === "working") {
    // The named run outranks the generic word: "Working" is true of an overnight
    // brief and of a one-line summary, and only one of them is news.
    return agentLine ?? LABELS.working;
  }
  if (state === "ingest") {
    return agentLine ?? LABELS.reading;
  }
  if (signals.waiting !== undefined && signals.waiting > 0) {
    return `${signals.waiting} ${LABELS.waiting}`;
  }
  // A deployment on the development path is not disconnected — it answers — but
  // every answer it gives is invented, and a reader who does not know that is
  // being misled by a product that looks like it works. A standing fact, so the
  // resting line states it calmly rather than raising it as a fault, in the same
  // words the sign-in screen already uses for it.
  if (signals.ai === "development") {
    return devLine;
  }
  return LABELS.idle;
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
): string {
  const name = open ? LABELS.collapse : LABELS.expand;
  return spend.allowed && spend.minor !== undefined
    ? `${name}. ${LABELS.spend}: ${money}`
    : name;
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
  const panel = useRef<HTMLElement>(null);
  const block = useRef<HTMLElement>(null);
  const phone = usePhoneViewport();
  const signals = useSignals();
  const model = useRecentCalls();
  const server = useAiActivity();
  const ticker = useAgentTicker();
  const spend = useAiSpend();
  const { locale } = useLocale();
  const { fault, acknowledge } = useAgentFault(server.recent);
  const { state, cause } = derive(signals, server, fault);

  // What the screen's margins draw, published rather than re-derived: the reads
  // above are all local to this component, so a second consumer calling the same
  // hooks would get a second set of them and report on those.
  // The unanswered queue is NOT part of it: it reaches a reader through this
  // panel's own line, with its count, rather than as a ring around the window
  // that stands for as long as the queue does.
  const reading = RUNNING.has(state);
  useEffect(() => {
    publishAgentEdge({ reading });
  }, [reading]);
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

  const frame = usePanelFrame(block, trigger, bar, open, phone);
  const money =
    spend.minor === undefined
      ? ""
      : formatMoney(spend.minor, spend.currency, locale);
  const hitLabel = railHitLabel(open, spend, money);
  // Above the early return with every other hook: a screen this section draws
  // nothing on is still a render it has to make the same calls in.
  // The newest settled run, for the rotation; the newest live one, for the bar.
  // The bar keeps the live run because that is what is true this second.
  const settledLine = server.recent[0] ? lineFor(server.recent[0], t) : null;
  const resting = useIdleLine(
    idleLines(signals, t("auth.coreDevelopment"), settledLine),
  );

  // The one screen it absents itself from, and the reason is not layout: the Ask
  // surface IS the agent, at hero size, and a second Core in the rail would be
  // the product disagreeing with itself about how many agents there are. The
  // railless screens need no rule of their own any more, because a section in
  // the rail is absent wherever the rail is. Below every hook, because a screen
  // this component draws nothing on is still a render it has to make the same
  // calls in.
  if (route.screen === "ai") {
    return null;
  }

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
      : barLine(state, signals, t("auth.coreDevelopment"), agentLine);
  return (
    <section
      className="arblock"
      data-core-state={state}
      aria-label={LABELS.region}
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
              spend={spend}
            />
          </div>,
          document.body,
        )}
      {/* One button carries the whole block: the click, the accessible name and
          the expanded state. A wrapper with a click handler is a target no
          keyboard can reach, and the CTA underneath keeps its own. */}
      <button
        type="button"
        className="arhit"
        ref={trigger}
        aria-expanded={open}
        // The spend joins the NAME, not only the box.
        //
        // An `aria-label` replaces everything inside the button, so the figure
        // and its scope are drawn for a sighted reader and reach a screen
        // reader from nowhere at all — and on the collapsed rail the
        // stylesheet hides `.arwords` too, so there is no second route to it.
        // A figure somebody is accountable for cannot be the half of this
        // control that only some readers get.
        aria-label={hitLabel}
        // Opening the panel is what acknowledges a broken run: it is the
        // reader turning to the agent's report, so it is the moment the fault
        // stops needing to be held. Until then the orb holds it, however many
        // hours it takes them to look.
        onClick={() => {
          if (!open) {
            acknowledge();
          }
          setOpen((current) => !current);
        }}
      >
        <MarginceCoreScene
          state={state}
          feed={false}
          size="md"
          className="arorb"
        />
        {/* Hidden by the stylesheet on the collapsed rail, where the orb and the
            count are the whole report and the button's name carries the rest. */}
        <span className="arwords">
          {/* The agent's line, and it is the agent's alone. It used to be shared
              with the tool's own narration below, which meant the one sentence
              about the agent vanished for as long as anything was loading: the
              two subjects took turns in one slot, and a reader could not tell
              which of them was talking. */}
          <span className="arline">{line}</span>
          {/* What the TOOL is fetching for this reader, one named thing at a
              time, in its own quieter register underneath. It is true at the
              same moment as the line above and about something else, so it sits
              beside it rather than replacing it. Keyed on the event id so a
              repeated phrase still arrives as a new line rather than sitting
              there looking stuck: what a reader is counting is EVENTS, and two
              identical sentences in a row are two of them. */}
          {ticker.length > 0 && (
            <span className="artool" key={ticker[0].id}>
              {ticker[0].said}
            </span>
          )}
          {/* The spend sits in the bar and not only in the panel: it is the one
              figure somebody is accountable for, and a number nobody opens a
              panel to see is a number nobody sees. Absent when this seat may not
              read it, and absent again when nothing in the month carried a
              price. */}
          {spend.allowed && spend.minor !== undefined && (
            <span className="arspend">
              {money}
              {/* The scope beside the figure, in the slot the stylesheet
                  already reserved for it (`.arspend > .arscope`, "the money,
                  and the scope it was spent in, on one line"). It shipped
                  without one, so the rail carried a bare currency amount naming
                  nothing — and the button's own aria-label overrides the text
                  inside it, so no reader got the word from anywhere. The
                  expanded panel says "Cost this month"; this is the same fact
                  in the space a rail has, from the same string. */}
              <span className="arscope">{LABELS.spendScope}</span>
            </span>
          )}
        </span>
        <ChevronRight
          size={15}
          className={open ? "archev open" : "archev"}
          aria-hidden="true"
        />
      </button>
    </section>
  );
}
