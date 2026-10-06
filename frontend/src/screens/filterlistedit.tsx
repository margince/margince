// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Changing a Live List's filter: the list page's "Edit filter" opens
// `#/filters/list/<id>` straight into the rows of the list's own tree, and
// "Save to <list>" writes it back to that list, held to the version read at
// opening.

import { useState } from "react";
import { navigate } from "../app/router";
import { useGuardedLeave } from "../app/unsaved";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemCodeOf, problemMessageOf } from "./common";
import { useFilterVocabulary } from "./filterdata";
import { useFilterDraft, useFirstAnswer, wireSignature } from "./filterdraft";
import { FilterEditor, useRowsFocus } from "./filtereditor";
import { useFilterExport } from "./filterexport";
import { FilterFoot } from "./filterfoot";
import { FocusedHead, FocusedPending, FocusedState } from "./filterhead";
import { FilterOutcome } from "./filtermatches";
import { usePlainWords } from "./filterpropose";
import {
  EDIT_LIST_SEGMENT,
  type ObjectTab,
  RESOURCE_OF,
  TAB_LABEL,
  tabOfListType,
  UNIT_LABEL,
} from "./filtersaddress";
import { SaveFilterModal, useLandOnSaved } from "./filtersave";
import { ListRuleUses, ruleUsesOf } from "./listrules";
import {
  type List,
  useList,
  useListsAvailable,
  useUpdateList,
} from "./lists.queries";
import { useListAudienceLabel } from "./listsharing";
import {
  decode,
  encode,
  type Group,
  isComplete,
  type Node,
  rootGroup,
} from "./segmentpredicate";
import "./filters.css";

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

/**
 * `#/filters/list/<id>`. Every state prints one h1: no list is read while
 * lists are off, and a list whose filter no builder here can hold says so
 * rather than opening an empty one.
 */
export function ListFilterPage({ listId }: Readonly<{ listId: string }>) {
  const t = useT();
  const listsOn = useListsAvailable();
  const read = useList(listId, listsOn);
  // Its version is the one "Save to" is held to, so a colleague's change
  // since opening is refused rather than overwritten unseen.
  const opened = useFirstAnswer(read);
  const title = t("lists.editFilter");
  if (!listsOn) {
    return <FocusedState title={title} sentence={t("lists.unavailable")} />;
  }
  const tab = opened ? tabOfListType(opened.entity_type) : undefined;
  const tree = opened ? decode(opened.definition) : null;
  if ((read.isError && !opened) || (opened && (!tab || tree === null))) {
    return (
      <FocusedState title={title} sentence={t("lists.filterCannotOpen")} />
    );
  }
  if (!opened || !tab || tree === null) {
    return <FocusedPending title={title} label={t("lists.loading")} />;
  }
  return <ListFilter list={opened} tab={tab} tree={tree} />;
}

