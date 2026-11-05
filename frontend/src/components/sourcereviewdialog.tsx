// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type UseQueryResult,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field, Textarea } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DateInput, type ISODate } from "../design-system/dateinput";
import { useSettledValue } from "../design-system/debouncedsearch";
import { ErrorLine } from "../design-system/errorline";
import { FieldRow } from "../design-system/fieldgrid";
import { TimezoneSelect } from "../design-system/timezoneselect";
import { acquisitionKindLabel } from "../format/acquisitionkind";
import { formatDate } from "../format/format";
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
import {
  type SourceEvidenceRef,
  SourceReviewEvidence,
} from "./sourcereviewevidence";

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
type SourceReviewProps = Readonly<{
  caseId: string;
  open: boolean;
  onClose: () => void;
}>;

export function SourceReviewDialog(props: SourceReviewProps) {
  return <SourceReviewForm key={props.caseId} {...props} />;
}

function SourceReviewForm({ caseId, open, onClose }: SourceReviewProps) {
  const t = useT();
  const canWork = useCanChangeNoticeCase();
  const queryClient = useQueryClient();
  const [answer, setAnswer] = useState<SourceAnswer | "">("");
  const [other, setOther] = useState<SourceKind | "">("");
  const [date, setDate] = useState<ISODate | "">("");
  const readerZone = useViewerZone();
  const [zone, setZone] = useState(readerZone);
  const [note, setNote] = useState("");
  const [selected, setSelected] = useState<SourceEvidenceRef | null>(null);
  const settledNote = useSettledValue(note);
  const kind = sourceKindFor(answer, other);
  const unsure = answer === "unsure";
  const met = answer === "met";
  const review = useQuery({
    queryKey: ["notice-cases", "one", caseId],
    enabled: open,
    queryFn: async () =>
      unwrap(
        await api.GET("/privacy/notice-case-reviews/{id}", {
          params: { path: { id: caseId } },
        }),
      ),
  });
  const request = kind
    ? {
        kind,
        note: settledNote,
        obtained_at: date ? startOfDayInZone(date, zone) : null,
        timezone: zone,
        met_in_person: met,
        ...(selected ? { selected_evidence: selected } : {}),
      }
    : null;
  const ready = canPreviewSource(request, canWork, note === settledNote);
  const preview = useQuery({
    queryKey: ["notice-source-preview", caseId, request],
    enabled: open && ready,
    retry: false,
    queryFn: async ({ signal }) =>
      request
        ? unwrap(
            await api.POST("/privacy/notice-case-reviews/{id}/source/preview", {
              params: { path: { id: caseId } },
              body: request,
              signal,
            }),
          )
        : null,
  });
  const previewReady =
    ready &&
    !preview.isFetching &&
    !preview.isError &&
    typeof preview.data?.preview_token === "string";
  const canConfirm = unsure ? canWork : previewReady;
  const assessment = useMutation({
    mutationFn: async (body: components["schemas"]["AssessNoticeSource"]) =>
      unwrap(
        await api.POST("/privacy/notice-case-reviews/{id}/source", {
          params: { path: { id: caseId } },
          body,
        }),
      ),
    onSuccess: () => {
      onClose();
      for (const key of SOURCE_READS) {
        void queryClient.invalidateQueries({ queryKey: key });
      }
    },
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["notice-source-preview", caseId],
      });
      void queryClient.invalidateQueries({
        queryKey: ["notice-source-evidence", caseId],
      });
    },
  });
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      intent="drawer"
      title={t("notice.source.title")}
      confirmLabel={t(unsure ? "notice.source.leave" : "notice.source.confirm")}
      confirmDisabled={!canConfirm}
      pending={assessment.isPending}
      error={assessment.isError ? problemMessageOf(assessment.error, t) : null}
      onConfirm={() => {
        if (unsure) onClose();
        else if (request && previewReady && preview.data)
          assessment.mutate({
            ...request,
            preview_token: preview.data.preview_token,
          });
      }}
    >
      <SourceOnRecord acquisition={review.data?.acquisition} />
      <SourceReviewEvidence
        caseId={caseId}
        originalKind={review.data?.acquisition?.kind}
        selected={selected}
        onSelect={setSelected}
      />
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
            hint={t(
              selected
                ? "notice.source.selectedHint"
                : "notice.source.evidenceHint",
            )}
            required={selected === null}
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
          <SourceAssessmentPreview ready={ready} query={preview} />
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
  ["notice-source-evidence"],
  ["notice-source-preview"],
] as const;

// Why the question is asked, and how the contact reached the record. The
// precise "no record of where" is said only when the acquisition says so.
function canPreviewSource(
  request: components["schemas"]["AssessNoticeSource"] | null,
  canWork: boolean,
  settled: boolean,
): boolean {
  return (
    canWork &&
    settled &&
    request !== null &&
    (!request.met_in_person || Boolean(request.obtained_at)) &&
    (request.note.trim() !== "" || request.selected_evidence !== undefined)
  );
}

function SourceOnRecord({
  acquisition,
}: Readonly<{
  acquisition: components["schemas"]["NoticeAcquisition"] | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
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

function SourcePreviewResult({
  preview,
}: Readonly<{ preview: components["schemas"]["NoticeSourcePreview"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const sentence = !preview.notice_required
    ? t("notice.source.previewNone")
    : preview.due_at
      ? t(
          preview.deadline_is_upper_bound
            ? "notice.source.previewLatest"
            : "notice.source.previewOwed",
          { date: formatDate(preview.due_at, locale, preview.timezone) },
        )
      : t("notice.source.noDeadline");
  return <FieldRow label={t("notice.source.preview")}>{sentence}</FieldRow>;
}

function SourceAssessmentPreview({
  ready,
  query,
}: Readonly<{
  ready: boolean;
  query: UseQueryResult<components["schemas"]["NoticeSourcePreview"] | null>;
}>) {
  const t = useT();
  if (!ready) return null;
  if (query.isFetching) return <p role="status">{t("common.loading")}</p>;
  if (query.isError)
    return (
      <>
        <ErrorLine error={query.error} />
        <Button variant="link" onClick={() => query.refetch()}>
          {t("common.retry")}
        </Button>
      </>
    );
  return query.data && typeof query.data.preview_token === "string" ? (
    <SourcePreviewResult preview={query.data} />
  ) : null;
}
