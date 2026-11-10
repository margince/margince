// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field, Textarea } from "../../design-system/atoms";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { Select } from "../../design-system/select";
import { type ClockTime, TimeInput } from "../../design-system/timeinput";
import { dueInstant } from "../../format/calendarday";
import { useT } from "../../i18n";
import { useCan, useCanMutate } from "../../lib/capability";
import { contact360Query } from "../../lib/contact.queries";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import type { ReasonTarget } from "./composereason";

type Context = components["schemas"]["CommunicationContext"];
type Fact = components["schemas"]["RecordQualifyingEventRequest"];
type Target = Readonly<{
  target: ReasonTarget;
  chosen?: Context;
  dealId?: string;
}>;

export function RecordConversationFact({
  target,
  chosen,
  dealId,
  renderOpener,
}: Readonly<{
  target?: ReasonTarget;
  chosen?: Context;
  dealId?: string;
  renderOpener?: (opener: ReactNode) => ReactNode;
}>) {
  const t = useT();
  const mayWriteContact = useCan("contact", "update");
  const mayWriteLead = useCan("lead", "update");
  const mayWrite = (subject: ReasonTarget["subject"] | undefined) =>
    subject?.entity_type === "contact"
      ? mayWriteContact
      : subject?.entity_type === "lead" && mayWriteLead;
  const mayMutate = useCanMutate();
  const [opened, setOpened] = useState<Target>();
  const available =
    mayWrite(target?.subject) &&
    mayMutate &&
    target &&
    (target.category !== "active_deal_followup" || dealId);
  const opener = available && (
    <Button onClick={() => setOpened({ target, chosen, dealId })}>
      {t("compose.fact.open")}
    </Button>
  );
  return (
    <>
      {renderOpener ? renderOpener(opener) : opener}
      {opened && mayWrite(opened.target.subject) && mayMutate && (
        <ConversationFactDialog
          {...opened}
          onClose={() => setOpened(undefined)}
        />
      )}
    </>
  );
}

function ConversationFactDialog({
  target,
  chosen,
  dealId,
  onClose,
}: Target & { onClose: () => void }) {
  const t = useT();
  const zone = useViewerZone();
  const queries = useQueryClient();
  const [kind, setKind] = useState<Fact["kind"] | "">("");
  const [note, setNote] = useState("");
  const [day, setDay] = useState<ISODate | "">("");
  const [time, setTime] = useState<ClockTime | "">("");
  const quote =
    chosen === "precontract_quote" || target.category === "precontract_quote";
  const record = useMutation({
    mutationFn: async (ask: {
      subject: ReasonTarget["subject"];
      fact: Fact;
    }) => {
      const endpoint =
        ask.subject.entity_type === "lead"
          ? "/leads/{id}/consent/qualifying-events"
          : "/contacts/{id}/consent/qualifying-events";
      unwrap(
        await api.POST(endpoint, {
          params: { path: { id: ask.subject.entity_id } },
          body: ask.fact,
        }),
      );
    },
    onSuccess: async (_, ask) => {
      if (ask.subject.entity_type === "contact")
        await Promise.all(
          [
            ["contactConsentGuard", ask.subject.entity_id],
            ["contact-consent", ask.subject.entity_id],
            contact360Query(ask.subject.entity_id).queryKey,
          ].map((queryKey) => queries.invalidateQueries({ queryKey })),
        );
      onClose();
    },
  });
  const options = [
    {
      value: "requested_by_subject",
      label: t(quote ? "compose.fact.quoteRequest" : "compose.fact.request"),
    },
  ];
  if (!quote && target.category !== "active_deal_followup")
    options.push({ value: "in_person", label: t("compose.fact.met") });
  return (
    <RecordFormDialog
      open
      title={t("compose.fact.title", { address: target.address })}
      onClose={onClose}
      fieldCount={4}
      closeDisabled={record.isPending}
      unsaved={Boolean(kind || note || day || time)}
      form={
        <div className="form-stack">
          <p>{t("compose.fact.scope", { address: target.address })}</p>
          <Field label={t("compose.fact.kind")}>
            {(control) => (
              <Select
                {...control}
                value={kind}
                options={options}
                disabled={record.isPending}
                onChange={(next) => {
                  if (next === "in_person" || next === "requested_by_subject")
                    setKind(next);
                }}
              />
            )}
          </Field>
          <Field label={t("compose.fact.note")}>
            {(control) => (
              <Textarea
                {...control}
                grow
                value={note}
                maxLength={500}
                disabled={record.isPending}
                onChange={(event) => setNote(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("compose.scheduleDate")}>
            {(control) => (
              <DateInput
                {...control}
                value={day}
                disabled={record.isPending}
                onChange={(event) => setDay(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("compose.scheduleTime")}>
            {(control) => (
              <TimeInput
                {...control}
                value={time}
                disabled={record.isPending}
                onChange={(event) => setTime(event.target.value)}
              />
            )}
          </Field>
          {record.isError && (
            <ErrorLine>{problemMessageOf(record.error, t)}</ErrorLine>
          )}
        </div>
      }
      actions={
        <>
          <Button variant="ghost" disabled={record.isPending} onClick={onClose}>
            {t("evidence.cancel")}
          </Button>
          <Button
            pending={record.isPending}
            disabled={!kind || !note.trim() || !day || !time}
            onClick={(event) => {
              if (
                refuseUnreadableField(event.currentTarget) ||
                !kind ||
                !day ||
                !time
              )
                return;
              record.mutate({
                subject: target.subject,
                fact: {
                  kind,
                  note: note.trim(),
                  occurred_at: dueInstant(day, zone, time),
                  asks_for_quote: kind === "requested_by_subject" && quote,
                  ...(dealId ? { about_deal: dealId } : {}),
                },
              });
            }}
          >
            {t("compose.fact.save")}
          </Button>
        </>
      }
    />
  );
}
