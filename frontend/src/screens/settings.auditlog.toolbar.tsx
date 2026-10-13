import { useState } from "react";
import { api } from "../api/client";
import { isEntityKind } from "../app/entity";
import { Disclosure, Field, TextInput } from "../design-system/atoms";
import { DateInput, type ISODate, isISODate } from "../design-system/dateinput";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select, type SelectOption } from "../design-system/select";
import { useT } from "../i18n";
import { HUMAN_ACTOR_PREFIX } from "./audit";
import { throwProblem } from "./common";
import { useRosterNames } from "./roster";
import {
  AUDIT_ACTION_LABEL,
  AUDIT_ACTIONS,
  AUDIT_TYPE_LABEL,
  type AuditWhen,
} from "./settings.auditlog.vocab";

export type AuditLogFilters = Readonly<{
  // A typed principal (`human:<uuid>`, `agent:sdr`): the Who dial writes one,
  // and the Actor ID box shows and edits the same value.
  actor: string;
  entityType: string;
  entityId: string;
  batchId: string;
  action: string;
  when: AuditWhen;
  // The Custom range's two calendar days, read on the company clock.
  from: string;
  to: string;
}>;

// The unfiltered question the view opens on. One object, so the toolbar hands
// back a whole answer and the query key is that same answer.
export const UNFILTERED_AUDIT_LOG: AuditLogFilters = {
  actor: "",
  entityType: "",
  entityId: "",
  batchId: "",
  action: "",
  when: "any",
  from: "",
  to: "",
};

// A record the reader names rather than an id they paste: the trail's routable
// kinds, found through the same search the palette uses.
async function searchAuditRecords(q: string): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/search", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data
    .filter((hit) => isEntityKind(hit.type))
    .map((hit) => ({ id: hit.id, name: hit.title ?? hit.id }));
}

function useWhoOptions(meUserId: string | undefined): SelectOption[] {
  const t = useT();
  // Every member the directory names, departed and invited seats included:
  // the question is who acted, not who can be assigned work today. A failed
  // read still leaves Anyone and You, and the Actor ID box for everyone else.
  const roster = useRosterNames("user", true);
  const members = (roster.data ?? []).flatMap((entry) =>
    "display_name" in entry && entry.id !== meUserId
      ? [{ value: HUMAN_ACTOR_PREFIX + entry.id, label: entry.display_name }]
      : [],
  );
  return [
    { value: "", label: t("settings.auditLog.anyone") },
    ...(meUserId
      ? [{ value: HUMAN_ACTOR_PREFIX + meUserId, label: t("audit.you") }]
      : []),
    ...members,
  ];
}

function useDialOptions() {
  const t = useT();
  const when: SelectOption[] = [
    { value: "any", label: t("settings.auditLog.anyTime") },
    { value: "today", label: t("settings.auditLog.today") },
    { value: "last7", label: t("settings.auditLog.last7") },
    { value: "last30", label: t("settings.auditLog.last30") },
    { value: "custom", label: t("settings.auditLog.custom") },
  ];
  const action: SelectOption[] = [
    { value: "", label: t("settings.auditLog.anyAction") },
    ...AUDIT_ACTIONS.map((value) => ({
      value,
      label: t(AUDIT_ACTION_LABEL[value]),
    })),
  ];
  const type: SelectOption[] = [
    { value: "", label: t("settings.auditLog.anyType") },
    ...Object.entries(AUDIT_TYPE_LABEL).map(([value, key]) => ({
      value,
      label: t(key),
    })),
  ];
  return { when, action, type };
}

const WHEN_VALUES: readonly AuditWhen[] = [
  "any",
  "today",
  "last7",
  "last30",
  "custom",
];

function asWhen(value: string): AuditWhen {
  return WHEN_VALUES.find((when) => when === value) ?? "any";
}

