// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Field, Textarea } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DateInput, type ISODate } from "../design-system/dateinput";
import { FieldRow } from "../design-system/fieldgrid";
import { TimezoneSelect } from "../design-system/timezoneselect";
import { acquisitionKindLabel } from "../format/acquisitionkind";
import { startOfDayInZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { useCan, useCanWrite } from "../lib/capability";
import { acquisitionCaption } from "../lib/noticeacquisition";
import { problemMessageOf, unwrap } from "../lib/problem";
import {
  OTHER_KINDS,
  SOURCE_ANSWER_LABEL,
  SOURCE_ANSWERS,
  type SourceAnswer,
  type SourceKind,
  sourceKindFor,
} from "../lib/sourcequestion";
import { useViewerZone } from "../lib/viewerzone";

/**
 * Whether this reader may change a notice case. The server asks for
 * `privacy_request` update and read both (`requireNoticeCaseUpdate`).
 */
export function useCanChangeNoticeCase(): boolean {
  const read = useCan("privacy_request", "read");
  const update = useCanWrite("privacy_request", "update");
  return read && update;
}

/**
 * The source question for one review, as a drawer. It says why it is asked
 * and what is on record, asks "How did you receive their contact details?",
 * and says what saving does. The privacy queue and the contact page open it.
 */
export function SourceReviewDialog({
  caseId,
  open,
  onClose,
}: Readonly<{
  caseId: string;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const canWork = useCanChangeNoticeCase();
  const queryClient = useQueryClient();
  const [answer, setAnswer] = useState<SourceAnswer | "">("");
  const [other, setOther] = useState<SourceKind | "">("");
  const [date, setDate] = useState<ISODate | "">("");
  const readerZone = useViewerZone();
  const [zone, setZone] = useState(readerZone);
  const [note, setNote] = useState("");
  const kind = sourceKindFor(answer, other);
  const unsure = answer === "unsure";
  // A meeting also records itself as the exchange the communication-context
  // answer records, so the server needs the day it happened.
  const met = answer === "met";
  const assessment = useMutation({
    mutationFn: async (vars: {
      id: string;
      kind: SourceKind;
      obtainedAt: string | null;
      note: string;
      timezone: string;
      metInPerson: boolean;
    }) =>
      unwrap(
        await api.POST("/privacy/notice-case-reviews/{id}/source", {
          params: { path: { id: vars.id } },
          body: {
            kind: vars.kind,
            note: vars.note,
            obtained_at: vars.obtainedAt,
            timezone: vars.timezone,
            met_in_person: vars.metInPerson,
          },
        }),
      ),
    onSuccess: () => {
      onClose();
      for (const key of SOURCE_READS) {
        void queryClient.invalidateQueries({ queryKey: key });
      }
    },
  });
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      intent="drawer"
      title={t("notice.source.title")}
      confirmLabel={t(unsure ? "notice.source.leave" : "notice.source.confirm")}
      confirmDisabled={
        !canWork || (!unsure && (!kind || !note.trim() || (met && !date)))
      }
      pending={assessment.isPending}
      error={assessment.isError ? problemMessageOf(assessment.error, t) : null}
      onConfirm={() => {
        if (unsure) onClose();
        else if (kind)
          assessment.mutate({
            id: caseId,
            kind,
            obtainedAt: date ? startOfDayInZone(date, zone) : null,
            note,
            timezone: zone,
            metInPerson: met,
          });
      }}
    >
      <SourceOnRecord caseId={caseId} />
      <ChoiceList
        legend={t("notice.source.question")}
        value={answer}
        onChange={setAnswer}
        choices={SOURCE_ANSWERS.map((value) => ({
          value,
          label: t(SOURCE_ANSWER_LABEL[value]),
          description:
            value === "unsure" ? t("notice.source.unsureHint") : undefined,
        }))}
      />
      {answer === "other" && (
        <ChoiceList
          legend={t("notice.source.otherQuestion")}
          value={other}
          onChange={setOther}
          choices={OTHER_KINDS.map((value) => ({
            value,
            label: acquisitionKindLabel(value, t),
          }))}
        />
      )}
      {kind && (
        <>
          <Field
            label={t("noticeDuty.obtainedOn")}
            hint={t("notice.source.dateHint")}
            required={met}
          >
            {(control) => (
              <DateInput
                {...control}
                value={date}
                onChange={(event) => setDate(event.target.value)}
              />
            )}
          </Field>
          <Field
            label={t("scheduling.timezone")}
            hint={t("notice.source.zoneHint")}
          >
            {(control) => (
              <TimezoneSelect
                {...control}
                name="notice-source-zone"
                value={zone}
                onChange={setZone}
              />
            )}
          </Field>
          <Field
            label={t("notice.source.evidence")}
            hint={t("notice.source.evidenceHint")}
            required
          >
            {(control) => (
              <Textarea
                {...control}
                value={note}
                maxLength={500}
                onChange={(event) => setNote(event.target.value)}
              />
            )}
          </Field>
          <p className="t-caption">
            {t(met ? "notice.source.metEffect" : "notice.source.effect")}
          </p>
        </>
      )}
    </ConfirmModal>
  );
}

// The reads a confirmed source changes: the queues, the brief and the contact.
const SOURCE_READS = [
  ["notice-cases"],
  ["brief"],
  ["worklist"],
  ["contact360"],
  ["contact-consent"],
] as const;

// Why the question is asked, and how the contact reached the record. The
// precise "no record of where" is said only when the acquisition says so.
function SourceOnRecord({ caseId }: Readonly<{ caseId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const review = useQuery({
    queryKey: ["notice-cases", "one", caseId],
    queryFn: async () =>
      unwrap(
        await api.GET("/privacy/notice-case-reviews/{id}", {
          params: { path: { id: caseId } },
        }),
      ),
  });
  const acquisition = review.data?.acquisition;
  if (!acquisition) return null;
  return (
    <>
      <p>
        {t(
          acquisition.kind === "unknown_legacy"
            ? "notice.source.whyUnknown"
            : "notice.source.whyUnconfirmed",
        )}
      </p>
      <FieldRow label={t("notice.source.onRecord")}>
        {acquisitionCaption(acquisition, t, locale, zone)}
      </FieldRow>
    </>
  );
}
