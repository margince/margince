// When a Send anyway goes: now, or at a moment the sender picks. A later
// moment records the decision now and the message waits for it.

import { useState } from "react";
import { Field } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { type ClockTime, TimeInput } from "../../design-system/timeinput";
import { useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";
import { type ScheduleFields, scheduleFields } from "./composeschedule";

type When = "now" | "later";

export type SendAnywayWhen = Readonly<{
  /** The moment to send at, or undefined to send now. */
  later: ScheduleFields | undefined;
  /** True while "later" is chosen and no full moment is picked. */
  incomplete: boolean;
}>;

/** The state behind the choice, kept by the dialog that confirms. */
export function useSendAnywayWhen() {
  const t = useT();
  const zone = useViewerZone();
  const [when, setWhen] = useState<When>("now");
  const [day, setDay] = useState<ISODate | "">("");
  const [time, setTime] = useState<ClockTime | "">("");
  const fields =
    when === "later" && day && time
      ? scheduleFields(`${day}T${time}`, undefined, zone)
      : undefined;
  const chosen: SendAnywayWhen = {
    later: fields,
    incomplete: when === "later" && fields === undefined,
  };
  return {
    when,
    setWhen,
    day,
    setDay,
    time,
    setTime,
    chosen,
    /** Why confirm waits: a later moment is chosen and not yet complete. */
    missing: chosen.incomplete ? t("compose.schedulePick") : undefined,
    confirmLabel: t(fields ? "compose.scheduleSend" : "directSend.confirm"),
  };
}

export function DirectSendWhen({
  state,
}: Readonly<{ state: ReturnType<typeof useSendAnywayWhen> }>) {
  const t = useT();
  return (
    <>
      <ChoiceList<When>
        legend={t("directSend.when")}
        value={state.when}
        onChange={state.setWhen}
        choices={[
          { value: "now", label: t("compose.scheduleNow") },
          {
            value: "later",
            label: t("directSend.whenLater"),
            description: t("directSend.whenLaterDetail"),
          },
        ]}
      />
      {state.when === "later" && (
        <>
          <Field label={t("compose.scheduleDate")}>
            {(control) => (
              <DateInput
                {...control}
                value={state.day}
                onChange={(event) => state.setDay(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("compose.scheduleTime")}>
            {(control) => (
              <TimeInput
                {...control}
                value={state.time}
                onChange={(event) => state.setTime(event.target.value)}
              />
            )}
          </Field>
        </>
      )}
    </>
  );
}
