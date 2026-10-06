// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Changing a Live List's filter: the list page's "Edit filter" opens the
// builder on the list's own tree, and "Save to <list>" writes it back to that
// list, held to the version the builder opened.

import { useState } from "react";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemCodeOf, problemMessageOf } from "./common";
import { EDIT_LIST_SEGMENT, tabOfListType } from "./filtersaddress";
import { ListRuleUses, ruleUsesOf } from "./listrules";
import { type List, useList, useUpdateList } from "./lists.queries";
import { decode, encode, isComplete, type Node } from "./segmentpredicate";

/** Whether this reader may change this list's filter here. */
export function mayEditFilter(list: List): boolean {
  return (
    list.list_type === "dynamic" &&
    list.can_edit &&
    !list.archived_at &&
    tabOfListType(list.entity_type) !== undefined
  );
}

/** "Edit filter": the builder, opened on this Live List's filter. */
export function EditFilterAction({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const tab = tabOfListType(list.entity_type);
  if (!mayEditFilter(list) || tab === undefined) {
    return null;
  }
  return (
    <Button
      variant="ghost"
      onClick={() =>
        navigate({ screen: "filters", id: EDIT_LIST_SEGMENT, id2: list.id })
      }
    >
      {t("lists.editFilter")}
    </Button>
  );
}

/** The list the builder is editing, as it was when the builder opened. */
export type EditedList = Readonly<{ list: List; version: number }>;

/**
 * Loads the filter of the Live List the address names into the builder once,
 * when the list is of the tab's record type. A reader who may change the list
 * also gets it back with the version it was read at: a later refetch must not
 * move the version a save is held to, or a colleague's change would be lost.
 */
export function useOpenListFromAddress(
  listId: string | undefined,
  tab: string,
  load: (tree: Node) => void,
): Readonly<{ edited: EditedList | null; opening: boolean }> {
  const list = useList(listId ?? "", listId !== undefined);
  const [edited, setEdited] = useState<EditedList | null>(null);
  const [settled, setSettled] = useState(false);
  if (listId === undefined) {
    return { edited: null, opening: false };
  }
  if (!settled && list.data) {
    const tree = decode(list.data.definition);
    setSettled(true);
    if (tree && tabOfListType(list.data.entity_type) === tab) {
      load(tree);
      if (mayEditFilter(list.data)) {
        setEdited({ list: list.data, version: list.data.version });
      }
    }
  } else if (!settled && list.isError) {
    setSettled(true);
  }
  return { edited, opening: !settled };
}

/** Says, above the builder, which list a save will change. */
export function EditingListNotice({
  edited,
}: Readonly<{ edited: EditedList }>) {
  const t = useT();
  return (
    <Callout
      tone="info"
      title={t("lists.editingTitle", { name: edited.list.name })}
    >
      {t("lists.editingBody")}
    </Callout>
  );
}

/** "Save to <list>": writes the tree on screen to the list being edited. */
export function SaveToListAction({
  edited,
  tree,
}: Readonly<{ edited: EditedList; tree: Node }>) {
  const t = useT();
  const update = useUpdateList();
  const [open, setOpen] = useState(false);
  if (!isComplete(tree)) {
    return null;
  }
  const name = edited.list.name;
  const error = !update.isError
    ? null
    : problemCodeOf(update.error) === "version_skew"
      ? t("lists.saveFilterConflict")
      : problemMessageOf(update.error, t);
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        {t("lists.saveFilterTo", { name })}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => {
          setOpen(false);
          update.reset();
        }}
        title={t("lists.saveFilterTitle", { name })}
        confirmLabel={t("lists.saveFilterConfirm")}
        pending={update.isPending}
        error={error}
        onConfirm={() =>
          update.mutate(
            {
              id: edited.list.id,
              version: edited.version,
              definition: encode(tree) as Record<string, unknown>,
            },
            {
              onSuccess: () =>
                navigate({ screen: "lists", id: edited.list.id }),
            },
          )
        }
      >
        <p>{t("lists.saveFilterBody")}</p>
        <ListRuleUses
          rules={ruleUsesOf(edited.list)}
          lead={t("lists.rules.settingsLeadLive")}
        />
      </ConfirmModal>
    </>
  );
}
