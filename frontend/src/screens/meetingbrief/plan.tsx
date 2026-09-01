// The preparation plan: what to DO in the room.
//
// Rendered above the cited summary rather than instead of it. The server says
// how ready its plan is, and only a `prepared` one leads — an outline adds its
// objective and its arc on top of the sections a reader already had, so a
// half-built plan can never displace what was already working.

import { AlertTriangle, Target } from "lucide-react";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Eyebrow } from "../../design-system/eyebrow";
import { FactList } from "../../design-system/factlist";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";
import { SentenceList } from "../record360";

type MeetingBrief = components["schemas"]["MeetingBrief"];
type MeetingPlan = NonNullable<MeetingBrief["plan"]>;
type BriefSentence = components["schemas"]["OrganizationBriefSentence"];

type OpenRecord = (entityType: string, entityId: string) => void;

// One cited claim, with its receipts. The brief's own renderer rather than a
// second spelling of a citation.
function Claim({
  sentence,
  onOpenRecord,
}: Readonly<{ sentence: BriefSentence; onOpenRecord: OpenRecord }>) {
  return <SentenceList sentences={[sentence]} onOpenRecord={onOpenRecord} />;
}

// The outcome to earn, and the reminder not to force it. The lead panel, tinted
// by who wrote it: indigo means a model did, and a deterministic composition
// takes the accent instead rather than borrowing the claim.
export function ObjectivePanel({
  plan,
  onOpenRecord,
}: Readonly<{ plan: MeetingPlan; onOpenRecord: OpenRecord }>) {
  const t = useT();
  if (!plan.objective) {
    return null;
  }
  return (
    <Panel
      title={t("person.meeting.objective")}
      titleLevel={3}
      tone={plan.generated_by === "model" ? "ai" : "accent"}
    >
      <PanelBody>
        <div className="mb-objective">
          <Target aria-hidden="true" />
          <div>
            <Claim
              sentence={plan.objective.sentence}
              onOpenRecord={onOpenRecord}
            />
            <p className="mb-caveat">{plan.objective.caveat}</p>
          </div>
        </div>
        {plan.opening && (
          <div className="mb-open">
            <Eyebrow as="h4">{t("person.meeting.openWith")}</Eyebrow>
            <Claim sentence={plan.opening} onOpenRecord={onOpenRecord} />
          </div>
        )}
      </PanelBody>
    </Panel>
  );
}

// The moments that still bear on today, oldest first.
export function AccountArc({
  plan,
  onOpenRecord,
  formatDay,
}: Readonly<{
  plan: MeetingPlan;
  onOpenRecord: OpenRecord;
  formatDay: (utcIso: string) => string;
}>) {
  const t = useT();
  if (plan.account_arc.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("person.meeting.arc")}
      titleLevel={3}
      sub={t("person.meeting.arcSub")}
    >
      {plan.account_arc.map((moment) => (
        <PanelRow key={`${moment.from}-${moment.title}`}>
          <div className="mb-arc-row">
            <time dateTime={moment.from}>{formatDay(moment.from)}</time>
            <div>
              {moment.title && <strong>{moment.title}</strong>}
              <Claim sentence={moment.summary} onOpenRecord={onOpenRecord} />
            </div>
          </div>
        </PanelRow>
      ))}
    </Panel>
  );
}

// The three ways this meeting can end well. A meeting that ends with none of
// them ended with nothing, which is what the three columns are for.
export function AdvancePanel({
  plan,
  onOpenRecord,
}: Readonly<{ plan: MeetingPlan; onOpenRecord: OpenRecord }>) {
  const t = useT();
  const legs = [
    { key: "minimum", sentence: plan.advance.minimum },
    { key: "best", sentence: plan.advance.best },
    { key: "fallback", sentence: plan.advance.fallback },
  ] as const;
  return (
    <Panel title={t("person.meeting.close")} titleLevel={3} tone="accent">
      <PanelBody>
        <div className="mb-advance">
          {legs.map((leg) => (
            <div key={leg.key}>
              <Eyebrow as="h4">
                {t(`person.meeting.advance.${leg.key}`)}
              </Eyebrow>
              <Claim sentence={leg.sentence} onOpenRecord={onOpenRecord} />
            </div>
          ))}
        </div>
      </PanelBody>
    </Panel>
  );
}

