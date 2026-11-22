// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import {
  type UserOption,
  useMemberFilterOptions,
} from "../../components/assigneepicker";
import { Field, TextInput } from "../../design-system/atoms";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { FilterBar } from "../../design-system/filterbar";
import { Select, type SelectOption } from "../../design-system/select";
import { forReader } from "../../format/collate";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { actorAttribution, HUMAN_ACTOR_PREFIX } from "../../lib/auditactor";
import {
  AUDIT_ACTION_VALUES,
  AUDIT_ENTITY_TYPE_VALUES,
  actionLabel,
  entityLabel,
} from "./audit";
import {
  type AuditLogEntry,
  type AuditLogFilters,
  isFullId,
  isRangeReversed,
} from "./settings-audit.format";

// Each dial applies on its own, so there is nothing to submit.
export function AuditLogFilterBar({
  filters,
  entries,
  onChange,
}: Readonly<{
  filters: AuditLogFilters;
  entries: readonly AuditLogEntry[];
  onChange: (next: AuditLogFilters) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const set = (key: keyof AuditLogFilters) => (value: string) =>
    onChange({ ...filters, [key]: value });
  const dateDial = (key: "from" | "to", label: MessageKey, error?: string) => (
    <Field label={t(label)} error={error}>
      {(field) => (
        <DateInput
          {...field}
          value={isoOrBlank(filters[key])}
          onChange={(event) => set(key)(event.target.value)}
        />
      )}
    </Field>
  );
  return (
    <FilterBar label={t("settings.auditFilters")}>
      <div className="auditlog-dials">
        <ActorDial
          value={filters.actor}
          entries={entries}
          onChange={set("actor")}
        />
        <Field label={t("settings.auditEntity")}>
          {(field) => (
            <Select
              {...field}
              value={filters.entityType}
              onChange={set("entityType")}
              options={entityTypeOptions(filters.entityType, t, locale)}
            />
          )}
        </Field>
        <Field label={t("settings.auditAction")}>
          {(field) => (
            <Select
              {...field}
              value={filters.action}
              onChange={set("action")}
              options={[
                { value: "", label: t("settings.auditAnyAction") },
                ...sortedByLabel(
                  AUDIT_ACTION_VALUES.map((action) => ({
                    value: action,
                    label: actionLabel(action, t),
                  })),
                  locale,
                ),
              ]}
            />
          )}
        </Field>
        <EntityIdDial value={filters.entityId} onChange={set("entityId")} />
        {dateDial("from", "settings.auditFrom")}
        {dateDial(
          "to",
          "settings.auditTo",
          isRangeReversed(filters)
            ? t("settings.auditRangeReversed")
            : undefined,
        )}
      </div>
    </FilterBar>
  );
}

function sortedByLabel<O extends SelectOption>(
  options: readonly O[],
  locale: Locale,
): O[] {
  return [...options].sort((a, b) => forReader(a.label, b.label, locale));
}

// The labels the Target column shows, sending the key the server stores. A
// type a link named and this client has no label for still shows as chosen.
function entityTypeOptions(
  chosen: string,
  t: Translator,
  locale: Locale,
): SelectOption[] {
  const types =
    chosen && !AUDIT_ENTITY_TYPE_VALUES.includes(chosen)
      ? [...AUDIT_ENTITY_TYPE_VALUES, chosen]
      : AUDIT_ENTITY_TYPE_VALUES;
  return [
    { value: "", label: t("settings.auditAnyEntity") },
    ...sortedByLabel(
      types.map((type) => ({ value: type, label: entityLabel(type, t) })),
      locale,
    ),
  ];
}

function ActorDial({
  value,
  entries,
  onChange,
}: Readonly<{
  value: string;
  entries: readonly AuditLogEntry[];
  onChange: (next: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const members = useMemberFilterOptions(true).options;
  const options = actorOptions(value, members, entries, t, locale);
  const member = value.startsWith(HUMAN_ACTOR_PREFIX);
  return (
    <Field
      label={t("settings.auditActor")}
      hint={member ? t("settings.auditActorDelegated") : undefined}
    >
      {(field) => (
        <Select
          {...field}
          value={value}
          onChange={onChange}
          options={[
            { value: "", label: t("settings.auditAnyActor") },
            ...options,
          ]}
        />
      )}
    </Field>
  );
}

// Members come from the roster. Agents have no directory, so the dial offers
// the ones on the rows it has read that act for nobody. An agent acting for a
// member is found through that member.
export function actorOptions(
  chosen: string,
  members: readonly UserOption[],
  entries: readonly AuditLogEntry[],
  t: Translator,
  locale: Locale,
): SelectOption[] {
  const colleagues = members.map((member) => ({
    ...member,
    value: HUMAN_ACTOR_PREFIX + member.value,
  }));
  const machines = new Map<string, string>();
  for (const entry of entries) {
    const machine =
      entry.actor_type === "agent" || entry.actor_type === "connector";
    if (machine && !entry.on_behalf_of) {
      const who = actorAttribution(entry, undefined);
      machines.set(entry.actor_id, who.identifier ?? entry.actor_id);
    }
  }
  const options: SelectOption[] = [
    ...sortedByLabel(colleagues, locale),
    ...sortedByLabel(
      [...machines].map(([id, label]) => ({ value: id, label })),
      locale,
    ),
  ];
  if (chosen && !options.some((option) => option.value === chosen)) {
    const label = chosen.startsWith(HUMAN_ACTOR_PREFIX)
      ? t("audit.unknownMember")
      : chosen;
    options.push({ value: chosen, label });
  }
  return options;
}

// A part of an id stays at the field with a hint, as the server answers only
// a whole one. The list keeps the last filter it was given.
function EntityIdDial({
  value,
  onChange,
}: Readonly<{ value: string; onChange: (next: string) => void }>) {
  const t = useT();
  const [draft, setDraft] = useState(value);
  const [applied, setApplied] = useState(value);
  if (value !== applied) {
    setApplied(value);
    setDraft(value);
  }
  const typed = draft.trim();
  const refused = typed !== "" && !isFullId(typed);
  return (
    <Field
      label={t("settings.auditEntityId")}
      error={refused ? t("settings.auditEntityIdHint") : undefined}
    >
      {(field) => (
        <TextInput
          {...field}
          value={draft}
          onChange={(event) => {
            const next = event.target.value;
            setDraft(next);
            const trimmed = next.trim();
            if (trimmed === "" || isFullId(trimmed)) {
              setApplied(trimmed);
              onChange(trimmed);
            }
          }}
        />
      )}
    </Field>
  );
}
