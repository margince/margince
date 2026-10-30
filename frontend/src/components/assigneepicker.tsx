// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import {
  RosterFailedLine,
  useRoster,
  useRosterPartial,
  useRosterPartialHint,
} from "./entityref";
import { useEveryMember } from "./roster";

/** A workspace user as a picker option. */
export type UserOption = { value: string; label: string };

/**
 * Picker options with the roster read behind them, so a picker can say when
 * that read failed instead of offering an empty list (`RosterFailedLine`).
 */
export type UserOptions = {
  options: UserOption[];
  roster: ReturnType<typeof useRoster>;
};

type RosterEntry = NonNullable<ReturnType<typeof useRoster>["data"]>[number];
type MemberStatus = components["schemas"]["User"]["status"];

/**
 * Whether the server says the reader may make this entry a record's owner.
 *
 * The rule is the server's and depends on the reader's own write scope. So the
 * roster answers it per row (`assignable`) and the client does not re-derive it.
 * An entry without the answer (an older server) is offered, and the write
 * decides.
 */
export function mayOwnRecords(entry: RosterEntry): boolean {
  if (!("display_name" in entry) || entry.is_agent) {
    return false;
  }
  return entry.assignable !== false;
}

/**
 * The colleagues the reader may make a record's owner: an active human seat
 * that can write, inside the reader's own write scope. Every owner picker reads
 * this, so a picker never offers somebody the owner change then refuses.
 */
export function useRecordOwnerOptions(enabled = true): UserOptions {
  const roster = useRoster("user", enabled);
  const options = (roster.data ?? []).filter(mayOwnRecords).map((entry) => ({
    value: entry.id,
    label: "display_name" in entry ? entry.display_name : entry.id,
  }));
  return { options, roster };
}

const STATUS_SUFFIX: Partial<
  Record<
    MemberStatus,
    "list.ownerDeactivated" | "list.ownerSuspended" | "list.ownerInvited"
  >
> = {
  deactivated: "list.ownerDeactivated",
  suspended: "list.ownerSuspended",
  invited: "list.ownerInvited",
};

/**
 * Every member a list can be filtered by owner, those who left included and
 * named as such. That is how a former colleague's records are found to hand on.
 */
export function useMemberFilterOptions(enabled: boolean) {
  const t = useT();
  const walk = useEveryMember(enabled);
  const options: UserOption[] = (walk.data?.entries ?? []).flatMap((entry) => {
    if (!("display_name" in entry) || entry.is_agent) {
      return [];
    }
    const suffix = STATUS_SUFFIX[entry.status];
    return [
      {
        value: entry.id,
        label: suffix
          ? t(suffix, { name: entry.display_name })
          : entry.display_name,
      },
    ];
  });
  return {
    options,
    loading: walk.isPending,
    failed: walk.isError,
    partial: walk.data?.partial === true,
  };
}

/**
 * The workspace's contacts as assignment options — everyone the roster carries
 * LESS agent seats.
 *
 * An agent seat is an Agent Runner identity, not a colleague: the server refuses
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
export function useAssignableUserOptions(enabled = true): UserOptions {
  const roster = useRoster("user", enabled);
  const options = (roster.data ?? [])
    .filter((entry) => !("is_agent" in entry && entry.is_agent))
    .map((entry) => ({
      value: entry.id,
      label: ("display_name" in entry ? entry.display_name : null) ?? entry.id,
    }));
  return { options, roster };
}

/**
 * The assignee picker for a task being written: the workspace's contacts, less
 * agent seats, plus a leading "Unassigned".
 *
 * Renders nothing while `active` is false, and its roster walk is deferred to
 * that same flag — a note or meeting is not held by a colleague, so neither the
 * control nor the `/users` walk behind it appears while one is being logged.
 * Because it OFFERS colleagues it owes the roster-partial caveat beside it: a
 * picker missing contacts looks exactly like a small workspace, so the `Field`
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
  const { options, roster } = useAssignableUserOptions(active);
  const partialHint = useRosterPartialHint(useRosterPartial("user", active));
  if (!active) {
    return null;
  }
  return (
    <>
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
      <RosterFailedLine kind="user" roster={roster} />
    </>
  );
}
