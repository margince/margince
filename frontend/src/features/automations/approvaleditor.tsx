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
import { Select } from "../../design-system/select";
import { isRealCalendarDay } from "../../format/calendarday";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  EDITABLE_FIELDS,
  type EditableField,
  humanizeKind,
} from "./approvalkind";
import "./approvalrow.css";
import "../../design-system/querystates.css";

// The two slots an ApprovalRow hands to `DecisionCard`: the "view everything"
// dialog behind its meta line, and the inline staged-draft editor. They sit
// apart from the row because neither is about deciding — one reads the whole
// proposal, the other rewrites the part of it this installation lets a contact
// change — and the row is already the longest thing on the surface.

/**
 * What the inline editor offers for one proposal.
 *
 * The default is every string field as a text box, which is the right shape
 * for a payload whose values are prose. A kind that declares an EDITABLE_FIELDS
 * policy gets exactly the fields it named, in the shape it named them —
 * identifiers and enums are not prose, and offering them as free text asks a
 * reader to type their way into a refusal.
 */
export function editableStrings(
  kind: string,
  change: Record<string, unknown>,
): EditableField[] {
  if (kind === "commitment_task" && typeof change.existing_task_id === "string")
    return [];
  // Own-property only: `kind` is a wire string, and a value named `constructor`
  // would otherwise find a function on Object's prototype, pass the truthy
  // check, and crash the queue on .filter instead of falling back to the
  // generic editor.
  const declared = Object.hasOwn(EDITABLE_FIELDS, kind)
    ? EDITABLE_FIELDS[kind]
    : undefined;
  if (declared) {
    // A declared field the payload does not carry is skipped rather than
    // rendered empty: an editor offering a field that is not in the change
    // would ADD it on approve, and the server reads an added path as a
    // retargeted edit.
    return declared.filter((entry) => typeof change[entry.field] === "string");
  }
  return Object.entries(change)
    .filter((entry): entry is [string, string] => typeof entry[1] === "string")
    .map(([field]) => ({ field, as: "text" }) as const);
}

/** Outcomes that keep the row mounted. Already-decided is not one: its refetch
 *  unmounts the row, so `useDecisionSink` says it at screen level. */
export function DecideOutcome({
  decide,
  skewKey,
  alreadyDecided,
  onReRead,
}: Readonly<{
  decide: { isError: boolean; error: unknown };
  skewKey: MessageKey | null;
  alreadyDecided: boolean;
  onReRead: () => void;
}>) {
  const t = useT();
  const generic = decide.isError && !skewKey && !alreadyDecided;
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

/**
 * The inline staged-draft editor: the string fields of the proposed_change, in
 * the shape the kind declared for them, going up as `edited_payload`.
 *
 * It is handed to the card as its `editor` slot rather than living inside the
 * primitive, and that boundary is the point: an edit re-enters the admission
 * gate from scratch on the server (re-tiered, re-RBAC'd, new diff_hash —
 * ADR-0036), so what it may offer is a question about THIS kind and this
 * contract, which is screen knowledge. The card knows how to draw a decision;
 * it does not know what this installation lets a contact change.
 */
export function StagedEditor({
  disabled,
  acceptLabel,
  fields,
  draft,
  onChange,
  pending,
  onApprove,
  onCancel,
  records,
}: Readonly<{
  /** The records the proposed work is about; their named colleagues lead the doer picker. */
  records?: readonly AssignedRecord[];
  disabled?: boolean;
  acceptLabel?: string;
  fields: readonly EditableField[];
  draft: Record<string, string>;
  onChange: (field: string, value: string) => void;
  pending: boolean;
  onApprove: () => void;
  onCancel: () => void;
}>) {
  const t = useT();
  return (
    <div className="form-stack approval-editor" data-date-scope>
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
      <ActionRow
        primary={
          /* The edited approve is the same write as the plain one and was the
             one path with no gate at all, so a second press sent a second
             verdict. */
          <Button
            variant="primary"
            pending={pending}
            disabled={disabled}
            onClick={(event) => {
              if (!refuseUnreadableField(event.currentTarget)) {
                onApprove();
              }
            }}
          >
            {acceptLabel ?? t("decision.approveEdited")}
          </Button>
        }
      >
        {/* Cancel keeps its word: leaving the editor without a verdict is not a
            verb any glyph says, and an X here would be read as Reject. */}
        <Button disabled={pending} onClick={onCancel}>
          {t("deals.cancel")}
        </Button>
      </ActionRow>
    </div>
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