function ListFilter({
  list,
  tab,
  tree,
}: Readonly<{ list: List; tab: ObjectTab; tree: Node }>) {
  const t = useT();
  const toast = useToast();
  const audienceOf = useListAudienceLabel();
  const [draft, dispatch] = useFilterDraft(() => rootGroup(tree));
  // The tree on screen when Save was pressed, as on a new filter.
  const [saving, setSaving] = useState<Group | null>(null);
  const resource = RESOURCE_OF[tab];
  const vocabulary = useFilterVocabulary(resource);
  const words = usePlainWords({ resource, dispatch });
  const exportRun = useFilterExport();
  const focus = useRowsFocus();
  const editable = mayEditFilter(list);
  const complete = isComplete(draft.tree);
  const changed = wireSignature(draft.tree) !== wireSignature(tree);
  const leave = useGuardedLeave(changed);
  const land = useLandOnSaved(tab, leave);
  const records = t(UNIT_LABEL[tab]);

  const foot = editable ? (
    <FilterFoot
      resource={resource}
      tree={draft.tree}
      mode={{
        kind: "list",
        changed,
        onDiscard: () => {
          focus.focusRows();
          dispatch({ type: "reset", tree });
        },
        saveTo: (
          <SaveToListAction
            list={list}
            tree={draft.tree}
            disabled={!changed || !complete}
            onSaved={() => {
              toast.show(t("lists.savedTo", { name: list.name }));
              leave({ screen: "lists", id: list.id });
            }}
          />
        ),
      }}
      onSave={() => setSaving(draft.tree)}
      exportRun={exportRun}
    />
  ) : complete ? (
    // A list the reader may read but not change: its filter is theirs to
    // start from, kept the way a new filter is.
    <FilterFoot
      resource={resource}
      tree={draft.tree}
      mode={{ kind: "new" }}
      onSave={() => setSaving(draft.tree)}
      exportRun={exportRun}
    />
  ) : undefined;

  return (
    <div className="wrap filters-screen">
      <FocusedHead
        title={list.name}
        facts={t("filters.listFacts", {
          records: t(TAB_LABEL[tab]),
          who: audienceOf(list),
        })}
      />
      {editable && <EditingListNotice list={list} />}
      <Panel title={t("filters.builderTitle")} footer={foot}>
        <PanelBody>
          <div ref={focus.rows}>
            <FilterEditor
              draft={draft}
              dispatch={dispatch}
              vocabulary={vocabulary}
              words={words}
              records={records}
            />
          </div>
        </PanelBody>
      </Panel>
      <FilterOutcome tab={tab} tree={draft.tree} vocabulary={vocabulary} />
      <SaveFilterModal
        open={saving !== null}
        onClose={() => setSaving(null)}
        tab={tab}
        tree={saving ?? draft.tree}
        onSaved={(kind, id, name) => {
          setSaving(null);
          land(kind, id, name);
        }}
      />
    </div>
  );
}

/** Says, above the builder, which list a save will change. */
export function EditingListNotice({ list }: Readonly<{ list: List }>) {
  const t = useT();
  return (
    <Callout tone="info" title={t("lists.editingTitle", { name: list.name })}>
      {t("lists.editingBody")}
    </Callout>
  );
}

/**
 * "Save to <list>": the page's one emerald verb. It names the automations
 * watching the list before it writes, held to the version of `list`, and
 * hands back to the page once the list holds the tree, so the page decides
 * where the reader goes.
 */
export function SaveToListAction({
  list,
  tree,
  disabled = false,
  onSaved,
}: Readonly<{
  /** The list as read when the page opened. */
  list: List;
  tree: Node;
  disabled?: boolean;
  onSaved: () => void;
}>) {
  const t = useT();
  const update = useUpdateList();
  // The tree the reader confirmed is the one on screen when they asked: an
  // answer landing behind the dialog is not theirs to write to a shared list.
  const [pinned, setPinned] = useState<Node | null>(null);
  const name = list.name;
  const error = !update.isError
    ? null
    : problemCodeOf(update.error) === "version_skew"
      ? t("lists.saveFilterConflict")
      : problemMessageOf(update.error, t);
  return (
    <>
      <Button
        variant="primary"
        disabled={disabled}
        onClick={() => setPinned(tree)}
      >
        {t("lists.saveFilterTo", { name })}
      </Button>
      <ConfirmModal
        open={pinned !== null}
        onClose={() => {
          setPinned(null);
          update.reset();
        }}
        title={t("lists.saveFilterTitle", { name })}
        confirmLabel={t("lists.saveFilterConfirm")}
        pending={update.isPending}
        error={error}
        onConfirm={() =>
          update.mutate(
            {
              id: list.id,
              version: list.version,
              definition: encode(pinned ?? tree) as Record<string, unknown>,
            },
            { onSuccess: onSaved },
          )
        }
      >
        <p>{t("lists.saveFilterBody")}</p>
        <ListRuleUses
          rules={ruleUsesOf(list)}
          lead={t("lists.rules.settingsLeadLive")}
        />
      </ConfirmModal>
    </>
  );
}
