// The brief's header band: which meeting, prepared for whom, and by which
// writer.
//
// The meeting's own facts arrive as props, because the wire does not carry
// them: `MeetingBrief` is the prepared prose. A caller that holds none passes
// none, and the band falls back to the brief's own `header` section.

import type { components } from "../../../api/schema";
import { Avatar } from "../../../design-system/atoms";
import { useT } from "../../../i18n";
import { WrittenBy } from "../../records/record360";

type MeetingBrief = components["schemas"]["MeetingBrief"];

// What the opening page knows about the room. Every field is optional because
// the three callers hold different amounts of it.
export type MeetingFacts = Readonly<{
  subject?: string | null;
  participants?: readonly { contact_id: string; full_name: string }[];
}>;

// Who the brief is about, for the line under the title.
export type PreparedFor = Readonly<{
  name: string;
  identity: string;
  companyName?: string;
}>;

export function BriefHeader({
  brief,
  meeting,
  preparedFor,
}: Readonly<{
  brief?: MeetingBrief;
  meeting?: MeetingFacts;
  preparedFor?: PreparedFor;
}>) {
  const t = useT();
  return (
    <>
      {preparedFor && (
        <p className="mb-prepared-for t-sub">
          {preparedFor.companyName
            ? t("contact.meeting.preparedForAt", {
                name: preparedFor.name,
                company: preparedFor.companyName,
              })
            : t("contact.meeting.preparedFor", { name: preparedFor.name })}
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
