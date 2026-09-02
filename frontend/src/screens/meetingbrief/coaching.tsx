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
      title={t("person.meeting.coach.title")}
      titleLevel={3}
      tone={writtenByModel ? "ai" : "accent"}
      titleAction={<Badge quiet>{t("person.meeting.coach.eyebrow")}</Badge>}
    >
      <PanelBody>
        <p className="mb-coach-lead">{coaching.focus}</p>
        <p className="mb-coach-mode">{coaching.failure_mode}</p>
        <FactList
          facts={[
            {
              key: "listen",
              term: t("person.meeting.coach.listenFor"),
              value: coaching.listen_for,
            },
            {
              key: "watch",
              term: t("person.meeting.coach.watchFor"),
              value: coaching.watch_for,
            },
            {
              key: "intervene",
              term: t("person.meeting.coach.interveneIf"),
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
export function MeetingPaths({ coaching }: Readonly<{ coaching: Coaching }>) {
  const t = useT();
  if (coaching.paths.length === 0) {
    return null;
  }
  return (
    <Panel title={t("person.meeting.coach.paths")} titleLevel={3}>
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
