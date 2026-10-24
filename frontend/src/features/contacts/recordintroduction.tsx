// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Recording an introduction that already happened outside Margince.
//
// Built from the design-system parts as they are: the dialog waits for a
// design. The server decides who may record, so this form only refuses what is
// plainly incomplete.

import { useId, useState } from "react";
import { useRoster } from "../../components/roster";
import { Button, Field, Modal } from "../../design-system/atoms";
import {
  DateInput,
  type ISODate,
  isISODate,
} from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
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
  const titleId = useId();
  const viewerId = useViewerId();
  const roster = useRoster("user", open);
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
  const ready =
    introducer !== "" &&
    introduced !== "" &&
    introducer !== introduced &&
    isISODate(day) &&
    channel !== "";

  const submit = () => {
    if (!ready) {
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
      <div className="form-stack">
        <p>{t("contact.intro.recordHint")}</p>
        <Field label={t("contact.intro.recordIntroducer")}>
          {(control) => (
            <Select
              {...control}
              options={colleagues}
              value={introducer}
              onChange={setIntroducer}
            />
          )}
        </Field>
        <Field label={t("contact.intro.recordIntroduced")}>
          {(control) => (
            <Select
              {...control}
              options={colleagues}
              value={introduced}
              onChange={setIntroduced}
            />
          )}
        </Field>
        <Field label={t("contact.intro.recordDate")}>
          {(control) => (
            <DateInput
              {...control}
              value={day}
              onChange={(e) =>
                setDay(isISODate(e.target.value) ? e.target.value : "")
              }
            />
          )}
        </Field>
        <Field label={t("contact.intro.recordChannel")}>
          {(control) => (
            <Select
              {...control}
              options={CHANNELS.map((c) => ({
                value: c.value,
                label: t(c.label),
              }))}
              value={channel}
              onChange={(v) =>
                setChannel(CHANNELS.find((c) => c.value === v)?.value ?? "")
              }
            />
          )}
        </Field>
        {record.isError ? (
          <ErrorLine>{t("contact.intro.recordFailed")}</ErrorLine>
        ) : null}
      </div>
      <div className="actions">
        <Button onClick={onClose} variant="ghost">
          {t("contact.intro.cancel")}
        </Button>
        <Button onClick={submit} disabled={!ready} pending={record.isPending}>
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
