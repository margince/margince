import { type ReactNode, type RefObject, useId, useState } from "react";
import { AppLink } from "../design-system/applink";
import { SegmentedControl } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import type { MarginceCoreState } from "../design-system/margince-core";
import type { WorklistItem } from "../features/worklist/worklist.queries";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCan } from "../lib/capability";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import { Recommended } from "./agentadvice";
import type { AgentJob } from "./agentjobs";
import { Readings, TaskDial } from "./agentpanel-readings";
import { ReadyWork } from "./agentpanel-work";
import type { PanelFrame } from "./agentpanelframe";
import { modelText } from "./agentrail.model";
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
 * Who is reporting, how it is, and what it runs on.
 *
 * One title, and it is the agent's name: whose report it is stays fixed while
 * the rest changes every few seconds. The live sentence stands under it as the
 * status. The model is named only to a seat that may read the call trace.
 */
function PanelHead({
  state,
  name,
  line,
  model,
  tasks,
  children,
}: Readonly<{
  state: MarginceCoreState;
  name: string;
  line: SpokenLine;
  model: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>;
  /** The reader's open tasks; undefined until the day is read. */
  tasks: number | undefined;
  /** The figures, on the head's last line. */
  children: ReactNode;
}>) {
  const t = useT();
  return (
    <header className="arphead">
      {tasks !== undefined && <TaskDial count={tasks} />}
      <Heading size="medium" as="h2" className="arptitle">
        {name}
      </Heading>
      {/* No orb here: the rail already carries one, and two a few pixels
          apart never quite agree. The state's word and tone carry it. */}
      <p className="arpstate t-caption">
        <i aria-hidden="true" />
        {t(STATE_WORD[state])}
      </p>
      <p className="arpsaying">
        <RailLine line={line} />
      </p>
      {children}
      {model.allowed && (
        <p className="arpmodel t-caption">
          {t("agent.fact.model")} <b>{modelText(model, t)}</b>
        </p>
      )}
    </header>
  );
}

const TABS = ["now", "activity", "system"] as const;
type Tab = (typeof TABS)[number];

/**
 * The panel: the agent's control centre.
 *
 * The head carries the task dial and the figures, over three bodies. Now holds what is running,
 * the reader's open tasks and what the agent recommends. Activity holds what
 * settled. System holds what it stands on. The promise closes all three.
 *
 * Every part is absent rather than empty when its read has nothing to say. An
 * empty section claims an answer nobody got.
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
  ready,
  here,
  jobs,
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
  /** The reader's day, the open page's rows first; undefined until it is read. */
  ready?: readonly WorklistItem[];
  /** The rows of it about the open page. */
  here?: readonly WorklistItem[];
  /** What the reader can hand the agent from this page. */
  jobs?: readonly AgentJob[];
}>) {
  const t = useT();
  // The log opens Settings, which serves the AI usage page only to a seat that
  // may read diagnostics. A link a rep cannot follow is not offered.
  const mayOpenLog = useCan("ai_diagnostics", "read");
  const [tab, setTab] = useState<Tab>("now");
  const hasWork = ready !== undefined && ready.length > 0;
  // Counted as the recap lists it: a run the copy cannot name is not a row.
  const advises =
    (jobs?.length ?? 0) > 0 ||
    signals.offline.length > 0 ||
    (signals.waiting ?? 0) > 0;
  const done = settled?.filter((item) => speak(item, t) !== null).length;
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
      <PanelHead
        state={state}
        name={signals.name}
        line={line}
        model={model}
        tasks={ready?.length}
      >
        <Readings
          waiting={signals.waiting}
          done={done}
          spend={spend}
          logHref={mayOpenLog ? AI_SETTINGS_HREF : undefined}
        />
      </PanelHead>

      {/* The three bodies stay mounted: a job started under Now keeps its
          answer while the reader looks at Activity. */}
      <div className="artab" hidden={tab !== "now"}>
        {/* Live work leads: a run happening this second outranks a queue. */}
        <RunSection items={running} unnamed={unnamed} />
        {/* A day nobody has read is no section; a day read empty says so. */}
        {ready !== undefined && (
          <PanelSection title={t("agent.panel.tasks")}>
            {hasWork ? (
              <ReadyWork rows={ready} here={here ?? []} />
            ) : (
              <p className="arnone t-caption">{t("agent.panel.noTasks")}</p>
            )}
          </PanelSection>
        )}
        {advises && (
          <PanelSection title={t("agent.panel.recommended")}>
            <Recommended signals={signals} jobs={jobs ?? []} />
          </PanelSection>
        )}
      </div>

      <div className="artab" hidden={tab !== "activity"}>
        <PanelSection
          title={t("agent.panel.recent")}
          action={
            mayOpenLog ? (
              <AppLink className="link-button" href={AI_SETTINGS_HREF}>
                {t("agent.panel.fullLog")}
              </AppLink>
            ) : undefined
          }
        >
          <Recap settled={settled} />
        </PanelSection>
      </div>

      <div className="artab" hidden={tab !== "system"}>
        <PanelSection title={t("agent.panel.runtime")}>
          <RuntimeFacts
            offline={signals.offline}
            model={model}
            ai={signals.ai}
            license={signals.license}
            licenseLine={signals.licenseLine}
          />
        </PanelSection>
        {/* The one control here, under everything that reports. */}
        <EdgeLightSetting />
      </div>

      {/* The switch between the bodies closes the panel, as its dock. */}
      <div className="artabs">
        <SegmentedControl
          options={TABS}
          value={tab}
          onChange={setTab}
          label={t("agent.tab.label")}
          labels={{
            now: t("agent.tab.now"),
            activity: t("agent.tab.activity"),
            system: t("agent.tab.system"),
          }}
        />
      </div>

      {/* Last, and the only line here no read produced: the agent reaches no
          further than this reader does, whatever the rows above managed to
          answer. A footnote because it is a standing promise rather than news.
          Held by AC-shell-8. */}
      <p className="arclaim t-caption">{t("shell.agent.scope")}</p>
    </section>
  );
}