// What the record does not say, and the question that closes each gap.
//
// Shown rather than hidden: a gap a reader does not know about is one they
// walk in assuming was covered, and every line here is a question they can
// ask in the room.
export function Unknowns({ plan }: Readonly<{ plan: MeetingPlan }>) {
  const t = useT();
  if (plan.unknowns.length === 0) {
    return null;
  }
  return (
    <Panel title={t("person.meeting.unknowns")} titleLevel={3}>
      {plan.unknowns.map((unknown) => (
        <PanelRow key={unknown.kind}>
          <span className="mb-unknown">{unknown.question}</span>
        </PanelRow>
      ))}
    </Panel>
  );
}

// What they are likely to ask us, each with the record we expect it from.
//
// Ordered by how likely it is to come up, and shown with its basis rather than
// as a bare list: a hypothesis a reader cannot check is one they either trust
// blindly or ignore, and both are worse than not printing it.
export function LikelyAsks({
  plan,
  onOpenRecord,
}: Readonly<{ plan: MeetingPlan; onOpenRecord: OpenRecord }>) {
  const t = useT();
  if (plan.likely_asks.length === 0) {
    return null;
  }
  return (
    <Panel title={t("person.meeting.likelyAsks")} titleLevel={3}>
      {plan.likely_asks.map((ask) => (
        <PanelRow key={ask.question}>
          <div className="mb-ask">
            <div className="mb-ask-head">
              <strong>{ask.question}</strong>
              <Badge tone={ask.relevance === "high" ? "warn" : undefined} quiet>
                {t(`person.meeting.relevance.${ask.relevance}`)}
              </Badge>
            </div>
            <Claim sentence={ask.basis} onOpenRecord={onOpenRecord} />
            <p className="mb-ask-prepare">{ask.prepare}</p>
          </div>
        </PanelRow>
      ))}
    </Panel>
  );
}

// The one watch-out, with what to say, show and not promise.
export function TopRisk({
  plan,
  onOpenRecord,
}: Readonly<{ plan: MeetingPlan; onOpenRecord: OpenRecord }>) {
  const t = useT();
  if (!plan.top_risk) {
    return null;
  }
  const { response_plan: response } = plan.top_risk;
  return (
    <section className="mb-risks">
      <h3 className="mb-section-title">{t("person.meeting.beReady")}</h3>
      <Callout tone="warn" icon={AlertTriangle}>
        <Claim sentence={plan.top_risk.text} onOpenRecord={onOpenRecord} />
        <FactList
          facts={[
            { key: "say", term: t("person.meeting.say"), value: response.say },
            {
              key: "show",
              term: t("person.meeting.show"),
              value: response.show,
            },
            {
              key: "avoid",
              term: t("person.meeting.avoid"),
              value: response.avoid,
            },
          ]}
        />
      </Callout>
    </section>
  );
}

// What the meeting may turn into, and what to do if it does.
export function Scenarios({ plan }: Readonly<{ plan: MeetingPlan }>) {
  const t = useT();
  if (plan.scenarios.length === 0) {
    return null;
  }
  return (
    <Panel title={t("person.meeting.scenarios")} titleLevel={3}>
      {plan.scenarios.map((scenario) => (
        <PanelRow key={scenario.label}>
          <div className="mb-path-row">
            <Badge quiet>{scenario.label}</Badge>
            <span>{scenario.play}</span>
          </div>
        </PanelRow>
      ))}
    </Panel>
  );
}
