// When a Send anyway goes: now, or at a moment the sender picks. A later
// moment records the decision now and the message waits for it.

import { useState } from "react";
import { ChoiceList } from "../../design-system/choicelist";
import { useNow } from "../../format/now";
import { useT } from "../../i18n";
import { MomentFields, useMomentChoice } from "./scheduledmoment";

type When = "now" | "later";

/** The choice behind the dialog, picked in `zone`: the held message's own. */
export function useSendAnywayWhen(zone: string) {
  const t = useT();
  const now = new Date(useNow(60_000));
  const [when, setWhen] = useState<When>("now");
  const moment = useMomentChoice(zone, now);
  const later = when === "later" ? moment.fields : undefined;
  const missing = () => {
    if (when !== "later" || moment.ready) return undefined;
    return t(moment.past ? "compose.schedulePast" : "compose.schedulePick");
  };
  return {
    when,
    setWhen,
    moment,
    /** The moment to send at, or undefined to send now. */
    later,
    /** Why confirm waits: a later moment that is not whole, or is past. */
    missing: missing(),
    confirmLabel: t(
      later?.scheduled_at ? "compose.scheduleSend" : "directSend.confirm",
    ),
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
        <MomentFields
          choice={state.moment}
          dayLabel={t("compose.scheduleDate")}
          timeLabel={t("compose.scheduleTime")}
        />
      )}
    </>
  );
}
