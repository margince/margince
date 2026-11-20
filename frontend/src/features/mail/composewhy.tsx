import { ShieldAlert } from "lucide-react";
import { Field, TextInput } from "../../design-system/atoms";
import { DateInput } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import type { ISODate } from "../../format/datepattern";
import { useT } from "../../i18n";
import {
  asksWhy,
  type CommunicationContext,
  type ComposeReason,
  contextOfReason,
  reasonHint,
  reasonOf,
  reasonOptions,
} from "../../lib/compose-context";
import type { Activity } from "./compose";
import type { Meeting } from "./composemeeting";
import type { MissingField } from "./composesend";

// Why the reader is writing, asked where the record does not answer it. A
// reply derives its category from the thread it answers, which the checks say.
// The line under the pick says what the answer does to the message, so a rep
// sees that only marketing carries an unsubscribe link.
export function ComposeWhy({
  anchorActivity,
  needsReason,
  flagged,
  context,
  onContextChange,
  meeting,
}: Readonly<{
  anchorActivity: Activity | undefined;
  /** The check could not settle why a reply is written, or a reason is set. */
  needsReason: boolean;
  flagged: ReadonlySet<MissingField>;
  context: CommunicationContext | "";
  onContextChange: (next: CommunicationContext | "") => void;
  meeting: Meeting;
}>) {
  const t = useT();
  if (!asksWhy(anchorActivity, needsReason || meeting.on)) {
    return null;
  }
  const reason = reasonOf(context, meeting.on);
  const missing = reason === "";
  const choose = (next: ComposeReason | "") => {
    meeting.setOn(next === "in_person");
    onContextChange(next === "" ? "" : contextOfReason(next));
  };
  // One strip in the warning band: what is missing and why it matters, with
  // the pick that fixes it at the end. Once picked it says what the pick does.
  return (
    <div
      className={
        missing ? "compose-strip compose-strip-warning" : "compose-strip"
      }
    >
      {missing && <ShieldAlert aria-hidden="true" />}
      <p className="compose-strip-text">
        {missing ? (
          <>
            <strong>{t("compose.whyMissing")}</strong>{" "}
            {t("compose.whyMissingText")}
          </>
        ) : (
          <>
            <strong>{t("compose.why")}:</strong> {t(reasonHint(reason))}
          </>
        )}
      </p>
      <Field
        className="compose-strip-field"
        label={t("compose.why")}
        labelHidden
        error={flagged.has("context") ? t("compose.missingWhy") : undefined}
      >
        {(control) => (
          <Select
            {...control}
            options={reasonOptions(t, meeting.canRecord)}
            value={reason}
            placeholder={t("compose.whyPick")}
            onChange={(value) => choose(value as ComposeReason | "")}
          />
        )}
      </Field>
      {meeting.on && <MeetingFields meeting={meeting} flagged={flagged} />}
    </div>
  );
}

// Where and when the rep met the contact. Where is the evidence; the day
// starts on today and cannot lie ahead.
function MeetingFields({
  meeting,
  flagged,
}: Readonly<{ meeting: Meeting; flagged: ReadonlySet<MissingField> }>) {
  const t = useT();
  return (
    <div className="compose-strip-meeting">
      <Field
        label={t("compose.met.where")}
        required
        error={flagged.has("meeting") ? t("compose.met.missing") : undefined}
      >
        {(control) => (
          <TextInput
            {...control}
            value={meeting.where}
            maxLength={500}
            placeholder={t("compose.met.wherePlaceholder")}
            onChange={(event) => meeting.setWhere(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("compose.met.when")}>
        {(control) => (
          <DateInput
            {...control}
            value={meeting.day}
            max={meeting.today as ISODate}
            today={meeting.today as ISODate}
            onChange={(event) => meeting.setDay(event.target.value)}
          />
        )}
      </Field>
      {meeting.failed && <ErrorLine>{t("compose.met.failed")}</ErrorLine>}
    </div>
  );
}
