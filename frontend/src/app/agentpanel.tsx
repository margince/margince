import {
  type ReactNode,
  type RefObject,
  useEffect,
  useId,
  useRef,
} from "react";
import { AppLink } from "../design-system/applink";
import { Heading } from "../design-system/heading";
import {
  MarginceCoreScene,
  type MarginceCoreState,
} from "../design-system/margince-core";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useCan } from "../lib/capability";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import type { PanelFrame } from "./agentpanelframe";
import { STATE_WORD, spendFigure } from "./agentrail-copy";
import { EdgeLightSetting } from "./agentrail-edgelight";
import { RailLine } from "./agentrail-line";
import type { AiActivityItem, AiCall, Signals, Spend } from "./agentrail-reads";
import { Recap, RuntimeFacts } from "./agentrecap";
import { PANEL_HEADING } from "./ai-activity-lines";
import { type SpokenLine, speak } from "./ai-activity-speak";

// The rail's "full log" is the call list, on its own Settings page.
const AI_CALL_LOG_HREF = routeHash(settingsHref("model-calls"));

/**
 * One titled part of the report: a labelled region with its own heading. So the
 * panel is four named passages rather than one column of text. `action` rides
 * the heading's line: a verb of the section, at the far end of its title.
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
 * The runs live now, in the reader's words, under the section's heading.
 *
 * A kind or state the copy map has no line for draws nothing: no fallback
 * sentence, no message key. A surface that answers an unknown run with an
 * invented sentence is one a reader cannot trust about the runs it does name.
 * An emptied section is absent. Work the live total holds beyond the rows gets
 * one caption, unless a listed row is live: a stalled one is outside its lease.
 */
function RunSection({
  items,
  unnamed,
}: Readonly<{ items: readonly AiActivityItem[]; unnamed: boolean }>) {
  const heading = PANEL_HEADING.running;
  const t = useT();
  // flatMap rather than map+filter: the empty array drops the run and narrows
  // the line to a string. A filtered predicate would only have claimed it.
  const said = items.flatMap((item) => {
    const line = speak(item, t);
    return line === null ? [] : [{ item, line }];
  });
  const namesLive = said.some(({ item }) => item.state !== "stalled");
  if (said.length === 0 && !unnamed) return null;
  return (
    <PanelSection title={t(heading)}>
      {said.length > 0 && (
        <ul className="arruns">
          {said.map(({ item, line }) => (
            <li className="arbox arrun" key={item.id}>
              <span className="arrunline">
                <RailLine line={line} />
              </span>
            </li>
          ))}
        </ul>
      )}
      {unnamed && !namesLive && (
        <p className="arempty t-caption">{t("agent.panel.unnamedLive")}</p>
      )}
    </PanelSection>
  );
}

/**
 * Frees the Core's GL context in `host` when its canvas leaves the page. Past
 * the browser's limit the oldest context is lost, and that is the rail's.
 */
function useFreesCoreContext(host: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const canvas = host.current?.querySelector("canvas");
    return () => {
      // A tick later: a StrictMode remount keeps the canvas, and so its context.
      globalThis.setTimeout(() => {
        if (canvas && !canvas.isConnected) {
          canvas
            .getContext("webgl2")
            ?.getExtension("WEBGL_lose_context")
            ?.loseContext();
        }
      }, 0);
    };
  }, [host]);
}

/**
 * Who is reporting, how it is, and what it has cost.
 *
 * One title, and it is the agent's name. Whose report it is stays fixed while
 * the rest of the surface changes every few seconds. The live sentence stands
 * under it as the status, not the title. It captions the state (a fault, a run
 * in flight, a resting reading) and is no entry in the log below. As a heading
 * it would rename the region on every poll.
 *
 * The month's figure is meta beside the state, said here and nowhere else on
 * the panel. It is one fact, and printed twice it invites the reader to check
 * whether the two agree.
 */
