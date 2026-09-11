// The coaching layer, for a lead reading a teammate's meeting.
//
// Its presence is the SERVER's answer, never a client's question: the drawer
// renders what arrived and asks nobody's role. A client that decided this for
// itself would be deciding it for whoever it was pointed at.
//
// It leads the body, above the outcome to earn, because a lead opening this is
// preparing to coach rather than to run the meeting — and it says plainly that
// the rest of the page is the rep's own brief, unchanged.

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { FactList } from "../../design-system/factlist";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";

type MeetingBrief = components["schemas"]["MeetingBrief"];
type MeetingPlan = NonNullable<MeetingBrief["plan"]>;
type Coaching = NonNullable<MeetingPlan["manager_coaching"]>;

export function CoachPanel({
  coaching,
  writtenByModel,
}: Readonly<{ coaching: Coaching; writtenByModel: boolean }>) {
  const t = useT();
  return (
    <Panel
      title={t("contact.meeting.coach.title")}
      titleLevel={3}
      tone={writtenByModel ? "ai" : "accent"}
      titleAction={<Badge quiet>{t("contact.meeting.coach.eyebrow")}</Badge>}
    >
      <PanelBody>
        <p className="mb-coach-lead">{coaching.focus}</p>
        <p className="mb-coach-mode">{coaching.failure_mode}</p>
        <FactList
          facts={[
            {
              key: "listen",
              term: t("contact.meeting.coach.listenFor"),
              value: coaching.listen_for,
            },
            {
              key: "watch",
              term: t("contact.meeting.coach.watchFor"),
              value: coaching.watch_for,
            },
            {
              key: "intervene",
              term: t("contact.meeting.coach.interveneIf"),
              value: coaching.intervene_if,
            },
          ]}
        />
      </PanelBody>
    </Panel>
  );
}

// The branches a lead rehearses against. The same ones the rep's own plan
// carries, so the two are preparing for one meeting.
export function MeetingPaths({
  coaching,
  writtenByModel,
}: Readonly<{ coaching: Coaching; writtenByModel: boolean }>) {
  const t = useT();
  if (coaching.paths.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("contact.meeting.coach.paths")}
      titleLevel={3}
      // The same layer the panel above draws, so the same writer wrote it. A
      // lead reading one tinted card beside an untinted one would take the two
      // for two different readings.
      tone={writtenByModel ? "ai" : undefined}
    >
      {coaching.paths.map((path) => (
        <PanelRow key={path.label}>
          <div className="mb-path-row">
            <Badge quiet>{path.label}</Badge>
            <span>{path.play}</span>
          </div>
        </PanelRow>
      ))}
    </Panel>
  );
}
