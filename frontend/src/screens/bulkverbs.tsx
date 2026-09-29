// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import type { ListSelection } from "../design-system/listtable";
import { Select } from "../design-system/select";
import { stable } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import {
  BulkChangeDialog,
  type BulkChangeRequest,
  type BulkChangeResult,
  type BulkRow,
} from "./bulkchange";
import { ShortlistVerb } from "./bulkshortlist";
import { RosterPartialNote, useRoster, useRosterPartial } from "./entityref";

type BulkRecordType = components["schemas"]["BulkRecordType"];
type BulkVerb = components["schemas"]["BulkVerb"];

/**
 * The verbs every record list offers over its selection: hand the rows to an
 * owner, or archive them. Both open `BulkChangeDialog`; nothing is written
 * before the reader has seen the preview.
 *
 * `children` are a list's own further verbs, drawn between the two.
 */
export function BulkVerbs({
  recordType,
  rows,
  busy = false,
  onDone,
  children,
}: Readonly<{
  recordType: BulkRecordType;
  /** The selected rows, with the versions the list currently holds. */
  rows: readonly BulkRow[];
  /** Another verb of the caller's own is running. */
  busy?: boolean;
  onDone: (result: BulkChangeResult) => void;
  children?: ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [ownerId, setOwnerId] = useState("");
  const [request, setRequest] = useState<BulkChangeRequest | null>(null);
  const roster = useRoster("user", true);
  const rosterPartial = useRosterPartial("user", true);
  const partialNoteId = useId();

  // An owner picked for one selection must not be applied to the next, so the
  // picker clears when the membership changes. `stable` because the key is only
  // compared with itself.
  const selectionKey = rows
    .map((row) => row.id)
    .sort(stable)
    .join(",");
  const [armedFor, setArmedFor] = useState(selectionKey);
  if (armedFor !== selectionKey) {
    setArmedFor(selectionKey);
    setOwnerId("");
  }

  const open = (verb: BulkVerb) =>
    setRequest({
      recordType,
      verb,
      rows: [...rows],
      ownerId: verb === "reassign_owner" ? ownerId : undefined,
      openId: crypto.randomUUID(),
    });

  const idle = !busy && rows.length > 0;
  return (
    <>
      <span className="t-caption">
        {plural("bulk.selected", rows.length, {
          count: formatNumber(rows.length, locale),
        })}
      </span>
      <Select
        aria-label={t("bulk.owner")}
        value={ownerId}
        placeholder={t("bulk.ownerPick")}
        disabled={busy}
        onChange={setOwnerId}
        // The caveat sits last in the row (below), so the picker names it.
        aria-describedby={rosterPartial ? partialNoteId : undefined}
        options={(roster.data ?? []).map((entry) => ({
          value: entry.id,
          label: "display_name" in entry ? entry.display_name : entry.id,
        }))}
      />
      <Button
        variant="primary"
        disabled={!idle || ownerId === ""}
        onClick={() => open("reassign_owner")}
      >
        {t("bulk.assign")}
      </Button>
      {children}
      <ShortlistVerb
        recordType={recordType}
        disabled={!idle}
        onPick={(list) =>
          setRequest({
            recordType,
            verb: "add_to_list",
            rows: [...rows],
            list,
            openId: crypto.randomUUID(),
          })
        }
      />
      <Button disabled={!idle} onClick={() => open("archive")}>
        {t("bulk.archive")}
      </Button>
      {/* Last, after every verb: the bar is one wrapping flex row, and a
          sentence between the picker and its button is where the row would
          break, splitting the control from its verb. */}
      <RosterPartialNote partial={rosterPartial} id={partialNoteId} />
      <BulkChangeDialog
        request={request}
        onClose={() => setRequest(null)}
        onDone={(result) => {
          setRequest(null);
          onDone(result);
        }}
      />
    </>
  );
}

/** A list row the bulk verbs can act on. */
export type BulkSelectable = Readonly<{
  id: string;
  version?: number;
  archived_at?: string | null;
}>;

/**
 * Row selection plus the bulk bar, for a record list whose only bulk verbs are
 * `BulkVerbs`. Only rows the list currently holds count as selected, so a row
 * that left the result set cannot linger as a selection nobody can clear.
 */
export function useBulkSelection<Row extends BulkSelectable>({
  rows,
  recordType,
  labelOf,
}: Readonly<{
  rows: readonly Row[];
  recordType: BulkRecordType;
  labelOf: (row: Row) => string;
}>): ListSelection<Row> {
  const t = useT();
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const selectedRows = rows.filter((row) => selected.has(row.id));
  return {
    selected: new Set(selectedRows.map((row) => row.id)),
    // An archived row takes neither verb: it has no owner to move and is
    // already archived.
    selectable: (row) => row.archived_at == null,
    onToggle: (row) =>
      setSelected((prev) => {
        const next = new Set(prev);
        if (next.has(row.id)) {
          next.delete(row.id);
        } else {
          next.add(row.id);
        }
        return next;
      }),
    label: (row) => t("bulk.selectRow", { name: labelOf(row) }),
    bar: (
      <BulkVerbs
        recordType={recordType}
        rows={selectedRows.map((row) => ({
          id: row.id,
          version: row.version,
          label: labelOf(row),
        }))}
        onDone={() => setSelected(new Set())}
      />
    ),
  };
}
