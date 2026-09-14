// The preparation plan: what to DO in the room.
//
// Rendered above the cited summary rather than instead of it. The server says
// how ready its plan is, and only a `prepared` one leads — an outline adds its
// objective and its arc on top of the sections a reader already had, so a
// half-built plan can never displace what was already working.

import { Target } from "lucide-react";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { FactList } from "../../design-system/factlist";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";
import { SentenceList } from "../record360";

type MeetingBrief = components["schemas"]["MeetingBrief"];
type MeetingPlan = NonNullable<MeetingBrief["plan"]>;
type BriefSentence = components["schemas"]["CompanyBriefSentence"];

type OpenRecord = (entityType: string, entityId: string) => void;

// Opens a cited message in the host's own email drawer. Threaded beside
// onOpenRecord for the same reason: the brief cites the conversations it was
// written from, and a citation that names a message should open it.
type OpenEmail = (activityId: string) => void;

// One cited claim, with its receipts. The brief's own renderer rather than a
// second spelling of a citation.
function Claim({
  sentence,
  onOpenRecord,
  onOpenEmail,
}: Readonly<{
  sentence: BriefSentence;
  onOpenRecord: OpenRecord;
  onOpenEmail?: OpenEmail;
}>) {
  return (
    <SentenceList
      sentences={[sentence]}
      onOpenRecord={onOpenRecord}
      onOpenEmail={onOpenEmail}
    />
  );
}

