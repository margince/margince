// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge, TableScroll } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { KeyedName } from "../../design-system/keyedname";
import type { ScrollRegionName } from "../../design-system/scrollregion";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import type { ObjectGrant, Role } from "../../lib/roles.queries";
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
  /** The raw key, in the code face under the name, for an operator who asked. */
  code?: string;
  /** A label beside the name, for the rows that differ from the rest. */
  badge?: string;
  grant: ObjectGrant;
  /** Each switch's full accessible name, identifiable out of context. */
  cellLabel: (action: CrudAction) => string;
  /** Whether a write carrying this row's whole grant is in flight. */
  pending: boolean;
  onChange: (action: CrudAction, next: boolean) => void;
}>;

/** A run of rows under one name; a lone section may go unnamed. */
export type GrantMatrixSection = Readonly<{
  key: string;
  title?: string;
  rows: readonly GrantMatrixRow[];
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
  sections,
  canManage,
  readOnlyReason,
  turnOnReason,
  busy = false,
  name,
  bleed,
}: Readonly<{
  /** The first column's header: what the rows are. */
  rowHeader: string;
  sections: readonly GrantMatrixSection[];
  canManage: boolean;
  readOnlyReason: string;
  /** Why a switch that is off may not be turned on; absent when it may. */
  turnOnReason?: string;
  /** Whether another write on the same subject is in flight. */
  busy?: boolean;
  /** Names the table and its scroll region: a phrase, or the heading over it. */
  name: ScrollRegionName;
  /** `TableScroll`'s `bleed`, for a grid standing straight in a `Panel`. */
  bleed?: boolean;
}>) {
  const t = useT();
  return (
    <TableScroll label={name} bleed={bleed} stickyFirst>
      <table
        className="table grant-matrix"
        aria-labelledby={typeof name === "string" ? undefined : name.labelledBy}
        aria-label={typeof name === "string" ? name : undefined}
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
        {sections.map((section) => (
          <tbody key={section.key}>
            {section.title === undefined ? null : (
              <tr className="grant-group">
                <th scope="rowgroup" colSpan={CRUD.length + 1}>
                  <Heading size="xsmall" as="h3">
                    {section.title}
                  </Heading>
                </th>
              </tr>
            )}
            {section.rows.map((row) => (
              <GrantRow
                key={row.key}
                row={row}
                canManage={canManage}
                readOnlyReason={readOnlyReason}
                turnOnReason={turnOnReason}
                busy={busy}
              />
            ))}
          </tbody>
        ))}
      </table>
    </TableScroll>
  );
}

function GrantRow({
  row,
  canManage,
  readOnlyReason,
  turnOnReason,
  busy,
}: Readonly<{
  row: GrantMatrixRow;
  canManage: boolean;
  readOnlyReason: string;
  turnOnReason: string | undefined;
  busy: boolean;
}>) {
  return (
    <tr>
      <th scope="row">
        <span className="grant-row-head">
          {row.code === undefined ? (
            <span>{row.name}</span>
          ) : (
            <KeyedName name={row.name} code={row.code} />
          )}
          {row.badge ? <Badge wrap>{row.badge}</Badge> : null}
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
              // Only a permission denial gets a reason. A write in flight
              // holds a switch too, and a sentence flashing for 200ms on
              // every flip is noise.
              reason={refusal}
              onChange={(next) => row.onChange(action, next)}
            />
          </td>
        );
      })}
    </tr>
  );
}
