import { type ReactNode, useRef } from "react";
import { useAssignableUserOptions } from "../../components/assigneepicker";
import {
  RosterFailedLine,
  useRosterPartial,
  useRosterPartialHint,
} from "../../components/entityref";
import {
  type AssignedRecord,
  assignedRecord,
} from "../../components/recordassignments.queries";
import { ActionRow } from "../../design-system/actionrow";
import {
  Button,
  Field,
  type FieldControl,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { focusFirstRefused } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { isRealCalendarDay } from "../../format/calendarday";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { type EditableField, humanizeKind } from "./approvalkind";
import "./approvalrow.css";
import "../../design-system/querystates.css";

// The two slots an ApprovalRow hands to `DecisionCard`: the "view everything"
// dialog behind its meta line, and the inline staged-draft editor. They sit
// apart from the row because neither is about deciding — one reads the whole
// proposal, the other rewrites the part of it this installation lets a contact
// change — and the row is already the longest thing on the surface.

/** Outcomes that keep the row mounted. Already-decided is not one: its refetch
 *  unmounts the row, so the screen says it. */
export function DecideOutcome({
  decide,
  skewKey,
  alreadyDecided,
  onReRead,
  shown = true,
}: Readonly<{
  /** False while a dialog over the card draws the same outcome itself. */
  shown?: boolean;
  decide: { isError: boolean; error: unknown };
  skewKey: MessageKey | null;
  alreadyDecided: boolean;
  onReRead: () => void;
}>) {
  const t = useT();
  const generic = decide.isError && !skewKey && !alreadyDecided;
  if (!shown) return null;
  return (
    <>
      {generic && <ErrorLine error={decide.error} />}
      {skewKey && (
        <ErrorLine
          actions={<Button onClick={onReRead}>{t("decision.reRead")}</Button>}
        >
          {t(skewKey)}
        </ErrorLine>
      )}
    </>
  );
}

function isBlank(value: string | undefined): boolean {
  return (value ?? "").trim() === "";
}

/**
 * What the editor should START with for one field.
 *
 * This is the seed AND the display rule, deliberately in one function: a date
 * control silently discards a value it cannot parse, so if the two disagreed
 * the reader would see an empty box while the unparseable original still rode
 * out on approve — an editor showing one thing and submitting another.
 *
 * "Cannot parse" covers more than a wrong shape. `2026-02-30` is spelled
 * correctly and is not a day, and the element blanks it exactly as it blanks
 * `27/09/2026` — so the seed asks whether the date EXISTS, through
 * isRealCalendarDay, rather than only whether it looks like one.
 *
 * A non-string payload value seeds empty rather than being stringified. The
 * editor only ever offers string fields, so a number or object arriving here is
 * a payload that does not match its kind's declaration, and inviting somebody
 * to edit `[object Object]` is worse than an empty box.
 */
export function editableSeed(entry: EditableField, staged: unknown): string {
  if (typeof staged !== "string") {
    return "";
  }
  return entry.as === "date" && !isRealCalendarDay(staged) ? "" : staged;
}

function useStagedParts({
  disabled,
  required = [],
  acceptLabel,
  fields,
  draft,
  onChange,
  pending,
  onApprove,
  onCancel,
  records,
}: StagedEditorProps) {
  const t = useT();
  const blank = required.some((field) => isBlank(draft[field]));
  const root = useRef<HTMLDivElement>(null);
  const fieldControls = (
    <>
      {fields.map((entry) =>
        entry.as === "doer" ? (
          <DoerField
            key={entry.field}
            records={records}
            label={entry.label ? t(entry.label) : entry.field}
            value={draft[entry.field] ?? ""}
            onChange={(value) => onChange(entry.field, value)}
          />
        ) : (
          <Field
            key={entry.field}
            label={entry.label ? t(entry.label) : entry.field}
            required={required.includes(entry.field)}
            error={
              required.includes(entry.field) && isBlank(draft[entry.field])
                ? t("decision.fieldRequired")
                : undefined
            }
          >
            {(control) => (
              <EditorControl
                entry={entry}
                control={control}
                value={draft[entry.field] ?? ""}
                onChange={(value) => onChange(entry.field, value)}
              />
            )}
          </Field>
        ),
      )}
    </>
  );
  const approve = (
    <Button
      variant="primary"
      pending={pending}
      disabled={disabled}
      onClick={(event) => {
        // Each blank required field already says so; the press goes there.
        if (blank) {
          focusFirstRefused(root.current);
        } else if (!refuseUnreadableField(event.currentTarget)) {
          onApprove();
        }
      }}
    >
      {acceptLabel ?? t("decision.approveEdited")}
    </Button>
  );
  const cancel = (
    <Button disabled={pending} onClick={onCancel}>
      {t("deals.cancel")}
    </Button>
  );
  return { root, fieldControls, approve, cancel };
}

type StagedEditorProps = Readonly<{
  /** The records the proposed work is about; their named colleagues lead the doer picker. */
  records?: readonly AssignedRecord[];
  disabled?: boolean;
  /** Fields that may not be sent blank. */
  required?: readonly string[];
  acceptLabel?: string;
  fields: readonly EditableField[];
  draft: Record<string, string>;
  onChange: (field: string, value: string) => void;
  pending: boolean;
  onApprove: () => void;
  onCancel: () => void;
}>;

/**
 * The staged-draft editor: the string fields of the proposed_change, in
 * the shape the kind declared for them, going up as `edited_payload`.
 *
 * It is handed to the card as its `editor` slot rather than living inside the
 * primitive, and that boundary is the point: an edit re-enters the admission
 * gate from scratch on the server (re-tiered, re-RBAC'd, new diff_hash —
 * ADR-0036), so what it may offer is a question about THIS kind and this
 * contract, which is screen knowledge. The card knows how to draw a decision;
 * it does not know what this installation lets a contact change.
 */
export function StagedEditor(props: StagedEditorProps) {
  const { root, fieldControls, approve, cancel } = useStagedParts(props);
  return (
    <div className="form-stack approval-editor" data-date-scope ref={root}>
      {fieldControls}
      <ActionRow
        primary={
          /* The edited approve is the same write as the plain one and was the
             one path with no gate at all, so a second press sent a second
             verdict. */
          approve
        }
      >
        {/* Cancel keeps its word: leaving the editor without a verdict is not a
            verb any glyph says, and an X here would be read as Reject. */}
        {cancel}
      </ActionRow>
    </div>
  );
}

/**
 * The staged-draft editor in the app's one form dialog. The caller mounts it
 * only while editing, so each opening starts from the draft the caller seeds.
 * `outcome` is the decision's refusal, drawn inside because the dialog covers
 * the card that would otherwise show it.
 */
export function StagedEditDialog({
  unsaved,
  outcome,
  returnFocusTo,
  ...editor
}: StagedEditorProps &
  Readonly<{
    /** Typed text the dialog's Escape, X and backdrop must ask about. */
    unsaved: boolean;
    outcome?: ReactNode;
    returnFocusTo?: () => HTMLElement | null;
  }>) {
  const t = useT();
  const { root, fieldControls, approve, cancel } = useStagedParts(editor);
  return (
    <RecordFormDialog
      open
      title={t("trust.edit")}
      onClose={editor.onCancel}
      fieldCount={editor.fields.length}
      unsaved={unsaved}
      closeDisabled={editor.pending}
      returnFocusTo={returnFocusTo}
      form={
        <div className="form-stack approval-editor" data-date-scope ref={root}>
          {fieldControls}
          {outcome}
        </div>
      }
      actions={
        <>
          {cancel}
          {approve}
        </>
      }
    />
  );
}

/**
 * Who does the accepted work: the reader by default, or a colleague. Agent
 * seats are left out, and a failed or partial roster says so, as in every
 * picker that hands work to somebody (`TaskAssigneeField`).
 */
function DoerField({
  label,
  value,
  onChange,
  records,
}: Readonly<{
  records?: readonly AssignedRecord[];
  label: string;
  value: string;
  onChange: (next: string) => void;
}>) {
  const t = useT();
  const { options, roster } = useAssignableUserOptions(true, records);
  const partialHint = useRosterPartialHint(useRosterPartial("user", true));
  return (
    <>
      <Field label={label} hint={partialHint}>
        {(control) => (
          <Select
            {...control}
            options={[{ value: "", label: t("approval.doerMe") }, ...options]}
            value={value}
            onChange={onChange}
          />
        )}
      </Field>
      <RosterFailedLine kind="user" roster={roster} />
    </>
  );
}

/** One staged field's control, in the shape its kind declared. */
function EditorControl({
  entry,
  control,
  value,
  onChange,
}: Readonly<{
  entry: EditableField;
  control: FieldControl;
  value: string;
  onChange: (next: string) => void;
}>) {
  switch (entry.as) {
    case "choice":
      return (
        <ChoiceSelect
          entry={entry}
          control={control}
          value={value}
          onChange={onChange}
        />
      );
    case "date":
      // A staged date that is not a day shows as an empty box the reader can
      // fill, not as text the field would refuse.
      return (
        <DateInput
          {...control}
          value={isoOrBlank(value)}
          onChange={(event) => onChange(event.target.value)}
        />
      );
    case "textarea":
      return (
        <Textarea
          {...control}
          rows={12}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      );
    default:
      return (
        <TextInput
          {...control}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      );
  }
}

/**
 * A wire enum offered by its labels. The VALUE stays the enum, which is what
 * gets submitted; an option with no label degrades to its own words.
 */
function ChoiceSelect({
  entry,
  control,
  value,
  onChange,
}: Readonly<{
  entry: Extract<EditableField, { as: "choice" }>;
  control: FieldControl;
  value: string;
  onChange: (next: string) => void;
}>) {
  const t = useT();
  return (
    <Select
      {...control}
      options={entry.options.map((option) => {
        const label = entry.optionLabels?.[option];
        return {
          value: option,
          label: label ? t(label) : humanizeKind(option),
        };
      })}
      value={value}
      onChange={onChange}
    />
  );
}

/**
 * The records a promise or follow-up card is about: a promise's links, or a
 * follow-up's deal. Link entries carry the server's field names.
 */
export function cardRecords(
  change: Readonly<Record<string, unknown>> | undefined,
): AssignedRecord[] {
  const links = Array.isArray(change?.links) ? change.links : [];
  const linked = links.flatMap((link: unknown) =>
    typeof link === "object" &&
    link !== null &&
    "EntityType" in link &&
    "EntityID" in link &&
    typeof link.EntityType === "string" &&
    typeof link.EntityID === "string"
      ? assignedRecord(link.EntityType, link.EntityID)
      : [],
  );
  const deal =
    typeof change?.deal_id === "string"
      ? assignedRecord("deal", change.deal_id)
      : [];
  return [...linked, ...deal];
}
