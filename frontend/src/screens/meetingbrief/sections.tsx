// The nine brief sections, arranged by what a reader does with them.
//
// The server sends them in ADR-0097 D5's reading order and this file does not
// re-sort that order for its own sake — it gives three of them a shape their
// job earns and leaves the rest as panels. The goal leads because burying the
// ask is the canonical prep failure. The risks are a callout because a
// watch-out a reader scrolls past is a watch-out they walk in without. The
// company background collapses because it is the one section that is context
// rather than preparation.
//
// A section the server did not send renders nothing at all: absent means the
// section had nothing to say, and a heading over an empty space tells a reader
// to look for something that is not there.

import { AlertTriangle } from "lucide-react";
import type { components } from "../../api/schema";
import { Disclosure } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { SentenceList } from "../record360";

type MeetingBrief = components["schemas"]["MeetingBrief"];
type BriefSection = components["schemas"]["MeetingBriefSection"];
type SectionKind = BriefSection["kind"];

// The order a reader reads them in, which is not the order they arrive in.
// `goal` and `risks` are drawn by their own components above this list, and
// `header` sits in the glance line, so the three are absent here.
const BODY_ORDER: SectionKind[] = [
  "what_changed",
  "commitments",
  "deal_state",
  "talking_points",
  "attendees",
];

type OpenRecord = (entityType: string, entityId: string) => void;

function find(
  sections: readonly BriefSection[],
  kind: SectionKind,
): BriefSection | undefined {
  return sections.find((section) => section.kind === kind);
}

// The meeting itself, in one line above the panels. No panel chrome: it is the
// answer to "which meeting is this", which a reader checks in a glance and
// never returns to.
export function GlanceLine({
  brief,
  onOpenRecord,
}: Readonly<{ brief: MeetingBrief; onOpenRecord: OpenRecord }>) {
  const header = find(brief.sections, "header");
  if (!header) {
    return null;
  }
  return (
    <section className="mb-glance">
      <SentenceList sentences={header.sentences} onOpenRecord={onOpenRecord} />
    </section>
  );
}

// The ask, in the lead panel. Its tone follows the writer: an indigo band means
// "Margince wrote this" everywhere in the product, so a deterministic brief —
// which is a composition over records, not a model's prose — takes the accent
// instead and is not dressed as something it is not.
export function GoalPanel({
  brief,
  onOpenRecord,
}: Readonly<{ brief: MeetingBrief; onOpenRecord: OpenRecord }>) {
  const t = useT();
  const goal = find(brief.sections, "goal");
  if (!goal) {
    return null;
  }
  return (
    <Panel
      title={t("person.meeting.goal")}
      titleLevel={3}
      tone={brief.generated_by === "model" ? "ai" : "accent"}
    >
      <PanelBody>
        <SentenceList
          sentences={goal.sentences}
          onOpenRecord={onOpenRecord}
          leadWithJudgement
        />
      </PanelBody>
    </Panel>
  );
}

// The watch-outs. A callout rather than a panel because this is the one
// section whose cost of being missed is a sentence said in the room that
// cannot be taken back.
export function RiskCallout({
  brief,
  onOpenRecord,
}: Readonly<{ brief: MeetingBrief; onOpenRecord: OpenRecord }>) {
  const t = useT();
  const risks = find(brief.sections, "risks");
  if (!risks) {
    return null;
  }
  // The heading is the SECTION's, not the callout's: a Callout titles itself
  // with a paragraph because it is an inline alert, and the risks are a named
  // part of the brief a reader navigates to. Putting the h3 outside keeps the
  // outline honest and still lets the callout draw the warning ground.
  return (
    <section className="mb-risks">
      <h3 className="mb-section-title">{t("person.meeting.risks")}</h3>
      <Callout tone="warn" icon={AlertTriangle}>
        <SentenceList sentences={risks.sentences} onOpenRecord={onOpenRecord} />
      </Callout>
    </section>
  );
}

// Everything between the ask and the background, each in its own panel under
// the heading the catalog names for it.
export function BodyPanels({
  brief,
  onOpenRecord,
}: Readonly<{ brief: MeetingBrief; onOpenRecord: OpenRecord }>) {
  const t = useT();
  return (
    <>
      {BODY_ORDER.map((kind) => {
        const section = find(brief.sections, kind);
        if (!section) {
          return null;
        }
        return (
          <Panel key={kind} title={t(`person.meeting.${kind}`)} titleLevel={3}>
            <PanelBody>
              <SentenceList
                sentences={section.sentences}
                onOpenRecord={onOpenRecord}
              />
            </PanelBody>
          </Panel>
        );
      })}
    </>
  );
}

// Background and what was withheld, behind one disclosure.
//
// Withheld sources are IN here rather than beside the prose because they are
// the same kind of thing: what this reader is not seeing. They are never
// silent — a brief that cannot see the Deal Room reads exactly like a brief
// about a deal whose buyer has done nothing, and a reader would walk in
// believing it.
export function Background({
  brief,
  onOpenRecord,
}: Readonly<{ brief: MeetingBrief; onOpenRecord: OpenRecord }>) {
  const t = useT();
  const context = find(brief.sections, "company_context");
  const omitted = brief.omitted ?? [];
  if (!context && omitted.length === 0) {
    return null;
  }
  return (
    <Disclosure summary={t("person.meeting.background")}>
      <div className="mb-background">
        {context && (
          <SentenceList
            sentences={context.sentences}
            onOpenRecord={onOpenRecord}
          />
        )}
        {omitted.map((omission) => (
          <SurfaceState
            key={omission.source}
            state="withheld"
            labelLevel="h4"
            label={t("person.meeting.omittedSource")}
            emptyLabel={t("person.meeting.empty")}
            detail={{ withheldReason: omission.reason }}
          >
            {null}
          </SurfaceState>
        ))}
      </div>
    </Disclosure>
  );
}