// The outcome to earn, and the reminder not to force it. The lead panel, tinted
// by who wrote it: indigo means a model did, and a deterministic composition
// takes the accent instead rather than borrowing the claim.
export function ObjectivePanel({
  plan,
  onOpenRecord,
  onOpenEmail,
}: Readonly<{
  plan: MeetingPlan;
  onOpenRecord: OpenRecord;
  onOpenEmail?: OpenEmail;
}>) {
  const t = useT();
  if (!plan.objective) {
    return null;
  }
  const byModel = plan.generated_by === "model";
  return (
    <Panel
      title={t("contact.meeting.objective")}
      titleLevel={3}
      tone={byModel ? "ai" : "accent"}
      // The disclosure rides the plan's LEAD and nowhere else: the panels under
      // it are the same writer's work and take the same tint, but a badge on
      // each of them would be the same sentence said six times.
      titleAction={
        byModel ? <Badge tone="ai">{t("co.assistant.aiTag")}</Badge> : undefined
      }
    >
      <PanelBody>
        <div className="mb-objective">
          <Target aria-hidden="true" />
          <div>
            <Claim
              sentence={plan.objective.sentence}
              onOpenRecord={onOpenRecord}
              onOpenEmail={onOpenEmail}
            />
            <p className="mb-caveat">{plan.objective.caveat}</p>
          </div>
        </div>
        {plan.opening && (
          <div className="mb-open">
            <Eyebrow as="h4">{t("contact.meeting.openWith")}</Eyebrow>
            <Claim
              sentence={plan.opening}
              onOpenRecord={onOpenRecord}
              onOpenEmail={onOpenEmail}
            />
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
  onOpenEmail,
  formatDay,
}: Readonly<{
  plan: MeetingPlan;
  onOpenRecord: OpenRecord;
  onOpenEmail?: OpenEmail;
  formatDay: (utcIso: string) => string;
}>) {
  const t = useT();
  if (plan.account_arc.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("contact.meeting.arc")}
      titleLevel={3}
      sub={t("contact.meeting.arcSub")}
      tone={plan.generated_by === "model" ? "ai" : undefined}
    >
      {plan.account_arc.map((moment) => (
        <PanelRow key={`${moment.from}-${moment.title}`}>
          <div className="mb-arc-row">
            <time className="t-caption" dateTime={moment.from}>
              {formatDay(moment.from)}
            </time>
            <div>
              {moment.title && <strong>{moment.title}</strong>}
              <Claim
                sentence={moment.summary}
                onOpenRecord={onOpenRecord}
                onOpenEmail={onOpenEmail}
              />
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
  onOpenEmail,
}: Readonly<{
  plan: MeetingPlan;
  onOpenRecord: OpenRecord;
  onOpenEmail?: OpenEmail;
}>) {
  const t = useT();
  const legs = [
    { key: "minimum", sentence: plan.advance.minimum },
    { key: "best", sentence: plan.advance.best },
    { key: "fallback", sentence: plan.advance.fallback },
  ] as const;
  return (
    <Panel
      title={t("contact.meeting.close")}
      titleLevel={3}
      tone={plan.generated_by === "model" ? "ai" : "accent"}
    >
      <PanelBody>
        <div className="mb-advance">
          {legs.map((leg) => (
            <div key={leg.key}>
              <Eyebrow as="h4">
                {t(`contact.meeting.advance.${leg.key}`)}
              </Eyebrow>
              <Claim
                sentence={leg.sentence}
                onOpenRecord={onOpenRecord}
                onOpenEmail={onOpenEmail}
              />
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
    <Panel
      title={t("contact.meeting.unknowns")}
      titleLevel={3}
      tone={plan.generated_by === "model" ? "ai" : undefined}
    >
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
  onOpenEmail,
}: Readonly<{
  plan: MeetingPlan;
  onOpenRecord: OpenRecord;
  onOpenEmail?: OpenEmail;
}>) {
  const t = useT();
  if (plan.likely_asks.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("contact.meeting.likelyAsks")}
      titleLevel={3}
      tone={plan.generated_by === "model" ? "ai" : undefined}
    >
      {plan.likely_asks.map((ask) => (
        <PanelRow key={ask.question}>
          <div className="mb-ask">
            <div className="mb-ask-head">
              <strong>{ask.question}</strong>
              <Badge tone={ask.relevance === "high" ? "warn" : undefined} quiet>
                {t(`contact.meeting.relevance.${ask.relevance}`)}
              </Badge>
            </div>
            <Claim
              sentence={ask.basis}
              onOpenRecord={onOpenRecord}
              onOpenEmail={onOpenEmail}
            />
            <p className="mb-ask-prepare">{ask.prepare}</p>
          </div>
        </PanelRow>
      ))}
    </Panel>
  );
}

// The one watch-out, with what to say, show and not promise. A tinted panel for
// the same reason the sections' risk panel is one: this is the section whose
// FINDING is the bad news, and the tint follows the WRITER first — indigo is
// claimed for every panel of a model-written plan, so a warn tint here would be
// the one card of that plan not saying who wrote it.
//
// It was a `Callout` holding a claim and a FactList, which is content rather
// than something the surface says about itself — a notice's body is prose, and
// a document's section is a panel.
export function TopRisk({
  plan,
  onOpenRecord,
  onOpenEmail,
}: Readonly<{
  plan: MeetingPlan;
  onOpenRecord: OpenRecord;
  onOpenEmail?: OpenEmail;
}>) {
  const t = useT();
  if (!plan.top_risk) {
    return null;
  }
  const { response_plan: response } = plan.top_risk;
  return (
    <Panel
      title={t("contact.meeting.beReady")}
      titleLevel={3}
      tone={plan.generated_by === "model" ? "ai" : "warn"}
    >
      <PanelBody>
        <Claim
          sentence={plan.top_risk.text}
          onOpenRecord={onOpenRecord}
          onOpenEmail={onOpenEmail}
        />
        <FactList
          facts={[
            { key: "say", term: t("contact.meeting.say"), value: response.say },
            {
              key: "show",
              term: t("contact.meeting.show"),
              value: response.show,
            },
            {
              key: "avoid",
              term: t("contact.meeting.avoid"),
              value: response.avoid,
            },
          ]}
        />
      </PanelBody>
    </Panel>
  );
}

// What the meeting may turn into, and what to do if it does.
export function Scenarios({ plan }: Readonly<{ plan: MeetingPlan }>) {
  const t = useT();
  if (plan.scenarios.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("contact.meeting.scenarios")}
      titleLevel={3}
      tone={plan.generated_by === "model" ? "ai" : undefined}
    >
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
