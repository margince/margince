import {
  useIsMutating,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import type { EntityKind } from "../app/entity";
import { useDetailsFieldTarget } from "../app/pageaside";
import type { Route } from "../app/router";
import { useUnsavedGuard } from "../app/unsaved";
import { Button } from "../design-system/atoms";
import { FieldGrid, FieldRow } from "../design-system/fieldgrid";
import { InlineChoice } from "../design-system/inlinechoice";
import { InlineText } from "../design-system/inlinetext";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { derivedRecordKeys } from "./activitykeys";
import {
  ProblemError,
  problemCodeOf,
  problemExistingId,
  problemMessageOf,
} from "./common";
import {
  type CreateField,
  type FormRows,
  fieldLabel,
  RecordFormBody,
  usePublishedValues,
} from "./create";
import { prefillFromRecord, prefillRowsFromRecord } from "./edit.prefill";
import { sameEditValue } from "./independentedit";
import { groupValue, selectOptions } from "./recordfieldvalues";
import { invalidateRecord } from "./recordwritekeys";

export type FieldRecord = Record<string, unknown> & {
  id: string;
  version?: number;
};
// Conflict comparisons use the original API values before form projection.
export function rawRecord(opened: FieldRecord): FieldRecord {
  const original = opened.original;
  if (
    original &&
    typeof original === "object" &&
    "id" in original &&
    typeof original.id === "string"
  )
    return { ...original, id: original.id };
  return opened;
}
export type RecordFieldSave = (
  values: Record<string, string>,
  rows: FormRows,
  opened: FieldRecord,
) => Promise<unknown>;
export type FieldGroup = { label: string; keys: string[] };

type Props = {
  onValuesChange?: (values: Record<string, string>) => void;
  title: string;
  kind: EntityKind;
  record: FieldRecord;
  fields: CreateField[];
  groups?: FieldGroup[];
  canEdit: boolean;
  notice?: string;
  maskedFields?: readonly string[];
  renderValues?: Readonly<Record<string, ReactNode>>;
  links?: Readonly<Record<string, { href: string; label: string }>>;
  readOnlyFields?: Readonly<Record<string, string>>;
  save: RecordFieldSave;
  resolveExisting?: (code: string, id: string) => Route;
  // Rows the caller adds under the record's own fields, in the same grid:
  // facts that live on the record but are not fields of it, such as tags.
  extraRows?: ReactNode;
};

export function RecordFields(props: Readonly<Props>) {
  const t = useT();
  const groups = props.groups ?? [];
  const grouped = new Set(groups.flatMap((group) => group.keys));
  const sections = [
    ...props.fields
      .filter((field) => !field.divider && !grouped.has(field.key))
      .map((field) => ({ label: fieldLabel(field, t), fields: [field] })),
    ...groups
      .map((group) => ({
        label: group.label,
        fields: props.fields.filter((field) => group.keys.includes(field.key)),
      }))
      .filter((group) => group.fields.length > 0),
  ];
  return (
    <Panel title={props.title}>
      <PanelBody>
        {props.notice && <p>{props.notice}</p>}
        <FieldGrid>
          {sections.map((section) => (
            <RecordField
              key={`${props.record.id}:${section.fields[0].key}`}
              {...props}
              label={section.label}
              fields={section.fields}
            />
          ))}
          {props.extraRows}
        </FieldGrid>
      </PanelBody>
    </Panel>
  );
}

function RecordField({
  label,
  ...props
}: Readonly<
  Props & {
    label: string;
  }
>) {
  const t = useT();
  const qc = useQueryClient();
  const opened = useRef(props.record);
  const target = useDetailsFieldTarget(props.fields[0].key);
  const [editing, setEditing] = useState(false);
  const [dirty, setDirty] = useState(false);
  useUnsavedGuard(dirty, "details");
  const pending =
    useIsMutating({ mutationKey: [`${props.kind}-edit`, props.record.id] }) > 0;
  const onEditingChange = useCallback(
    (next: boolean) => {
      if (next && !editing) opened.current = props.record;
      setEditing(next);
      if (!next) setDirty(false);
    },
    [editing, props.record],
  );
  const mutation = useMutation({
    mutationKey: [`${props.kind}-edit`, props.record.id],
    mutationFn: async ({
      values,
      rows,
      baseline,
      save,
    }: {
      values: Record<string, string>;
      rows: FormRows;
      baseline: FieldRecord;
      save: RecordFieldSave;
    }) => {
      try {
        return await save(values, rows, baseline);
      } catch (error) {
        if (problemCodeOf(error) === "version_skew")
          throw new ProblemError({
            code: "version_skew",
            detail: t("edit.versionSkew"),
          });
        throw error;
      }
    },
    onSuccess: async () => {
      await Promise.all([
        invalidateRecord(qc, props.kind, props.record.id),
        qc.invalidateQueries({
          queryKey: [props.kind === "company" ? "companies" : `${props.kind}s`],
        }),
        ...derivedRecordKeys(props.kind, props.record.id).map((queryKey) =>
          qc.invalidateQueries({ queryKey }),
        ),
      ]);
    },
  });
  const field = props.fields[0];
  const values = prefillFromRecord(
    props.fields,
    editing ? opened.current : props.record,
  );
  const reason = props.fields
    .map((entry) => props.readOnlyFields?.[entry.key])
    .find(Boolean);
  const canEdit = props.canEdit && !reason && (!pending || editing);
  const grouped =
    props.groups?.some((group) => group.keys.includes(field.key)) ||
    Boolean(field.searchTargets) ||
    props.fields.length > 1 ||
    field.type === "repeatable" ||
    field.type === "multiselect";
  const saveValue = async (next: string) => {
    if (field.required && !next.trim())
      throw new ProblemError({ detail: t("record.fieldRequired") });
    const refusal = field.validate?.(next);
    if (refusal) throw new ProblemError({ detail: refusal });
    await mutation.mutateAsync({
      values: { [field.key]: next },
      rows: {},
      baseline: opened.current,
      save: props.save,
    });
  };
  if (props.maskedFields?.includes(field.key))
    return <FieldRow label={label}>{t("record.notShown")}</FieldRow>;
  const renderedValue = props.renderValues?.[field.key];
  if (grouped)
    return (
      <FieldRow valueRef={target} label={label} align="top" stacked={editing}>
        {!editing && renderedValue}
        {editing ? (
          <RecordFieldForm
            {...props}
            opened={opened.current}
            pending={mutation.isPending}
            error={mutation.error}
            onDirtyChange={setDirty}
            onClose={() => onEditingChange(false)}
            onSubmit={(submitted, rows) =>
              mutation.mutate(
                {
                  values: submitted,
                  rows: rows ?? {},
                  baseline: opened.current,
                  save: props.save,
                },
                { onSuccess: () => onEditingChange(false) },
              )
            }
          />
        ) : (
          <GroupReading
            props={props}
            canEdit={canEdit}
            reason={reason}
            label={label}
            onEdit={() => onEditingChange(true)}
          />
        )}
      </FieldRow>
    );
  return (
    <RecordScalarField
      field={field}
      valueRef={target}
      link={props.links?.[field.key]}
      label={label}
      values={values}
      canEdit={canEdit}
      reason={reason}
      editing={editing}
      onEditingChange={onEditingChange}
      onSave={saveValue}
      onDirtyChange={setDirty}
    />
  );
}

function GroupReading({
  props,
  canEdit,
  reason,
  label,
  onEdit,
}: Readonly<{
  props: Props;
  canEdit: boolean;
  reason?: string;
  label: string;
  onEdit: () => void;
}>) {
  const t = useT();
  const rendered = props.renderValues?.[props.fields[0].key];
  const value = groupValue(
    props.fields,
    props.record,
    t,
    props.maskedFields,
    props.readOnlyFields,
  );
  if (!canEdit)
    return rendered ? null : (
      <span title={reason}>{value || t("field.unset")}</span>
    );
  const change = t("inlineChoice.change", { field: label });
  return (
    <Button
      variant="link"
      className="inline-editable"
      data-empty={!value}
      onClick={onEdit}
      aria-label={change}
    >
      {rendered ? change : value || t("field.unset")}
    </Button>
  );
}

function RecordScalarField({
  field,
  valueRef,
  link,
  label,
  values,
  canEdit,
  reason,
  editing,
  onEditingChange,
  onDirtyChange,
  onSave,
}: Readonly<{
  field: CreateField;
  valueRef: ReturnType<typeof useDetailsFieldTarget>;
  link?: { href: string; label: string };
  label: string;
  values: Record<string, string>;
  canEdit: boolean;
  reason?: string;
  editing: boolean;
  onEditingChange: (editing: boolean) => void;
  onDirtyChange: (dirty: boolean) => void;
  onSave: (value: string) => Promise<void>;
}>) {
  const t = useT();
  return (
    <FieldRow valueRef={valueRef} label={label}>
      {field.type === "select" ? (
        <InlineChoice
          label={label}
          hideLabel
          value={values[field.key] ?? ""}
          options={selectOptions(field, values, t)}
          render={(value) =>
            selectOptions(field, values, t).find(
              (option) => option.value === value,
            )?.label ?? value
          }
          canEdit={canEdit}
          readOnlyReason={reason}
          onEditingChange={onEditingChange}
          onDirtyChange={onDirtyChange}
          onSave={onSave}
        />
      ) : (
        <InlineText
          label={label}
          value={values[field.key] ?? ""}
          placeholder={t("field.unset")}
          multiline={field.type === "textarea"}
          type={
            field.type === "number" ||
            field.type === "date" ||
            field.type === "email"
              ? field.type
              : "text"
          }
          maxLength={field.maxLength}
          step={field.step ?? (field.type === "number" ? "any" : undefined)}
          canEdit={canEdit}
          readOnlyReason={reason}
          onEditingChange={onEditingChange}
          onDirtyChange={onDirtyChange}
          onSave={onSave}
        />
      )}
      {link && !editing && (
        <OffsiteLink href={link.href}>{link.label}</OffsiteLink>
      )}
    </FieldRow>
  );
}

function RecordFieldForm({
  opened,
  pending,
  error,
  onClose,
  onDirtyChange,
  onSubmit,
  ...props
}: Readonly<
  Props & {
    opened: FieldRecord;
    pending: boolean;
    error: unknown;
    onClose: () => void;
    onDirtyChange: (dirty: boolean) => void;
    onSubmit: (values: Record<string, string>, rows?: FormRows) => void;
  }
>) {
  const t = useT();
  const [values, setValues] = useState(() =>
    prefillFromRecord(props.fields, opened),
  );
  const [rows, setRows] = useState(() =>
    prefillRowsFromRecord(props.fields, opened),
  );
  const dirty =
    pending ||
    !sameEditValue(values, prefillFromRecord(props.fields, opened)) ||
    !sameEditValue(rows, prefillRowsFromRecord(props.fields, opened));
  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);
  usePublishedValues(values, props.onValuesChange);
  const publish = props.onValuesChange;
  useEffect(() => () => publish?.({}), [publish]);
  return (
    <div inert={pending} className="form-stack">
      <RecordFormBody
        fields={props.fields}
        values={values}
        setValues={setValues}
        rows={rows}
        setRows={setRows}
        pending={pending}
        error={error ? problemMessageOf(error, t) : null}
        existing={
          error instanceof ProblemError
            ? problemExistingId(error.problem)
            : null
        }
        resolveExisting={props.resolveExisting}
        onSubmit={(submitted, submittedRows) => {
          const original = prefillFromRecord(props.fields, opened);
          const originalRows = prefillRowsFromRecord(props.fields, opened);
          if (
            sameEditValue(submitted, original) &&
            sameEditValue(submittedRows, originalRows)
          )
            onClose();
          else onSubmit(submitted, submittedRows);
        }}
        onClose={onClose}
        intent="save"
      />
    </div>
  );
}