// The rare exact-id searches: kept, and folded away so the everyday dials lead.
function AuditLogIdSearch({
  filters,
  onChange,
}: Readonly<{
  filters: AuditLogFilters;
  onChange: (next: AuditLogFilters) => void;
}>) {
  const t = useT();
  const fields = [
    { key: "entityId", label: t("settings.auditLog.recordId") },
    { key: "batchId", label: t("settings.auditLog.batchId") },
    { key: "actor", label: t("settings.auditLog.actorId") },
  ] as const;
  return (
    <Disclosure summary={t("settings.auditLog.byId")}>
      <div className="audit-log-grid">
        {fields.map((field) => (
          <Field key={field.key} label={field.label}>
            {(control) => (
              <TextInput
                {...control}
                value={filters[field.key]}
                onChange={(event) =>
                  onChange({ ...filters, [field.key]: event.target.value })
                }
              />
            )}
          </Field>
        ))}
      </div>
    </Disclosure>
  );
}

function dateValue(raw: string): ISODate | "" {
  return isISODate(raw) ? raw : "";
}

function AuditLogRange({
  filters,
  onChange,
}: Readonly<{
  filters: AuditLogFilters;
  onChange: (next: AuditLogFilters) => void;
}>) {
  const t = useT();
  const ends = [
    { key: "from", label: t("settings.auditLog.from") },
    { key: "to", label: t("settings.auditLog.to") },
  ] as const;
  return (
    <div className="audit-log-grid">
      {ends.map((end) => (
        <Field key={end.key} label={end.label}>
          {(control) => (
            <DateInput
              {...control}
              value={dateValue(filters[end.key])}
              onChange={(event) =>
                onChange({ ...filters, [end.key]: event.target.value })
              }
            />
          )}
        </Field>
      ))}
    </div>
  );
}

// The dials, always in view above the trail: a record search, then When, Who,
// Action and Record type in one row. Each applies on its own (debounced by the
// card), so this is a bar and not a form: there is nothing to submit.
export function AuditLogToolbar({
  filters,
  meUserId,
  onChange,
}: Readonly<{
  filters: AuditLogFilters;
  meUserId?: string;
  onChange: (next: AuditLogFilters) => void;
}>) {
  const t = useT();
  const options = useDialOptions();
  const who = useWhoOptions(meUserId);
  // The name of the record the picker chose, held only while its id is still
  // the filter: a cleared or retyped Record ID drops it.
  const [picked, setPicked] = useState<RecordPickerCandidate | null>(null);
  const selected = picked?.id === filters.entityId ? picked : null;
  return (
    <div className="audit-log-toolbar">
      <Field label={t("settings.auditLog.record")}>
        {(control) => (
          <RecordPicker
            id={control.id}
            label={t("settings.auditLog.recordSearch")}
            searchTargets={searchAuditRecords}
            selected={selected}
            onPick={(candidate) => {
              setPicked(candidate);
              onChange({ ...filters, entityId: candidate.id });
            }}
          />
        )}
      </Field>
      <div className="audit-log-dials">
        <Field label={t("settings.auditLog.when")}>
          {(control) => (
            <Select
              {...control}
              options={options.when}
              value={filters.when}
              onChange={(when) => onChange({ ...filters, when: asWhen(when) })}
            />
          )}
        </Field>
        <Field label={t("settings.auditLog.who")}>
          {(control) => (
            <Select
              {...control}
              options={who}
              value={filters.actor}
              // A typed principal no option names (an agent, a connector) still
              // shows what the trail is narrowed to.
              placeholder={filters.actor}
              onChange={(actor) => onChange({ ...filters, actor })}
            />
          )}
        </Field>
        <Field label={t("settings.auditLog.action")}>
          {(control) => (
            <Select
              {...control}
              options={options.action}
              value={filters.action}
              onChange={(action) => onChange({ ...filters, action })}
            />
          )}
        </Field>
        <Field label={t("settings.auditLog.recordType")}>
          {(control) => (
            <Select
              {...control}
              options={options.type}
              value={filters.entityType}
              onChange={(entityType) => onChange({ ...filters, entityType })}
            />
          )}
        </Field>
      </div>
      {filters.when === "custom" && (
        <AuditLogRange filters={filters} onChange={onChange} />
      )}
      <AuditLogIdSearch filters={filters} onChange={onChange} />
    </div>
  );
}
