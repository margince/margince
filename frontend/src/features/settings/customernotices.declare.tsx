// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Textarea } from "../../design-system/field";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { dueInstant, todayIn } from "../../format/calendarday";
import { isoOrBlank } from "../../format/datepattern";
import { useLocale, useT } from "../../i18n";
import { useLegalEntities } from "../../lib/legalentities.queries";
import { blankRequired, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { NOTICES_KEY, useMarketingPurposes } from "./customernotices.queries";

// Declaring that existing customers were told about marketing mail when their
// address was collected.

type Draft = Readonly<{
  entity: string;
  group: string;
  channel: string;
  text: string;
  from: ISODate | "";
  until: ISODate | "";
  proof: string;
  proofDay: ISODate | "";
  purposes: string[];
}>;

// A picked day starts at midnight in the reader's own zone. The end day counts
// on screen but not on the wire, so it is sent as the next day's first second.
function startOf(day: string, zone: string): string {
  return dueInstant(day, zone, "00:00");
}
function afterEndOf(day: string, zone: string): string {
  return new Date(
    new Date(dueInstant(day, zone)).getTime() + 1000,
  ).toISOString();
}

export function DeclareForm({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults();
  const zone = useViewerZone();
  const queryClient = useQueryClient();
  const formId = useId();
  const entities = useLegalEntities();
  const purposes = useMarketingPurposes();
  const live = (entities.data ?? []).filter((e) => !e.archived_at);
  const fallbackEntity = live.find((e) => e.is_default)?.id ?? "";
  const [draft, setDraft] = useState<Draft>({
    entity: "",
    group: "",
    channel: "",
    text: "",
    from: "",
    until: "",
    proof: "",
    proofDay: "",
    purposes: [],
  });
  const entity = draft.entity || fallbackEntity;
  const declare = useMutation({
    mutationFn: async (d: Draft) =>
      unwrap(
        await api.POST("/existing-customer-notices", {
          body: {
            legal_entity_id: entity,
            customer_group: d.group.trim(),
            collection_channel: d.channel.trim(),
            notice_text: d.text.trim(),
            effective_from: startOf(d.from, zone),
            effective_until: d.until ? afterEndOf(d.until, zone) : null,
            evidence_reference: d.proof.trim() || null,
            evidence_dated_at:
              d.proof.trim() && d.proofDay ? startOf(d.proofDay, zone) : null,
            purpose_ids: d.purposes,
          },
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: NOTICES_KEY });
      onDone();
    },
  });
  function edit(next: Partial<Draft>) {
    setDraft((was) => ({ ...was, ...next }));
    for (const key of Object.keys(next)) faults.edited(key);
    if (declare.isError) declare.reset();
  }
  const labels = {
    entity: t("privacy.customerNoticeEntity"),
    group: t("privacy.customerNoticeGroup"),
    channel: t("privacy.customerNoticeChannel"),
    text: t("privacy.customerNoticeText"),
    from: t("privacy.customerNoticeFrom"),
    proof: t("privacy.customerNoticeProof"),
    proofDay: t("privacy.customerNoticeProofDay"),
    purposes: t("privacy.customerNoticePurposes"),
  };
  // Proof and its date go together: either one asks for the other.
  const proofGiven = draft.proof.trim() !== "" || draft.proofDay !== "";
  const blankFields = () =>
    blankRequired(locale, {
      ...(live.length > 1 ? { entity: [labels.entity, entity] as const } : {}),
      group: [labels.group, draft.group],
      channel: [labels.channel, draft.channel],
      text: [labels.text, draft.text],
      from: [labels.from, draft.from],
      ...(proofGiven
        ? {
            proof: [labels.proof, draft.proof] as const,
            proofDay: [labels.proofDay, draft.proofDay] as const,
          }
        : {}),
      purposes: [labels.purposes, draft.purposes.join(",")],
    });
  const marketing = purposes.data ?? [];
  return (
    <>
      <form
        id={formId}
        ref={faultsRoot}
        className="form-stack"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          if (faults.show(blankFields()) || declare.isPending) return;
          // No sending entity is known yet: the legal entities still load.
          if (entity !== "") declare.mutate(draft);
        }}
      >
        {live.length > 1 && (
          <Field label={labels.entity} required error={faults.of("entity")}>
            {(control) => (
              <Select
                id={control.id}
                aria-describedby={control["aria-describedby"]}
                aria-invalid={control["aria-invalid"]}
                value={entity}
                onChange={(value) => edit({ entity: value })}
                options={live.map((e) => ({
                  value: e.id,
                  label: e.display_name,
                }))}
              />
            )}
          </Field>
        )}
        <Field
          label={labels.group}
          hint={t("privacy.customerNoticeGroupHint")}
          required
          error={faults.of("group")}
        >
          {(control) => (
            <TextInput
              {...control}
              maxLength={500}
              value={draft.group}
              onChange={(event) => edit({ group: event.target.value })}
            />
          )}
        </Field>
        <Field
          label={labels.channel}
          hint={t("privacy.customerNoticeChannelHint")}
          required
          error={faults.of("channel")}
        >
          {(control) => (
            <TextInput
              {...control}
              maxLength={200}
              value={draft.channel}
              onChange={(event) => edit({ channel: event.target.value })}
            />
          )}
        </Field>
        <Field label={labels.text} required error={faults.of("text")}>
          {(control) => (
            <Textarea
              {...control}
              rows={4}
              maxLength={4000}
              value={draft.text}
              onChange={(event) => edit({ text: event.target.value })}
            />
          )}
        </Field>
        <Field label={labels.from} required error={faults.of("from")}>
          {(control) => (
            <DateInput
              {...control}
              value={draft.from}
              max={isoOrBlank(todayIn(zone)) || undefined}
              onChange={(event) => edit({ from: event.target.value })}
            />
          )}
        </Field>
        <Field
          label={t("privacy.customerNoticeUntil")}
          hint={t("privacy.customerNoticeUntilHint")}
        >
          {(control) => (
            <DateInput
              {...control}
              value={draft.until}
              min={draft.from || undefined}
              onChange={(event) => edit({ until: event.target.value })}
            />
          )}
        </Field>
        <Field
          label={labels.proof}
          hint={t("privacy.customerNoticeProofHint")}
          error={faults.of("proof")}
        >
          {(control) => (
            <TextInput
              {...control}
              maxLength={500}
              value={draft.proof}
              onChange={(event) => edit({ proof: event.target.value })}
            />
          )}
        </Field>
        <Field label={labels.proofDay} error={faults.of("proofDay")}>
          {(control) => (
            <DateInput
              {...control}
              value={draft.proofDay}
              max={isoOrBlank(todayIn(zone)) || undefined}
              onChange={(event) => edit({ proofDay: event.target.value })}
            />
          )}
        </Field>
        {marketing.length === 0 && purposes.isSuccess ? (
          <p className="t-caption">{t("privacy.customerNoticeNoPurposes")}</p>
        ) : (
          <ChoiceList
            multiple
            legend={labels.purposes}
            value={draft.purposes}
            onChange={(next) => edit({ purposes: next })}
            choices={marketing.map((p) => ({ value: p.id, label: p.label }))}
          />
        )}
        <ErrorLine>{faults.of("purposes")}</ErrorLine>
        <ErrorLine error={declare.error} />
      </form>
      <div className="actions">
        <Button
          type="submit"
          form={formId}
          variant="primary"
          pending={declare.isPending}
        >
          {t("privacy.customerNoticeSave")}
        </Button>
      </div>
    </>
  );
}
