// The brief's header band: which meeting, prepared for whom, and by which
// writer.
//
// The meeting's own facts arrive as props rather than from the brief, because
// the wire does not carry them: `MeetingBrief` is the prepared prose, and the
// subject, the time and the person are what the page that opened the drawer
// already holds. A caller that holds none of it — the deal page opens this
// from an activity id alone — passes none, and the band falls back to the
// brief's own `header` section rather than inventing a line.

import type { components } from "../../api/schema";
import { Avatar } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { WrittenBy } from "../record360";

type MeetingBrief = components["schemas"]["MeetingBrief"];

// What the opening page knows about the room. Every field is optional because
// the three callers hold different amounts of it.
export type MeetingFacts = Readonly<{
  subject?: string | null;
  startsAt?: string;
  participants?: readonly { person_id: string; full_name: string }[];
}>;

// Who the brief is about, for the line under the title.
export type PreparedFor = Readonly<{
  name: string;
  identity: string;
  organizationName?: string;
}>;

export function BriefHeader({
  brief,
  meeting,
  preparedFor,
  formatWhen,
}: Readonly<{
  brief?: MeetingBrief;
  meeting?: MeetingFacts;
  preparedFor?: PreparedFor;
  // Formatting belongs to the caller: this tier holds no locale and no zone,
  // exactly as SurfaceState's `staleAsOf` does.
  formatWhen?: (utcIso: string) => string;
}>) {
  const t = useT();
  const when =
    meeting?.startsAt && formatWhen ? formatWhen(meeting.startsAt) : undefined;
  return (
    <>
      {preparedFor && (
        <p className="mb-prepared-for">
          {preparedFor.organizationName
            ? t("person.meeting.preparedForAt", {
                name: preparedFor.name,
                org: preparedFor.organizationName,
              })
            : t("person.meeting.preparedFor", { name: preparedFor.name })}
        </p>
      )}
      {meeting?.subject && (
        <div className="mb-meeting-line">
          {preparedFor && (
            <Avatar
              name={preparedFor.name}
              identity={preparedFor.identity}
              size="md"
            />
          )}
          <div className="mb-meeting-text">
            <strong>{meeting.subject}</strong>
            {when && <span>{when}</span>}
          </div>
        </div>
      )}
      {brief && (
        <div className="mb-badges">
          <WrittenBy by={brief.generated_by} />
        </div>
      )}
    </>
  );
}
