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
import { Button } from "../design-system/atoms";
import { FieldGrid, FieldRow } from "../design-system/fieldgrid";
import { InlineChoice } from "../design-system/inlinechoice";
import { InlineEditVerb, InlineText } from "../design-system/inlinetext";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody } from "../design-system/panel";
import { useLocale, useT } from "../i18n";
import { derivedRecordKeys } from "../lib/activitykeys";
import type { EntityKind } from "../lib/entity";
import { sameEditValue } from "../lib/independentedit";
import {
  ProblemError,
  problemExistingId,
  problemMessageOf,
} from "../lib/problem";
import { invalidateRecord } from "../lib/recordwritekeys";
import type { Route } from "../lib/router";
import {
  type CreateField,
  type FormRows,
  fieldLabel,
  RecordFormBody,
  usePublishedValues,
} from "./create";
import { prefillFromRecord, prefillRowsFromRecord } from "./edit.prefill";
import { useDetailsFieldTarget } from "./pageaside";
import type {
  FieldGroup,
  FieldRecord,
  RecordFieldSave,
} from "./recordfieldtypes";
import { rawRecord } from "./recordfieldtypes";
import { dateDisplay, groupValue, selectOptions } from "./recordfieldvalues";
import { useUnsavedGuard } from "./unsaved";

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
  // On a scalar row the rendered value is a destination drawn beside the edit
  // verb, so it is never nested in the inline-edit button.
  renderValues?: Readonly<Record<string, ReactNode>>;
  // A value's resting reading where it differs from the raw value its editor
  // holds; a group row reads it only when the group is that one field.
  displayValues?: Readonly<Record<string, string>>;
  links?: Readonly<Record<string, { href: string; label: string }>>;
  // A provenance mark per field, drawn BESIDE a resting scalar value: the value
  // is the inline-edit button, and a mark nested in it would be a second one.
  marks?: Readonly<Record<string, ReactNode>>;
  readOnlyFields?: Readonly<Record<string, string>>;
  save: RecordFieldSave;
  resolveExisting?: (code: string, id: string) => Route;
  // See CreateAction: a refusal this screen answers with its own verbs.
  refusalNotice?: (
    error: unknown,
    resend: (extra: Record<string, unknown>) => void,
    pending: boolean,
  ) => ReactNode | null;
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
          {sections.map(({ label, fields }) => {
            const [first] = fields;
            return first ? (
              <RecordField
                key={`${props.record.id}:${first.key}`}
                {...props}
                label={label}
                field={first}
                fields={fields}
              />
            ) : null;
          })}
          {props.extraRows}
        </FieldGrid>
      </PanelBody>
    </Panel>
  );
}