function PanelHead({
  state,
  progress,
  name,
  line,
  spend,
  entry,
}: Readonly<{
  state: MarginceCoreState;
  progress?: number;
  name: string;
  line: SpokenLine;
  spend: Spend;
  entry: RefObject<HTMLHeadingElement | null>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const head = useRef<HTMLElement>(null);
  useFreesCoreContext(head);
  return (
    <header className="arphead" ref={head}>
      {/* The panel mounts only while open, so this Core and its GL context
          exist only then. */}
      <MarginceCoreScene
        state={state}
        feed={false}
        progress={progress}
        size="md"
        className="arporb"
      />
      {/* One wrapping row: where the two do not fit side by side, the figure
          drops under the state rather than breaking the state's words. */}
      <div className="arpmeta">
        <p className="arpstate t-caption">
          <i aria-hidden="true" />
          {t(STATE_WORD[state])}
        </p>
        {spend.allowed && spend.minor !== undefined && (
          <p className="arpmoney t-caption t-num">
            <b>{spendFigure(spend.minor, spend.currency, locale)}</b>{" "}
            {t("agent.thisMonth")}
          </p>
        )}
      </div>
      <Heading
        size="medium"
        as="h2"
        className="arptitle"
        tabIndex={-1}
        ref={entry}
      >
        {name}
      </Heading>
      <p className="arpsaying">
        <RailLine line={line} />
      </p>
    </header>
  );
}

/**
 * The panel: one report, in the order a reader asks for it.
 *
 * The head says who is reporting and how it is. Under it, four titled sections
 * answer what is running, what needs the reader, what has been done and what
 * it all stands on. That order is severity, then scope. The one control and the
 * one promise close it, under everything that reports.
 *
 * Every part is absent rather than empty when its read has nothing to say. This
 * surface reports what was answered, and an empty section claims an answer
 * nobody got.
 */
export function AgentPanel({
  state,
  progress,
  line,
  running,
  unnamed,
  settled,
  signals,
  model,
  spend,
  panel,
  focusMovedOn,
  frame,
  id,
}: Readonly<{
  state: MarginceCoreState;
  /** The import's share the rail's orb rings, so the two orbs agree. */
  progress?: number;
  /** The same line the card carries, so the two never disagree. */
  line: SpokenLine;
  /** The scheduled runs the server reports as live. */
  running: readonly AiActivityItem[];
  unnamed: boolean;
  /** What settled today, or undefined while no read of the feed has answered. */
  settled: readonly AiActivityItem[] | undefined;
  signals: Signals;
  model: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>;
  spend: Spend;
  panel: RefObject<HTMLElement | null>;
  /** Set once focus lands outside the panel after the press that opened it. */
  focusMovedOn: RefObject<boolean>;
  frame: PanelFrame;
  /** What the orb's `aria-controls` names. */
  id: string;
}>) {
  const { locale } = useLocale();
  const t = useT();
  // The log opens Settings, which serves the AI call log only to a seat that
  // may read diagnostics. A link a rep cannot follow is a dead end, so it is
  // not offered.
  const mayOpenLog = useCan("ai_diagnostics", "read");
  // Only a press opens the panel. It mounts after its frame is measured and its
  // code has loaded, so focus moves in here rather than at the press. A reader
  // who moved focus elsewhere meanwhile keeps it there. The title, not the first
  // link: that is a record the live line happens to name.
  const entry = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    if (!focusMovedOn.current) {
      entry.current?.focus();
    }
  }, [focusMovedOn]);
  return (
    <section
      className="arpanel"
      ref={panel}
      id={id}
      aria-label={t("agent.panel.label")}
      style={{
        left: frame.left,
        right: frame.right,
        bottom: frame.bottom,
        width: frame.width,
        maxHeight: frame.maxHeight,
      }}
    >
      <PanelHead
        state={state}
        progress={progress}
        name={signals.name}
        line={line}
        spend={spend}
        entry={entry}
      />
      {/* The head stays in view; only what it reports on scrolls. */}
      <div className="arpbody">
        {/* Above the counts: a run happening this second outranks a queue that
          has been waiting since yesterday. Only live work is listed here. What
          settled belongs to the recap further down, so an occurrence is in one
          section or the other, never both. The section is absent when its
          list is, rather than drawn empty. */}
        <RunSection items={running} unnamed={unnamed} />

        {/* Three cases, and the third is the doctrine of this surface: a count
          nobody has read differs from a count of zero.

          Absent when the read has not answered or this seat may not make it.
          The panel cannot tell those two apart, and either sentence would
          misreport one. "nothing needs you" claims an all-clear nobody read.
          "not readable on this seat" accuses a seat whose read is still in
          flight. Silence is the one true answer to an unanswered question.

          A read that answered zero is different, and it earns the sentence:
          the agent looked, and there is nothing waiting. */}
        {signals.waiting !== undefined && (
          <PanelSection title={t("agent.panel.needsYou")}>
            {signals.waiting === 0 ? (
              // A plain line: a dashed plate reads as a tile that failed to load.
              // Its own words: the head may already rest on the all-clear.
              <p className="arnone t-caption">
                {t("agent.panel.nothingWaiting")}
              </p>
            ) : (
              <div className="artiles">
                {/* The tile leads with the number a reader scans for. Its name
                  leads with the label, because "10" alone names nothing. */}
                <AppLink
                  className="arbox artile"
                  href="#/worklist"
                  aria-label={`${t("agent.panel.decisions")} ${formatNumber(signals.waiting, locale)}`}
                >
                  <b>{formatNumber(signals.waiting, locale)}</b>
                  <span>{t("agent.panel.decisions")}</span>
                </AppLink>
              </div>
            )}
          </PanelSection>
        )}

        <PanelSection
          title={t("agent.panel.recent")}
          action={
            // A verb, beside the title rather than inside it. Inside the heading,
            // it took the heading's weight and read as a second title.
            mayOpenLog ? (
              <AppLink className="link-button" href={AI_CALL_LOG_HREF}>
                {t("agent.panel.fullLog")}
              </AppLink>
            ) : undefined
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

        {/* The one thing on this panel that changes anything, under everything
          that reports: a control above the report it is about would be read as
          part of the report. */}
        <EdgeLightSetting />

        {/* Last, and the only line here no read produced: the agent reaches no
          further than this reader does, whatever the rows above managed to
          answer. A footnote because it is a standing promise rather than news.
          Held by AC-shell-8. */}
        <p className="arclaim t-caption">{t("shell.agent.scope")}</p>
      </div>
    </section>
  );
}
