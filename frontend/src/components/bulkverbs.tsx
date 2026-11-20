// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { useRowSelection } from "../design-system/listselection";
import type { ListSelection } from "../design-system/listtable";
import { Select } from "../design-system/select";
import { stable } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { RequiredGrant } from "../lib/capability";
import { useActionRefusal, useSeatRefusal } from "../lib/writerefusal";
import { useRecordOwnerOptions } from "./assigneepicker";
import {
  BulkChangeDialog,
  type BulkChangeRequest,
  type BulkChangeResult,
  type BulkRow,
} from "./bulkchange";
import { BULK_MAX_ITEMS } from "./bulkchangeapi";
import { ShortlistVerb } from "./bulkshortlist";
import { TagVerbs } from "./bulktag";
import { TaskVerb } from "./bulktask";
import {
  RosterFailedLine,
  RosterPartialNote,
  useRosterPartial,
} from "./entityref";

// The record lists' types: a Worklist item has its own bar
// (worklist.bulkdone.tsx) and takes none of these verbs.
type BulkRecordType = Exclude<
  components["schemas"]["BulkRecordType"],
  "worklist_item"
>;
type BulkVerb = components["schemas"]["BulkVerb"];

/**
 * The verbs every record list offers over its selection: hand the rows to an
 * owner, tag them, file a task under each, put them on a Shortlist, or archive
 * them. Each opens `BulkChangeDialog`; nothing is written before the reader has
 * seen the preview.
 *
 * `children` are a list's own further verbs. They get the bar's refusal
 * sentence id when `childrenRequire`, or else the seat, refuses them.
 */
export function BulkVerbs({
  recordType,
  rows,
  busy = false,
  onDone,
  shortlist,
  childrenRequire,
  children,
}: Readonly<{
  recordType: BulkRecordType;
  /** The selected rows, with the versions the list currently holds. */
  rows: readonly BulkRow[];
  /** Another verb of the caller's own is running. */
  busy?: boolean;
  onDone: (result: BulkChangeResult) => void;
  /** The Shortlist these rows are shown on, which the rows can be taken off. */
  shortlist?: Readonly<{ id: string; name: string }>;
  /** The grant the list's own verbs ask. */
  childrenRequire?: RequiredGrant;
  children?: (reasonId: string | undefined) => ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [ownerId, setOwnerId] = useState("");
  const [request, setRequest] = useState<BulkChangeRequest | null>(null);
  const { options: ownerOptions, roster } = useRecordOwnerOptions();
  const rosterPartial = useRosterPartial("user", true);
  const partialNoteId = useId();
  const refusalId = useId();
  const refused = useBulkRefusals(recordType, childrenRequire);
  const refusedId = (verb: keyof BulkRefusals) =>
    refused[verb] === undefined ? undefined : refusalId;
  const sentence = Object.values(refused).find(
    (refusal) => refusal !== undefined,
  );
  const ownerDescribedBy =
    [rosterPartial ? partialNoteId : undefined, refusedId("update")]
      .filter(Boolean)
      .join(" ") || undefined;

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

  const open = (
    verb: BulkVerb,
    also: Pick<BulkChangeRequest, "list" | "tag" | "task"> = {},
  ) =>
    setRequest({
      recordType,
      verb,
      rows: [...rows],
      ownerId: verb === "reassign_owner" ? ownerId : undefined,
      openId: crypto.randomUUID(),
      ...also,
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
        disabled={busy || refused.update !== undefined}
        onChange={setOwnerId}
        // The caveat and any refusal sit last in the row (below), so the
        // picker names them.
        aria-describedby={ownerDescribedBy}
        options={ownerOptions}
      />
      <Button
        variant="primary"
        disabled={!idle || ownerId === ""}
        reasonId={refusedId("update")}
        onClick={() => open("reassign_owner")}
      >
        {t("bulk.assign")}
      </Button>
      {children?.(refusedId("children"))}
      <TagVerbs
        disabled={!idle}
        reasonId={refusedId("update")}
        onPick={(verb, tag) => open(verb, { tag })}
      />
      <TaskVerb
        disabled={!idle}
        reasonId={refusedId("task")}
        onReady={(task) => open("create_task", { task })}
      />
      <ShortlistVerb
        recordType={recordType}
        disabled={!idle}
        reasonId={refusedId("seat")}
        exclude={shortlist?.id}
        onPick={(list) => open("add_to_list", { list })}
      />
      {shortlist && (
        <Button
          disabled={!idle}
          reasonId={refusedId("seat")}
          onClick={() => open("remove_from_list", { list: shortlist })}
        >
          {t("bulk.removeFromThisShortlist")}
        </Button>
      )}
      {/* A lead has no archive: it leaves the queue by being disqualified. */}
      {recordType !== "lead" && (
        <Button
          disabled={!idle}
          reasonId={refusedId("archive")}
          onClick={() => open("archive")}
        >
          {t("bulk.archive")}
        </Button>
      )}
      {/* Last, after every verb: the bar is one wrapping flex row, and a
          sentence between the picker and its button is where the row would
          break, splitting the control from its verb. */}
      {sentence !== undefined && (
        <span className="t-caption" id={refusalId}>
          {sentence}
        </span>
      )}
      <RosterPartialNote partial={rosterPartial} id={partialNoteId} />
      <RosterFailedLine kind="user" roster={roster} />
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

type BulkRefusals = Readonly<{
  update: string | undefined;
  task: string | undefined;
  archive: string | undefined;
  seat: string | undefined;
  children: string | undefined;
}>;

// Why each verb is refused, from the grant its server admission asks up front
// (compose/bulkrecordverbs.go admitVerb). A new owner and a tag ask the
// record's update; a task asks the activity create. Archive is admitted on
// update or delete. A Shortlist change asks only the list's own rules, so
// only a read seat refuses it, as it refuses every bulk write.
function useBulkRefusals(
  recordType: BulkRecordType,
  childrenRequire: RequiredGrant | undefined,
): BulkRefusals {
  const update = useActionRefusal({ object: recordType, action: "update" });
  const remove = useActionRefusal({ object: recordType, action: "delete" });
  const task = useActionRefusal({ object: "activity", action: "create" });
  const seat = useSeatRefusal();
  const childGrant = useActionRefusal(
    childrenRequire ?? { object: recordType, action: "update" },
  );
  return {
    update,
    task,
    archive: update !== undefined && remove !== undefined ? update : undefined,
    seat,
    children: childrenRequire ? childGrant : seat,
  };
}

/** A list row the bulk verbs can act on. */
export type BulkSelectable = Readonly<{
  id: string;
  version?: number;
  archived_at?: string | null;
}>;

/**
 * Row selection plus the bulk bar, for a record list whose only bulk verbs are
 * `BulkVerbs`.
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
  const chosen = useRowSelection({
    rows,
    keyOf: (row) => row.id,
    limit: BULK_MAX_ITEMS,
  });
  return {
    selected: chosen.selected,
    limit: chosen.limit,
    // An archived row takes neither verb: it has no owner to move and is
    // already archived.
    selectable: (row) => row.archived_at == null,
    onChange: chosen.onChange,
    label: (row) => t("bulk.selectRow", { name: labelOf(row) }),
    bar: (
      <BulkVerbs
        recordType={recordType}
        rows={chosen.selectedRows.map((row) => ({
          id: row.id,
          version: row.version,
          label: labelOf(row),
        }))}
        onDone={chosen.clear}
      />
    ),
  };
}
