// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { X } from "lucide-react";
import { useId } from "react";
import { ActionRow } from "../design-system/actionrow";
import { Button, Field } from "../design-system/atoms";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody, PanelGroupHead } from "../design-system/panel";
import { MultiSelect, Select } from "../design-system/select";
import { forReader } from "../format/collate";
import { ordinalNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  draftProblem,
  type FilterDraft,
  inOptionOrder,
  type MeasureDraft,
  nextRowId,
  type QuestionDraft,
  retarget,
} from "./analytics.questions.draft";
import {
  type AnalyticsEntity,
  analyticsFieldLabel,
  entityLabel,
  fieldReference,
  fieldsForFn,
  filterFields,
  fnLabel,
  fnsFor,
  type MeasureFn,
  opLabel,
  QUESTION_OPS,
  type QuestionOp,
  valueControlOp,
  valueShape,
} from "./analytics.questions.vocab";
import type { VocabularyField } from "./filterdata";
import { ValueControl } from "./filtervalue";
import "./analytics.questions.css";

type Translate = ReturnType<typeof useT>;

// The Filters builder's value control reads a field TYPE; a question's field
// has a shape and, for an id, the record type it points at.
function controlType(
  entity: AnalyticsEntity,
  field: string,
): VocabularyField["type"] {
  const shape = valueShape(entity, field);
  if (shape !== "text") {
    return shape;
  }
  return fieldReference(field) ? "id" : "text";
}

// A boolean has no empty reading on a two-way control, so it starts on the
// answer the control already shows rather than on a blank nobody can see.
function startingValue(entity: AnalyticsEntity, field: string) {
  return valueShape(entity, field) === "boolean" ? true : "";
}

function fieldOptions(t: Translate, fields: readonly string[]) {
  return fields.map((field) => ({
    value: field,
    label: analyticsFieldLabel(t, field),
  }));
}

/**
 * The question as the reader is composing it: a population, how to group it,
 * what to measure and how to narrow it. Every choice offered comes from this
 * seat's own schema, so a question it builds names nothing the engine would
 * refuse as unknown.
 */
