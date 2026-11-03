// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import type { components } from "../../api/schema";
import {
  Button,
  Checkbox,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { navigate } from "../../lib/router";
import { useViewerId } from "../../lib/session";
import { analyticsFieldLabel } from "./analytics.questions.vocab";
import { ReportComposition, useComposition } from "./reporting.composition";
import {
  blocksOf,
  hasAssessment,
  layoutEdits,
  layoutOf,
  MAX_CARDS,
} from "./reporting.layout";
import type { ReportingReport, ReportingSelection } from "./reporting.model";
import { unsaveable } from "./reporting.save";
import { selectionWithProposal } from "./reporting.setupapply";
import {
  type SetupProposal,
  SetupProposalStep,
} from "./reporting.setupproposal";

type Audience = ReportingReport["audience"];

/** A custom report as the builder holds it, before and after its preview. */
export type CustomReportDraft = Readonly<{
  name: string;
  description: string;
  audience: Audience;
  selection: ReportingSelection;
}>;

// The draft the preview page reads. One at a time: a preview is the next
// step of the builder that was just closed, never a list. It is held for the
// reader who wrote it, so a later sign-in in the same tab never reads it.
let held: Readonly<{ reader: string; draft: CustomReportDraft }> | undefined;

export function holdCustomReport(
  reader: string,
  draft: CustomReportDraft,
): void {
  held = { reader, draft };
}

export function heldCustomReport(
  reader: string | undefined,
): CustomReportDraft | undefined {
  return held && reader !== undefined && held.reader === reader
    ? held.draft
    : undefined;
}

/** The population the builder's question reads: live open deals. */
const QUESTION_ENTITY = "pipeline-current";
const QUESTION_GROUPS = ["owner_id", "company_id", "stage_id"] as const;
const QUESTION_ROWS = 50;

type Measure = "count" | "amount";

function questionOf(
  measure: Measure,
  group: (typeof QUESTION_GROUPS)[number],
): NonNullable<components["schemas"]["ReportingCard"]["question"]> {
  return {
    entity: QUESTION_ENTITY,
    group_by: [group],
    measures:
      measure === "count"
        ? [{ fn: "count" }]
        : [{ fn: "sum", field: "amount_base_minor" }],
    limit: QUESTION_ROWS,
  };
}

/**
 * The custom report builder, in one drawer. It holds the name and the
 * question the report answers, the reporting selection, the charts in their
 * order with the assessment, and a custom question. Preview opens the report
 * in Analytics before anything is saved.
 */
export function CustomReportDrawer({
  initial,
  draft,
  onClose,
}: Readonly<{
  // The selection a new report starts from.
  initial: ReportingSelection;
  // A draft coming back from its preview.
  draft?: CustomReportDraft;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const reader = useViewerId();
  const composition = useComposition(draft?.selection ?? initial);
  const [name, setName] = useState(draft?.name ?? "");
  const [description, setDescription] = useState(draft?.description ?? "");
  const [audience, setAudience] = useState<Audience>(
    draft?.audience ?? "private",
  );
  const [measure, setMeasure] = useState<Measure>("count");
  const [group, setGroup] =
    useState<(typeof QUESTION_GROUPS)[number]>("owner_id");
  const { selection, metrics, kept } = composition;
  const cards = kept.reduce(
    (count, section) => count + section.cards.length,
    0,
  );
  const full = cards >= MAX_CARDS;
  const incomplete =
    !name.trim() ||
    cards === 0 ||
    reader === undefined ||
    unsaveable(selection, metrics);
  // The proposal fills what the words named and leaves the rest as it
  // stands; a name the reader typed is theirs.
  const applyProposal = (proposal: SetupProposal) => {
    if (proposal.name && name.trim() === "") setName(proposal.name);
    const next = selectionWithProposal(selection, proposal);
    composition.setSelection(next);
    if (next.metrics !== selection.metrics) {
      composition.setMetrics(next.metrics);
      if (next.blocks !== selection.blocks)
        composition.setLayout(layoutOf({ ...next, layout: undefined }));
    }
  };
  const preview = () => {
    if (reader === undefined) return;
    holdCustomReport(reader, {
      name: name.trim(),
      description: description.trim(),
      audience,
      selection: {
        ...selection,
        metrics: [...metrics],
        layout: kept,
        blocks: blocksOf(kept),
      },
    });
    onClose();
    navigate({ screen: "analytics", id: "reports", id2: "preview" });
  };
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="modal-title">
          {draft ? draft.name : t("reporting.newCustomReport")}
        </Heading>
        <p className="t-caption">{t("reporting.customBuilderPurpose")}</p>
      </DrawerHead>
      <DrawerBody>
        <div className="form-stack">
          <Field label={t("reporting.name")} required>
            {(field) => (
              <TextInput
                {...field}
                value={name}
                onChange={(event) => setName(event.target.value)}
                maxLength={160}
              />
            )}
          </Field>
          <Field label={t("reporting.whatToUnderstand")}>
            {(field) => (
              <Textarea
                {...field}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                rows={3}
              />
            )}
          </Field>
          <Button
            variant="ghost"
            onClick={() => setDescription(t("reporting.exampleQuestion"))}
          >
            {t("reporting.useExample")}
          </Button>
          <SetupProposalStep
            site="custom_report_setup"
            description={description}
            onApply={applyProposal}
          />
          <Field label={t("reporting.audience")}>
            {(field) => (
              <Select
                {...field}
                value={audience}
                options={audienceOptions(selection, t)}
                onChange={(value) => {
                  if (
                    value === "private" ||
                    value === "team" ||
                    value === "workspace"
                  )
                    setAudience(value);
                }}
              />
            )}
          </Field>
          <Heading as="h3" size="small">
            {t("reporting.reportingSelection")}
          </Heading>
          <ReportComposition
            composition={composition}
            showDimensions
            onScopeChange={() => setAudience("private")}
          >
            <Checkbox
              label={t("reporting.includeAssessment")}
              disabled={full && !hasAssessment(kept)}
              checked={hasAssessment(kept)}
              onChange={(event) =>
                composition.setLayout(
                  event.target.checked
                    ? layoutEdits.addAssessment(kept)
                    : layoutEdits.removeAssessment(kept),
                )
              }
            />
          </ReportComposition>
          <Heading as="h3" size="small">
            {t("reporting.customQuestion")}
          </Heading>
          <p className="t-caption">{t("reporting.customQuestionHint")}</p>
          <Field label={t("reporting.measure")}>
            {(field) => (
              <Select
                {...field}
                value={measure}
                options={[
                  { value: "count", label: t("reporting.dealCount") },
                  { value: "amount", label: t("reporting.openValueBase") },
                ]}
                onChange={(value) => {
                  if (value === "count" || value === "amount")
                    setMeasure(value);
                }}
              />
            )}
          </Field>
          <Field label={t("reporting.groupBy")}>
            {(field) => (
              <Select
                {...field}
                value={group}
                options={QUESTION_GROUPS.map((name) => ({
                  value: name,
                  label: analyticsFieldLabel(t, name),
                }))}
                onChange={(value) => {
                  const chosen = QUESTION_GROUPS.find(
                    (candidate) => candidate === value,
                  );
                  if (chosen) setGroup(chosen);
                }}
              />
            )}
          </Field>
          <Button
            variant="ghost"
            disabled={full}
            onClick={() =>
              composition.setLayout(
                layoutEdits.addQuestion(kept, questionOf(measure, group)),
              )
            }
          >
            {t("reporting.addQuestionCard")}
          </Button>
        </div>
      </DrawerBody>
      <DrawerFoot>
        <Button variant="ghost" onClick={onClose}>
          {t("reporting.cancel")}
        </Button>
        <Button variant="primary" disabled={incomplete} onClick={preview}>
          {t("reporting.previewReport")}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}

// The audiences a selection's scope admits, as the save dialog offers them.
function audienceOptions(
  selection: ReportingSelection,
  t: ReturnType<typeof useT>,
): { value: Audience; label: string }[] {
  return [
    { value: "private", label: t("reporting.private") },
    ...(selection.scope.kind === "team"
      ? [{ value: "team" as const, label: t("reporting.team") }]
      : []),
    ...(selection.scope.kind === "workspace"
      ? [{ value: "workspace" as const, label: t("reporting.workspace") }]
      : []),
  ];
}
