// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Button, Field, Textarea } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { Panel, PanelBody } from "../../design-system/panel";
import { Select } from "../../design-system/select";
import { TimezoneSelect } from "../../design-system/timezoneselect";
import { acquisitionKindLabel } from "../../format/acquisitionkind";
import { startOfDayInZone, viewerZone } from "../../format/timezone";
import { useT } from "../../i18n";
import { ACQUISITION_KINDS } from "../../lib/acquisitionkinds";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import type { NoticeCase } from "./noticecases.logic";

type SourceKind = components["schemas"]["AssessNoticeSource"]["kind"];
const SOURCES = ACQUISITION_KINDS.filter(
  (kind): kind is SourceKind => kind !== "unknown_legacy",
);

export function NoticeSourceReview({ duty }: Readonly<{ duty: NoticeCase }>) {
  const t = useT();
  const canWork = useCanWrite("privacy_request", "update");
  const [open, setOpen] = useState(false);
  return (
    <Panel title={t("notice.source.title")}>
      <PanelBody>
        <p>{t("notice.source.explanation")}</p>
        {duty.contact_name && (
          <EntityRef
            kind="contact"
            id={duty.contact_id}
            name={duty.contact_name}
          />
        )}
        {canWork && (
          <Button onClick={() => setOpen(true)}>
            {t("notice.source.review")}
          </Button>
        )}
        {open && (
          <SourceReviewDialog
            key={duty.id}
            duty={duty}
            open={open}
            onClose={() => setOpen(false)}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

export function SourceReviewDialog({
  duty,
  open,
  onClose,
}: Readonly<{
  duty: NoticeCase;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const canWork = useCanWrite("privacy_request", "update");
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<SourceKind | "">("");
  const [date, setDate] = useState<ISODate | "">("");
  const [zone, setZone] = useState(viewerZone);
  const [note, setNote] = useState("");
  const assessment = useMutation({
    mutationFn: async (vars: {
      id: string;
      kind: SourceKind;
      obtainedAt: string | null;
      note: string;
      timezone: string;
    }) =>
      unwrap(
        await api.POST("/privacy/notice-case-reviews/{id}/source", {
          params: { path: { id: vars.id } },
          body: {
            kind: vars.kind,
            note: vars.note,
            obtained_at: vars.obtainedAt,
            timezone: vars.timezone,
          },
        }),
      ),
    onSuccess: () => {
      onClose();
      for (const key of [
        ["notice-cases"],
        ["brief"],
        ["worklist"],
        ["contact360"],
        ["contact-consent"],
      ]) {
        void queryClient.invalidateQueries({ queryKey: key });
      }
    },
  });
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      intent="form"
      title={t("notice.source.title")}
      confirmLabel={t("notice.source.confirm")}
      confirmDisabled={!canWork || !kind || !note.trim()}
      pending={assessment.isPending}
      error={assessment.isError ? problemMessageOf(assessment.error, t) : null}
      onConfirm={() => {
        if (kind)
          assessment.mutate({
            id: duty.id,
            kind,
            obtainedAt: date ? startOfDayInZone(date, zone) : null,
            note,
            timezone: zone,
          });
      }}
    >
      <Field label={t("noticeDuty.obtainedAs")} required>
        {(control) => (
          <Select
            {...control}
            name="notice-source-kind"
            value={kind}
            options={SOURCES.map((value) => ({
              value,
              label: acquisitionKindLabel(value, t),
            }))}
            onChange={(value) => {
              const source = SOURCES.find((candidate) => candidate === value);
              if (source) setKind(source);
            }}
          />
        )}
      </Field>
      <Field
        label={t("noticeDuty.obtainedOn")}
        hint={t("notice.source.dateHint")}
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
      <Field label={t("notice.source.evidence")} required>
        {(control) => (
          <Textarea
            {...control}
            value={note}
            maxLength={500}
            onChange={(event) => setNote(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
