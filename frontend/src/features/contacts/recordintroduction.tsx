// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Recording an introduction that already happened outside Margince.
//
// Built from the design-system parts as they are: the dialog waits for a
// design. The server decides who may record, so this form only refuses what is
// plainly incomplete.

import { useId, useState } from "react";
import {
  RosterFailedLine,
  RosterPartialNote,
  useRoster,
  useRosterPartial,
} from "../../components/roster";
import { Button, Field, Modal } from "../../design-system/atoms";
import {
  DateInput,
  type ISODate,
  isISODate,
} from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { useLocale, useT } from "../../i18n";
import { blankRequired, problemStatus } from "../../lib/problem";
import { useViewerId } from "../../lib/session";
import {
  type RecordIntroductionInput,
  useRecordIntroduction,
} from "./introrequests";

type Channel = RecordIntroductionInput["channel"];

const CHANNELS: readonly {
  value: Channel;
  label:
    | "contact.intro.channelInPerson"
    | "contact.intro.channelEmail"
    | "contact.intro.channelCall"
    | "contact.intro.channelOther";
}[] = [
  { value: "in_person", label: "contact.intro.channelInPerson" },
  { value: "email", label: "contact.intro.channelEmail" },
  { value: "call", label: "contact.intro.channelCall" },
  { value: "other", label: "contact.intro.channelOther" },
];

function failureKey(status: number) {
  switch (status) {
    case 403:
      return "contact.intro.recordFailed";
    case 409:
      return "contact.intro.recordDuplicate";
    case 404:
      return "contact.intro.recordGone";
    case 422:
      return "contact.intro.recordInvalid";
    default:
      return "contact.intro.recordFailedGeneric";
  }
}

/** RecordIntroductionDialog collects the two colleagues, the day and the channel. */
export function RecordIntroductionDialog({
  contactId,
  contactName,
  open,
  onClose,
}: Readonly<{
  contactId: string;
  contactName: string;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const titleId = useId();
  const viewerId = useViewerId();
  const roster = useRoster("user", open);
  const rosterPartial = useRosterPartial("user", open);
  const [introducer, setIntroducer] = useState("");
  const [introduced, setIntroduced] = useState(viewerId ?? "");
  const [day, setDay] = useState<ISODate | "">("");
  const [channel, setChannel] = useState<Channel | "">("");
  const record = useRecordIntroduction(contactId);

  const colleagues = (roster.data ?? []).flatMap((entry) =>
    "display_name" in entry
      ? [{ value: entry.id, label: entry.display_name }]
      : [],
  );
  const labels = {
    introducer: t("contact.intro.recordIntroducer"),
    introduced: t("contact.intro.recordIntroduced"),
    day: t("contact.intro.recordDate"),
    channel: t("contact.intro.recordChannel"),
  };
  // One colleague cannot introduce themselves; the two picks must differ.
  const sameColleague = Boolean(introducer) && introducer === introduced;
  const edit =
    <V,>(key: string, set: (value: V) => void) =>
    (value: V) => {
      set(value);
      faults.edited(key);
    };

  const submit = () => {
    const blank = blankRequired(locale, {
      introducer: [labels.introducer, introducer],
      introduced: [labels.introduced, introduced],
      day: [labels.day, day],
      channel: [labels.channel, channel],
    });
    if (faults.show(blank) || sameColleague || !isISODate(day) || !channel) {
      return;
    }
    record.mutate(
      {
        introducer_user_id: introducer,
        introduced_user_id: introduced,
        introduced_on: day,
        channel,
      },
      { onSuccess: () => onClose() },
    );
  };

  return (
    <Modal open={open} onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="modal-title">
        {t("contact.intro.recordTitle", { name: contactName })}
      </Heading>
      <div className="form-stack" ref={faultsRoot}>
        <p>{t("contact.intro.recordHint")}</p>
        <Field
          label={labels.introducer}
          required
          error={faults.of("introducer")}
        >
          {(control) => (
            <Select
              {...control}
              options={colleagues}
              value={introducer}
              onChange={edit("introducer", setIntroducer)}
            />
          )}
        </Field>
        <Field
          label={labels.introduced}
          required
          error={faults.of("introduced")}
        >
          {(control) => (
            <Select
              {...control}
              options={colleagues}
              value={introduced}
              onChange={edit("introduced", setIntroduced)}
            />
          )}
        </Field>
        <Field label={labels.day} required error={faults.of("day")}>
          {(control) => (
            <DateInput
              {...control}
              value={day}
              onChange={(e) =>
                edit(
                  "day",
                  setDay,
                )(isISODate(e.target.value) ? e.target.value : "")
              }
            />
          )}
        </Field>
        <Field label={labels.channel} required error={faults.of("channel")}>
          {(control) => (
            <Select
              {...control}
              options={CHANNELS.map((c) => ({
                value: c.value,
                label: t(c.label),
              }))}
              value={channel}
              onChange={(v) =>
                edit(
                  "channel",
                  setChannel,
                )(CHANNELS.find((c) => c.value === v)?.value ?? "")
              }
            />
          )}
        </Field>
        <RosterFailedLine kind="user" roster={roster} />
        <RosterPartialNote partial={rosterPartial} />
        {record.isError ? (
          <ErrorLine>{t(failureKey(problemStatus(record.error)))}</ErrorLine>
        ) : null}
      </div>
      <div className="actions">
        <Button onClick={onClose} variant="ghost">
          {t("contact.intro.cancel")}
        </Button>
        <Button
          onClick={submit}
          disabled={sameColleague}
          pending={record.isPending}
        >
          {t("contact.intro.recordSubmit")}
        </Button>
      </div>
    </Modal>
  );
}

/** RecordIntroductionButton opens the dialog from the Network tab. */
export function RecordIntroductionButton({
  contactId,
  contactName,
}: Readonly<{ contactId: string; contactName: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="ghost" onClick={() => setOpen(true)}>
        {t("contact.intro.recordAction")}
      </Button>
      {open ? (
        <RecordIntroductionDialog
          contactId={contactId}
          contactName={contactName}
          open
          onClose={() => setOpen(false)}
        />
      ) : null}
    </>
  );
}