function RecordField({
  label,
  field,
  ...props
}: Readonly<
  Props & {
    label: string;
    field: CreateField;
  }
>) {
  const t = useT();
  const qc = useQueryClient();
  const opened = useRef(props.record);
  const target = useDetailsFieldTarget(field.key);
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
      extra,
    }: {
      values: Record<string, string>;
      rows: FormRows;
      baseline: FieldRecord;
      save: RecordFieldSave;
      extra?: Record<string, unknown>;
    }) =>
      // An ordinary save carries no extra fields, only a refusal's resend does.
      extra
        ? save(values, rows, baseline, extra)
        : save(values, rows, baseline),
    onSuccess: async () => {
      await Promise.all([
        invalidateRecord(qc, props.kind, props.record.id, {
          anchor: rawRecord(props.record).is_anchor === true,
        }),
        qc.invalidateQueries({
          queryKey: [props.kind === "company" ? "companies" : `${props.kind}s`],
        }),
        ...derivedRecordKeys(props.kind, props.record.id).map((queryKey) =>
          qc.invalidateQueries({ queryKey }),
        ),
      ]);
    },
  });
  const values = prefillFromRecord(
    props.fields,
    editing ? opened.current : props.record,
  );
  const reason = props.fields
    .map((entry) => props.readOnlyFields?.[entry.key])
    .find(Boolean);
  // A save elsewhere on the record pauses editing without unmounting the verb.
  const editable = props.canEdit && !reason;
  const canEdit = editable && (!pending || editing);
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
            onResend={(extra) => {
              const last = mutation.variables;
              if (last)
                mutation.mutate(
                  { ...last, extra },
                  { onSuccess: () => onEditingChange(false) },
                );
            }}
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
            field={field}
            editable={editable}
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
      rendered={renderedValue}
      display={props.displayValues?.[field.key]}
      mark={props.marks?.[field.key]}
      label={label}
      values={values}
      editable={editable}
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
  field,
  editable,
  canEdit,
  reason,
  label,
  onEdit,
}: Readonly<{
  props: Props;
  field: CreateField;
  editable: boolean;
  canEdit: boolean;
  reason?: string;
  label: string;
  onEdit: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { key, editLabel } = field;
  const rendered = props.renderValues?.[key];
  if (rendered)
    return editable ? (
      <InlineEditVerb
        label={label}
        text={editLabel && t(editLabel)}
        disabled={!canEdit}
        onClick={onEdit}
      />
    ) : null;
  const value =
    (props.fields.length === 1 ? props.displayValues?.[key] : undefined) ??
    groupValue(
      props.fields,
      props.record,
      t,
      locale,
      props.maskedFields,
      props.readOnlyFields,
    );
  if (!canEdit) return <span title={reason}>{value || t("field.unset")}</span>;
  return (
    <Button
      variant="link"
      className="inline-editable"
      data-empty={!value}
      onClick={onEdit}
      aria-label={
        editLabel ? t(editLabel) : t("inlineChoice.change", { field: label })
      }
    >
      {value || t("field.unset")}
    </Button>
  );
}

function RecordScalarField({
  field,
  valueRef,
  link,
  rendered,
  display,
  mark,
  label,
  values,
  editable,
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
  rendered?: ReactNode;
  display?: string;
  mark?: ReactNode;
  label: string;
  values: Record<string, string>;
  editable: boolean;
  canEdit: boolean;
  reason?: string;
  editing: boolean;
  onEditingChange: (editing: boolean) => void;
  onDirtyChange: (dirty: boolean) => void;
  onSave: (value: string) => Promise<void>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const stored = values[field.key] ?? "";
  const shown = display ?? dateDisplay(field, stored, locale);
  return (
    <FieldRow valueRef={valueRef} label={label}>
      {field.type === "select" ? (
        <InlineChoice
          label={label}
          hideLabel
          value={values[field.key] ?? ""}
          options={selectOptions(field, values, t)}
          render={(value) =>
            display && value === values[field.key]
              ? display
              : (selectOptions(field, values, t).find(
                  (option) => option.value === value,
                )?.label ?? value)
          }
          canEdit={canEdit}
          readOnlyReason={reason}
          onEditingChange={onEditingChange}
          onDirtyChange={onDirtyChange}
          onSave={onSave}
        />
      ) : (
        <>
          {!editing && rendered}
          {(editable || !rendered) && (
            <InlineText
              label={label}
              value={stored}
              display={shown}
              verb={Boolean(rendered)}
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
        </>
      )}
      {link && !editing && (
        <OffsiteLink href={link.href}>{link.label}</OffsiteLink>
      )}
      {!editing && mark}
    </FieldRow>
  );
}

function RecordFieldForm({
  opened,
  pending,
  error,
  onResend,
  onClose,
  onDirtyChange,
  onSubmit,
  ...props
}: Readonly<
  Props & {
    opened: FieldRecord;
    pending: boolean;
    error: unknown;
    onResend: (extra: Record<string, unknown>) => void;
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
        notice={error ? props.refusalNotice?.(error, onResend, pending) : null}
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
