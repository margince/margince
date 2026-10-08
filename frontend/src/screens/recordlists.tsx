// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lists one record is on, on its own page: every Shortlist it was chosen
// for and every Live List whose filter selects it now, among the lists the
// reader may find. "Check a list" answers the question a member table cannot:
// why this record is NOT on a Live List, clause by clause.

import { useCallback, useRef, useState } from "react";
import { navigate } from "../app/router";
import { Button, Field } from "../design-system/atoms";
import { useFocusHandoff } from "../design-system/focushandoff";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SurfaceState } from "../design-system/surfacestate";
import { undoAction, useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import { LiveExplanation } from "./listexplain";
import { ListKindBadge } from "./listlibrary";
import {
  type List,
  type ListedRecordType,
  useExplanation,
  useLists,
  useListsAvailable,
  useRecordLists,
  useRemoveMember,
  useRestoreMember,
} from "./lists.queries";
import "./lists.css";

type RecordRef = Readonly<{ entityType: ListedRecordType; entityId: string }>;

/** The block as a card of its own, for a record page's column. */
export function RecordListsPanel({ entityType, entityId }: RecordRef) {
  const t = useT();
  if (!useListsAvailable()) {
    return null;
  }
  return (
    <Panel title={t("lists.record.title")}>
      <PanelBody>
        <RecordListsBody entityType={entityType} entityId={entityId} />
      </PanelBody>
    </Panel>
  );
}

/** The block's content, for a host that frames it itself. */
export function RecordListsBody({ entityType, entityId }: RecordRef) {
  const t = useT();
  const lists = useRecordLists(entityType, entityId);
  const found = lists.data?.data ?? [];
  // Where focus lands when a row leaves with the control that held it.
  const block = useRef<HTMLDivElement | null>(null);
  const focusLanding = useCallback(() => block.current, []);
  return (
    <div className="lists-record" ref={block} tabIndex={-1}>
      <SurfaceState
        state={
          lists.isPending
            ? "loading"
            : lists.isError
              ? "unavailable"
              : found.length > 0
                ? "ready"
                : "empty"
        }
        emptyLabel={t("lists.record.empty")}
        loadingLabel={t("lists.record.loading")}
        loadingLines={2}
      >
        <ul className="lists-record-set">
          {found.map((list) => (
            <RecordListRow
              key={list.id}
              list={list}
              entityId={entityId}
              focusLanding={focusLanding}
            />
          ))}
        </ul>
        {lists.data?.truncated && (
          <p className="t-caption">{t("lists.record.truncated")}</p>
        )}
      </SurfaceState>
      <CheckAList entityType={entityType} entityId={entityId} />
    </div>
  );
}

/** Pick a Live List of this record's type and read its verdict for the record. */
function CheckAList({ entityType, entityId }: RecordRef) {
  const t = useT();
  const live = useLists({ entityType, listType: "dynamic" });
  const [picked, setPicked] = useState("");
  const options = (live.data?.data ?? []).map((list) => ({
    value: list.id,
    label: list.name,
  }));
  if (options.length === 0) {
    return null;
  }
  return (
    <div className="lists-record-check">
      <Field label={t("lists.record.check")}>
        {(control) => (
          <Select
            {...control}
            value={picked}
            onChange={setPicked}
            placeholder={t("lists.record.checkPick")}
            options={options}
          />
        )}
      </Field>
      {picked !== "" && (
        <CheckedList
          listId={picked}
          entityType={entityType}
          entityId={entityId}
        />
      )}
    </div>
  );
}

function CheckedList({
  listId,
  entityType,
  entityId,
}: RecordRef & Readonly<{ listId: string }>) {
  const t = useT();
  const why = useExplanation(listId, entityType, entityId);
  return (
    <SurfaceState
      state={why.isPending ? "loading" : why.isError ? "unavailable" : "ready"}
      emptyLabel={t("lists.why.loading")}
      loadingLabel={t("lists.why.loading")}
      loadingLines={3}
    >
      {why.data && <LiveExplanation entityType={entityType} why={why.data} />}
    </SurfaceState>
  );
}

function RecordListRow({
  list,
  entityId,
  focusLanding,
}: Readonly<{
  list: List;
  entityId: string;
  focusLanding: () => HTMLElement | null;
}>) {
  const row = useRef<HTMLLIElement | null>(null);
  useFocusHandoff(row, focusLanding);
  return (
    <li ref={row}>
      <button
        type="button"
        className="link-button"
        onClick={() => navigate({ screen: "lists", id: list.id })}
      >
        {list.name}
      </button>
      <ListKindBadge list={list} />
      {list.list_type === "static" && list.can_edit && (
        <TakeOffAction list={list} entityId={entityId} />
      )}
    </li>
  );
}

/**
 * Taking this record off a Shortlist runs at once: the toast's Undo puts the
 * membership back with its author, date and note.
 */
function TakeOffAction({
  list,
  entityId,
}: Readonly<{ list: List; entityId: string }>) {
  const t = useT();
  const remove = useTakeOff(list.name);
  return (
    <Button
      variant="ghost"
      pending={remove.isPending}
      onClick={() =>
        remove.mutate({
          listId: list.id,
          entityType: list.entity_type,
          entityId,
        })
      }
    >
      {t("lists.remove")}
    </Button>
  );
}

// A refused removal or Undo has no dialog to stand in, so it stays as a toast.
function useTakeOff(name: string) {
  const t = useT();
  const toast = useToast();
  const sayRefused = (error: Error) =>
    toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
  const restore = useRestoreMember({
    onError: sayRefused,
    onSuccess: () => toast.show(t("lists.record.putBack", { name })),
  });
  return useRemoveMember({
    onError: sayRefused,
    onSuccess: (removal) =>
      toast.show(
        t("lists.record.takenOff", { name }),
        removal
          ? {
              action: undoAction(t("common.undo"), () =>
                restore.mutate(removal),
              ),
            }
          : undefined,
      ),
  });
}
