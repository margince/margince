// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useRoster, useRosterPartial, useRosterPartialHint } from "./entityref";

/** A workspace user as a picker option. */
export type UserOption = { value: string; label: string };

/**
 * The workspace's people as assignment options — everyone the roster carries
 * LESS agent seats.
 *
 * An agent seat is an Agent Runner identity, not a person: the server refuses
 * one as an assignee or owner (the activities module's
 * `ensureAssigneeCanHoldWork`), so offering it is a control whose only outcome
 * is a refusal. Every picker that hands work to somebody asks that same
 * question of the same roster, so it is spelled once here rather than re-derived
 * beside each one — a second copy is how the two would drift until one offered a
 * seat the other refused. A caller that OFFERS this list still owes its reader
 * `useRosterPartial` beside it, for the same reason `useRoster` does.
 *
 * `enabled` defers the roster walk for a caller mounted before it needs the
 * list — a composer whose assignee picker only applies to a task has no reason
 * to walk `/users` while a note is being written.
 */
export function useAssignableUserOptions(enabled = true): UserOption[] {
  const roster = useRoster("user", enabled);
  return (roster.data ?? [])
    .filter((entry) => !("is_agent" in entry && entry.is_agent))
    .map((entry) => ({
      value: entry.id,
      label: ("display_name" in entry ? entry.display_name : null) ?? entry.id,
    }));
}

/**
 * The assignee picker for a task being written: the workspace's people, less
 * agent seats, plus a leading "Unassigned".
 *
 * Renders nothing while `active` is false, and its roster walk is deferred to
 * that same flag — a note or meeting is not held by a colleague, so neither the
 * control nor the `/users` walk behind it appears while one is being logged.
 * Because it OFFERS colleagues it owes the roster-partial caveat beside it: a
 * picker missing people looks exactly like a small workspace, so the `Field`
 * carries the hint into the control's `aria-describedby`.
 */
export function TaskAssigneeField({
  active,
  value,
  onChange,
}: Readonly<{
  active: boolean;
  value: string;
  onChange: (next: string) => void;
}>) {
  const t = useT();
  const options = useAssignableUserOptions(active);
  const partialHint = useRosterPartialHint(useRosterPartial("user", active));
  if (!active) {
    return null;
  }
  return (
    <Field label={t("log.assignee")} hint={partialHint}>
      {(control) => (
        <Select
          {...control}
          options={[{ value: "", label: t("log.unassigned") }, ...options]}
          value={value}
          onChange={onChange}
        />
      )}
    </Field>
  );
}
