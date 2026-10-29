import { type ReactNode, type RefObject, useId } from "react";
import { Heading } from "../design-system/heading";
import type { MarginceCoreState } from "../design-system/margince-core";
import { formatMoney, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCan } from "../lib/capability";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import type { PanelFrame } from "./agentpanelframe";
import { EdgeLightSetting } from "./agentrail-edgelight";
import { RailLine } from "./agentrail-line";
import type { AiActivityItem, AiCall, Signals, Spend } from "./agentrail-reads";
import { Recap, RuntimeFacts } from "./agentrecap";
import { PANEL_HEADING } from "./ai-activity-lines";
import { type SpokenLine, speak } from "./ai-activity-speak";

/**
 * Where the whole trace lives, and where a model gets bound. Same page.
 *
 * The catalog splits that page into four, and this link belongs on
 * `model-calls`, because the rail's "full log" is the call list. It moves there
 * in the change that splits the cards. Pointing at it now would land a reader
 * on Account, because the screen still renders the combined entry.
 */
const AI_SETTINGS_HREF = routeHash(settingsHref("usage"));

/**
 * The state in a word, under the agent's name.
 *
 * The Core's own vocabulary is five machine words. Printed raw, a panel opened
 * on a broken installation would say "error" at a reader in the product's own
 * voice. This is that vocabulary in the reader's language. It is total over the
 * states the Core knows, so a sixth one cannot arrive unnamed.
 */
const STATE_WORD: Readonly<Record<MarginceCoreState, MessageKey>> = {
  idle: "agent.state.idle",
  ingest: "agent.state.ingest",
  working: "agent.state.working",
  warning: "agent.state.warning",
  error: "agent.state.error",
};

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
  line,
  running,
  unnamed,
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
  unnamed: boolean;
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
  // The log opens Settings, which serves the AI usage page only to a seat that
  // may read diagnostics. A link a rep cannot follow is a dead end, so it is
  // not offered.
  const mayOpenLog = useCan("ai_diagnostics", "read");
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
            // A quiet line rather than a dashed plate. Before anyone read its
            // words, the plate said "a tile failed to load". That is the
            // opposite of what an all-clear is for.
            //
            // Its own sentence, not the resting line's. That line says "Nothing
            // needs you" in the head of this same panel. One surface saying the
            // same four words twice reads as assembled rather than written.
            <p className="arnone t-caption">
              {t("agent.panel.nothingWaiting")}
            </p>
          ) : (
            <div className="artiles">
              {/* The tile leads with the number a reader scans for. Its name
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
          // A verb, beside the title rather than inside it. Inside the heading,
          // it took the heading's weight and read as a second title.
          mayOpenLog ? (
            <a className="link-button" href={AI_SETTINGS_HREF}>
              {t("agent.panel.fullLog")}
            </a>
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
    </section>
  );
}
