import { type ReactNode, useCallback, useEffect, useState } from "react";
import { EditableValue } from "../design-system/editablevalue";
import { FieldGrid, FieldRow } from "../design-system/fieldgrid";
import { InlineChoice } from "../design-system/inlinechoice";
import { InlineText } from "../design-system/inlinetext";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody } from "../design-system/panel";
import { useLocale, useT } from "../i18n";
import type { EntityKind } from "../lib/entity";
import { sameEditValue } from "../lib/independentedit";
import {
  ProblemError,
  problemExistingId,
  problemMessageOf,
} from "../lib/problem";
import type { Route } from "../lib/router";
import { type CreateField, type FormRows, fieldLabel } from "./create";
import { EditRecordModal } from "./edit";
import { prefillFromRecord, prefillRowsFromRecord } from "./edit.prefill";
import { useDetailsFieldTarget } from "./pageaside";
import type {
  FieldGroup,
  FieldRecord,
  RecordFieldSave,
} from "./recordfieldtypes";
import { dateDisplay, groupValue, selectOptions } from "./recordfieldvalues";
import { checkFieldValue, useRecordFieldWrite } from "./recordfieldwrite";
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
  // A value drawn as a destination: it keeps its own action inside the edit
  // pill, whose padding and bubble open the editor.
  renderValues?: Readonly<Record<string, ReactNode>>;
  // A value's resting reading where it differs from the raw value its editor
  // holds; a group row reads it only when the group is that one field.
  displayValues?: Readonly<Record<string, string>>;
  links?: Readonly<Record<string, { href: string; label: string }>>;
  // A provenance mark per field, drawn beside a scalar value's edit pill: a
  // mark inside it would be a second control in one.
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
  const [opened, setOpened] = useState(props.record);
  const target = useDetailsFieldTarget(field.key);
  const [editing, setEditing] = useState(false);
  const [dirty, setDirty] = useState(false);
  useUnsavedGuard(dirty, "details");
  const { mutation, pending } = useRecordFieldWrite(props.kind, props.record);
  const onEditingChange = useCallback(
    (next: boolean) => {
      if (next && !editing) setOpened(props.record);
      setEditing(next);
      if (!next) setDirty(false);
    },
    [editing, props.record],
  );
  const grouped =
    props.groups?.some((group) => group.keys.includes(field.key)) ||
    Boolean(field.searchTargets) ||
    props.fields.length > 1 ||
    field.type === "repeatable" ||
    field.type === "multiselect";
  // Only the open group form speaks for the record's answers, and its close clears them.
  const publish = grouped && editing ? props.onValuesChange : undefined;
  useEffect(() => () => publish?.({}), [publish]);
  const values = prefillFromRecord(
    props.fields,
    editing ? opened : props.record,
  );
  const reason = props.fields
    .map((entry) => props.readOnlyFields?.[entry.key])
    .find(Boolean);
  // A save elsewhere on the record pauses this field, never its own open editor.
  const editable = props.canEdit && !reason;
  const canEdit = editable && (!pending || editing);
  const saveValue = async (next: string) => {
    checkFieldValue(field, next, t);
    await mutation.mutateAsync({
      values: { [field.key]: next },
      rows: {},
      baseline: opened,
      save: props.save,
    });
  };
  const saveGroup = (
    submitted: Record<string, string>,
    rows: FormRows | undefined,
    baseline: FieldRecord,
  ) => {
    const unchanged =
      sameEditValue(submitted, prefillFromRecord(props.fields, baseline)) &&
      sameEditValue(rows ?? {}, prefillRowsFromRecord(props.fields, baseline));
    if (unchanged) {
      onEditingChange(false);
      return;
    }
    mutation.mutate(
      { values: submitted, rows: rows ?? {}, baseline, save: props.save },
      { onSuccess: () => onEditingChange(false) },
    );
  };
  const resend = (extra: Record<string, unknown>) => {
    const last = mutation.variables;
    if (last)
      mutation.mutate(
        { ...last, extra },
        { onSuccess: () => onEditingChange(false) },
      );
  };
  if (props.maskedFields?.includes(field.key))
    return <FieldRow label={label}>{t("record.notShown")}</FieldRow>;
  const renderedValue = props.renderValues?.[field.key];
  const refusal = mutation.error;
  if (grouped)
    return (
      <FieldRow valueRef={target} label={label} align="top">
        <GroupReading
          props={props}
          field={field}
          editable={editable}
          canEdit={canEdit}
          reason={reason}
          label={label}
          onEdit={() => {
            mutation.reset();
            onEditingChange(true);
          }}
        />
        <EditRecordModal
          open={editing}
          onClose={() => onEditingChange(false)}
          title={label}
          fields={props.fields}
          record={props.record}
          pending={mutation.isPending}
          error={refusal ? problemMessageOf(refusal, t) : null}
          existing={
            refusal instanceof ProblemError
              ? problemExistingId(refusal.problem)
              : null
          }
          resolveExisting={props.resolveExisting}
          notice={
            refusal
              ? props.refusalNotice?.(refusal, resend, mutation.isPending)
              : null
          }
          onValuesChange={publish}
          onDirtyChange={setDirty}
          onSubmit={saveGroup}
        />
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
      canEdit={canEdit}
      reason={reason}
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
  const value = rendered
    ? null
    : ((props.fields.length === 1 ? props.displayValues?.[key] : undefined) ??
      groupValue(
        props.fields,
        props.record,
        t,
        locale,
        props.maskedFields,
        props.readOnlyFields,
      ));
  if (!editable || (!rendered && !canEdit))
    return rendered ?? <span title={reason}>{value || t("field.unset")}</span>;
  return (
    <EditableValue
      editLabel={
        editLabel ? t(editLabel) : t("inlineChoice.change", { field: label })
      }
      placeholder={t("field.unset")}
      link={Boolean(rendered)}
      editable={canEdit}
      onEdit={onEdit}
    >
      {rendered ?? value}
    </EditableValue>
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
  canEdit,
  reason,
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
  canEdit: boolean;
  reason?: string;
  onEditingChange: (editing: boolean) => void;
  onDirtyChange: (dirty: boolean) => void;
  onSave: (value: string) => Promise<void>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const stored = values[field.key] ?? "";
  const options = selectOptions(field, values, t);
  return (
    <FieldRow valueRef={valueRef} label={label}>
      {field.type === "select" ? (
        <InlineChoice
          label={label}
          hideLabel
          value={stored}
          options={options}
          render={(value) =>
            display && value === stored
              ? display
              : (options.find((option) => option.value === value)?.label ??
                value)
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
          value={stored}
          display={display ?? dateDisplay(field, stored, locale)}
          link={rendered}
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
      {link && <OffsiteLink href={link.href}>{link.label}</OffsiteLink>}
      {mark}
    </FieldRow>
  );
}
