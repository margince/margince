// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge, TableScroll } from "../design-system/atoms";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import type { ObjectGrant, Role } from "./roles.queries";
import "./grantmatrix.css";

// The grid a role grant is written through: one row per subject, one column per
// verb, one switch per cell. The Extensions page draws it with a row per ROLE
// over one object, and the role editor with a row per OBJECT over one role; the
// grid is the same, and so is what a flipped switch writes.

/** The verbs a grant carries, as the contract spells them. */
export type CrudAction = components["schemas"]["RbacAction"];

// Read first, because it decides whether a screen renders at all, and its
// absence is what produces the confusing empty state.
export const CRUD: readonly CrudAction[] = [
  "read",
  "create",
  "update",
  "delete",
];

// The grant a role holds on an object, with an absent key resolving to the
// zero grant — the fail-closed reading /me gets too. The lookup is widened to
// `| undefined` because the generated index signature types every key as
// present, which would make the `??` look like dead code.
const NO_GRANT: ObjectGrant = {
  read: false,
  create: false,
  update: false,
  delete: false,
};

export function grantOf(role: Role, object: string): ObjectGrant {
  const objects: Readonly<Record<string, ObjectGrant | undefined>> =
    role.objects;
  return objects[object] ?? NO_GRANT;
}

/** One row of the grid: its subject, the grant it holds, and its write. */
export type GrantMatrixRow = Readonly<{
  key: string;
  name: string;
  /** A label beside the name, for the rows that differ from the rest. */
  badge?: string;
  grant: ObjectGrant;
  /** Each switch's full accessible name, identifiable out of context. */
  cellLabel: (action: CrudAction) => string;
  /** Whether a write carrying this row's whole grant is in flight. */
  pending: boolean;
  onChange: (action: CrudAction, next: boolean) => void;
}>;

/**
 * A real `<table>`, because the two axes ARE the meaning: a screen-reader user
 * landing on a switch mid-grid asks which row and which verb it belongs to, and
 * `scope` answers. Every cell is a `Switch` rather than a checkbox, because a
 * flip WRITES the grant — there is no Save.
 *
 * A reader who may not write gets `readOnlyReason` on every switch, attached by
 * `aria-describedby` and hidden from the eye: the caller states it once above
 * the grid. `turnOnReason` does the same for a reader who may turn rights off
 * and not on, on the switches that are off. `busy` holds every switch still
 * while a write the grid cannot see through is in flight.
 */
export function GrantMatrix({
  rowHeader,
  rows,
  canManage,
  readOnlyReason,
  turnOnReason,
  busy = false,
  labelledBy,
  scrollLabel,
  bleed,
}: Readonly<{
  /** The first column's header: what the rows are. */
  rowHeader: string;
  rows: readonly GrantMatrixRow[];
  canManage: boolean;
  readOnlyReason: string;
  /** Why a switch that is off may not be turned on; absent when it may. */
  turnOnReason?: string;
  /** Whether another write on the same subject is in flight. */
  busy?: boolean;
  /** The id of the label naming the grid; without one `scrollLabel` names it. */
  labelledBy?: string;
  /** The name the scroll region announces once the grid overflows. */
  scrollLabel: string;
  /** `TableScroll`'s `bleed`, for a grid standing straight in a `Panel`. */
  bleed?: boolean;
}>) {
  const t = useT();
  return (
    <TableScroll label={scrollLabel} bleed={bleed} stickyFirst>
      <table
        className="table grant-matrix"
        aria-labelledby={labelledBy}
        aria-label={labelledBy ? undefined : scrollLabel}
      >
        <thead>
          <tr>
            <th scope="col">{rowHeader}</th>
            {CRUD.map((action) => (
              <th scope="col" key={action}>
                {t(`extAccess.action.${action}`)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key}>
              <th scope="row">
                <span className="grant-row-head">
                  <span>{row.name}</span>
                  {row.badge ? <Badge>{row.badge}</Badge> : null}
                </span>
              </th>
              {CRUD.map((action) => {
                const refusal = !canManage
                  ? readOnlyReason
                  : row.grant[action]
                    ? undefined
                    : turnOnReason;
                return (
                  <td key={action} className="grant-cell">
                    <Switch
                      labelHidden
                      label={row.cellLabel(action)}
                      checked={row.grant[action]}
                      // The row being written keeps focus through `pending`;
                      // every other switch waits for it.
                      disabled={busy && !row.pending}
                      pending={row.pending}
                      // Only a permission denial gets a reason; a write in
                      // flight is the other way a switch is held, and a
                      // sentence flashing for 200ms on every flip is noise.
                      reason={refusal}
                      onChange={(next) => row.onChange(action, next)}
                    />
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </TableScroll>
  );
}