export function QuestionBuilder({
  entities,
  draft,
  baseCurrency,
  onChange,
  onAsk,
  onSave,
  asking,
  saving,
}: Readonly<{
  entities: readonly AnalyticsEntity[];
  draft: QuestionDraft;
  baseCurrency: string | null;
  onChange: (next: QuestionDraft) => void;
  onAsk: () => void;
  onSave: () => void;
  asking: boolean;
  saving: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const entity = entities.find((candidate) => candidate.name === draft.entity);
  const problem = draftProblem(draft, baseCurrency);
  const reason = problem ? t(problem) : undefined;
  const reasonId = useId();
  const population = [...entities]
    .map((candidate) => ({
      value: candidate.name,
      label: entityLabel(t, candidate.name),
    }))
    .sort((a, b) => forReader(a.label, b.label, locale));
  return (
    <Panel title={t("analytics.q.builderTitle")}>
      <PanelBody>
        <div className="questions-grid">
          <Field label={t("analytics.q.population")}>
            {(control) => (
              <Select
                {...control}
                options={population}
                value={draft.entity}
                placeholder={t("analytics.q.choosePopulation")}
                onChange={(name) => {
                  const next = entities.find((c) => c.name === name);
                  if (next) {
                    onChange(retarget(draft, next));
                  }
                }}
              />
            )}
          </Field>
          <Field label={t("analytics.q.groupBy")}>
            {(control) => (
              <MultiSelect
                {...control}
                options={fieldOptions(t, entity?.group_by ?? [])}
                values={draft.groupBy}
                placeholder={t("analytics.q.noGrouping")}
                disabled={!entity}
                onChange={(picked) =>
                  onChange({
                    ...draft,
                    groupBy: entity ? inOptionOrder(picked, entity) : [],
                  })
                }
              />
            )}
          </Field>
        </div>
      </PanelBody>
      {entity && (
        <>
          <MeasureRows entity={entity} draft={draft} onChange={onChange} />
          <FilterRows entity={entity} draft={draft} onChange={onChange} />
        </>
      )}
      <PanelBody>
        <ActionRow
          primary={
            <Button
              variant="primary"
              reasonId={reason ? reasonId : undefined}
              pending={asking}
              onClick={onAsk}
            >
              {t("analytics.q.ask")}
            </Button>
          }
        >
          <Button
            reasonId={reason ? reasonId : undefined}
            pending={saving}
            onClick={onSave}
          >
            {t("analytics.q.save")}
          </Button>
        </ActionRow>
        {/* One sentence refuses both verbs: they wait on the same missing
            choice, and saying it twice reads as two problems. */}
        {reason && (
          <p className="t-caption" id={reasonId}>
            {reason}
          </p>
        )}
      </PanelBody>
    </Panel>
  );
}

function MeasureRows({
  entity,
  draft,
  onChange,
}: Readonly<{
  entity: AnalyticsEntity;
  draft: QuestionDraft;
  onChange: (next: QuestionDraft) => void;
}>) {
  const t = useT();
  const replace = (id: number, next: MeasureDraft) =>
    onChange({
      ...draft,
      measures: draft.measures.map((m) => (m.id === id ? next : m)),
    });
  return (
    <>
      <PanelGroupHead
        title={t("analytics.q.measures")}
        level="h3"
        action={
          <Button
            onClick={() =>
              onChange({
                ...draft,
                measures: [
                  ...draft.measures,
                  { id: nextRowId(draft), fn: "count", field: "" },
                ],
              })
            }
          >
            {t("analytics.q.addMeasure")}
          </Button>
        }
      />
      <PanelBody>
        <ol className="questions-rows">
          {draft.measures.map((measure, index) => (
            <li
              key={measure.id}
              className="questions-row"
              aria-label={t("analytics.q.measureN", {
                n: ordinalNumber(index + 1),
              })}
            >
              <Select
                className="questions-control"
                aria-label={t("analytics.q.calculationN", {
                  n: ordinalNumber(index + 1),
                })}
                options={fnsFor(entity).map((fn) => ({
                  value: fn,
                  label: fnLabel(t, fn),
                }))}
                value={measure.fn}
                onChange={(value) => {
                  const fn = fnsFor(entity).find((c) => c === value);
                  if (fn) {
                    replace(measure.id, keepField(entity, measure, fn));
                  }
                }}
              />
              {measure.fn !== "count" && (
                <Select
                  className="questions-control"
                  aria-label={t("analytics.q.measureFieldN", {
                    n: ordinalNumber(index + 1),
                  })}
                  options={fieldOptions(t, fieldsForFn(entity, measure.fn))}
                  value={measure.field}
                  placeholder={t("analytics.q.chooseField")}
                  onChange={(field) =>
                    replace(measure.id, { ...measure, field })
                  }
                />
              )}
              {draft.measures.length > 1 && (
                <span className="questions-remove">
                  <IconAction
                    label={t("analytics.q.removeMeasure", {
                      n: ordinalNumber(index + 1),
                    })}
                    icon={<X aria-hidden />}
                    onClick={() =>
                      onChange({
                        ...draft,
                        measures: draft.measures.filter(
                          (m) => m.id !== measure.id,
                        ),
                      })
                    }
                  />
                </span>
              )}
            </li>
          ))}
        </ol>
      </PanelBody>
    </>
  );
}

// A field survives a change of aggregate only if the new one accepts it; one
// that does not is cleared for the reader to choose, never swapped silently.
function keepField(
  entity: AnalyticsEntity,
  measure: MeasureDraft,
  fn: MeasureFn,
): MeasureDraft {
  const field = fieldsForFn(entity, fn).includes(measure.field)
    ? measure.field
    : "";
  return { ...measure, fn, field };
}

function FilterRows({
  entity,
  draft,
  onChange,
}: Readonly<{
  entity: AnalyticsEntity;
  draft: QuestionDraft;
  onChange: (next: QuestionDraft) => void;
}>) {
  const t = useT();
  const fields = filterFields(entity);
  const replace = (id: number, next: FilterDraft) =>
    onChange({
      ...draft,
      filters: draft.filters.map((f) => (f.id === id ? next : f)),
    });
  return (
    <>
      <PanelGroupHead
        title={t("analytics.q.filters")}
        level="h3"
        action={
          <Button
            onClick={() =>
              onChange({
                ...draft,
                filters: [
                  ...draft.filters,
                  { id: nextRowId(draft), field: "", op: "eq", value: "" },
                ],
              })
            }
          >
            {t("analytics.q.addFilter")}
          </Button>
        }
      />
      {draft.filters.length > 0 && (
        <PanelBody>
          <ol className="questions-rows">
            {draft.filters.map((filter, index) => (
              <FilterRow
                key={filter.id}
                entity={entity}
                fields={fields}
                filter={filter}
                position={index + 1}
                onChange={(next) => replace(filter.id, next)}
                onRemove={() =>
                  onChange({
                    ...draft,
                    filters: draft.filters.filter((f) => f.id !== filter.id),
                  })
                }
              />
            ))}
          </ol>
        </PanelBody>
      )}
    </>
  );
}

function FilterRow({
  entity,
  fields,
  filter,
  position,
  onChange,
  onRemove,
}: Readonly<{
  entity: AnalyticsEntity;
  fields: readonly string[];
  filter: FilterDraft;
  position: number;
  onChange: (next: FilterDraft) => void;
  onRemove: () => void;
}>) {
  const t = useT();
  const controlOp = valueControlOp(filter.op);
  const fieldName = filter.field
    ? analyticsFieldLabel(t, filter.field)
    : t("analytics.q.filterN", { n: ordinalNumber(position) });
  return (
    <li
      className="questions-row"
      aria-label={t("analytics.q.filterN", { n: ordinalNumber(position) })}
    >
      <Select
        className="questions-control"
        aria-label={t("analytics.q.filterFieldN", {
          n: ordinalNumber(position),
        })}
        options={fieldOptions(t, fields)}
        value={filter.field}
        placeholder={t("analytics.q.chooseField")}
        onChange={(field) =>
          onChange({ ...filter, field, value: startingValue(entity, field) })
        }
      />
      <Select
        className="questions-control"
        aria-label={t("analytics.q.filterOperatorN", {
          n: ordinalNumber(position),
        })}
        options={QUESTION_OPS.map((op) => ({
          value: op,
          label: opLabel(t, op),
        }))}
        value={filter.op}
        onChange={(value) => {
          const op = QUESTION_OPS.find((candidate) => candidate === value);
          if (op) {
            onChange({ ...filter, op: op satisfies QuestionOp });
          }
        }}
      />
      {controlOp && filter.field !== "" && (
        <div className="questions-control">
          <ValueControl
            type={controlType(entity, filter.field)}
            references={fieldReference(filter.field)}
            options={undefined}
            op={controlOp}
            value={filter.value}
            onChange={(value) => onChange({ ...filter, value })}
            label={t("analytics.q.filterValueN", {
              n: ordinalNumber(position),
            })}
          />
        </div>
      )}
      <span className="questions-remove">
        <IconAction
          label={t("filters.removeClause", { field: fieldName })}
          icon={<X aria-hidden />}
          onClick={onRemove}
        />
      </span>
    </li>
  );
}
