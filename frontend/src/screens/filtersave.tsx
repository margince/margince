// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Save this filter": one question with two answers. A saved view is the
// reader's own way of looking, counted again on each opening; a Live List is
// a shared set whose members join and leave as their records change, checked
// by the same engine that previewed it.

import { useState } from "react";
import { Field, Textarea, TextInput } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import {
  type ObjectTab,
  RESOURCE_OF,
  UNIT_LABEL,
  VIEW_OF,
} from "./filtersaddress";
import { useCreateList, useListsAvailable } from "./lists.queries";
import {
  DEFAULT_AUDIENCE,
  type ListAudience,
  ListAudienceFields,
} from "./listsharing";
import { filterStateFrom, useSaveView } from "./savedviews.queries";
import { encode, type Node } from "./segmentpredicate";

export type SavedKind = "view" | "list";

export function SaveFilterModal({
  open,
  onClose,
  tab,
  tree,
  initialName = "",
  initialKeep = "view",
  onSaved,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  tab: ObjectTab;
  tree: Node;
  initialName?: string;
  initialKeep?: SavedKind;
  onSaved: (kind: SavedKind, id: string, name: string) => void;
}>) {
  const t = useT();
  const listsOn = useListsAvailable();
  const { create } = useSaveView();
  const createList = useCreateList();
  const [name, setName] = useState(initialName);
  const [keep, setKeep] = useState<SavedKind>(initialKeep);
  const [audience, setAudience] = useState<ListAudience>(DEFAULT_AUDIENCE);
  const [purpose, setPurpose] = useState("");
  // Every opening starts from what the caller offers and nothing typed
  // before: a create that offers the last name again is how a duplicate gets
  // made. Reset as it opens rather than as it closes, so the closing dialog
  // still shows what it held.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setName(initialName);
      setKeep(initialKeep);
      setAudience(DEFAULT_AUDIENCE);
      setPurpose("");
    }
  }

  const asList = listsOn && keep === "list";
  const run = asList ? createList : create;
  // Either create, whichever answer started it: flipping "Keep it as" mid-flight
  // must not re-enable the confirm and start a second record.
  const busy = create.isPending || createList.isPending;
  const trimmed = name.trim();
  const close = () => {
    // Escape and the backdrop reach past the disabled Cancel, and resetting a
    // mutation in flight drops its onSuccess: the record lands unannounced.
    if (busy) {
      return;
    }
    create.reset();
    createList.reset();
    onClose();
  };
  // The tree is read at the press, so what is saved is the filter on screen.
  const save = () => {
    if (asList) {
      createList.mutate(
        {
          name: trimmed,
          entityType: RESOURCE_OF[tab],
          listType: "dynamic",
          definition: encode(tree),
          sharing: audience.sharing,
          teamId: audience.sharing === "team" ? audience.teamId : null,
          purpose: purpose.trim(),
        },
        { onSuccess: (list) => onSaved("list", list.id, trimmed) },
      );
      return;
    }
    create.mutate(
      { resource: VIEW_OF[tab], name: trimmed, query: filterStateFrom(tree) },
      { onSuccess: (view) => onSaved("view", view.id, trimmed) },
    );
  };

  return (
    <ConfirmModal
      open={open}
      onClose={close}
      title={t("filters.saveTitle")}
      confirmLabel={t(asList ? "filters.saveListConfirm" : "views.save")}
      confirmDisabled={trimmed === ""}
      pending={busy}
      error={run.isError ? problemMessageOf(run.error, t) : null}
      onConfirm={save}
    >
      <Field label={t("views.name")}>
        {(control) => (
          <TextInput
            {...control}
            value={name}
            placeholder={t("filters.namePlaceholder", {
              records: t(UNIT_LABEL[tab]),
            })}
            onChange={(event) => setName(event.target.value)}
          />
        )}
      </Field>
      {listsOn && (
        <ChoiceList
          legend={t("filters.keepAs")}
          value={keep}
          disabled={busy}
          onChange={setKeep}
          choices={[
            {
              value: "view",
              label: t("filters.library.kindView"),
              description: t("filters.keepViewHint"),
            },
            {
              value: "list",
              label: t("lists.kind.live"),
              description: t("filters.keepListHint"),
            },
          ]}
        />
      )}
      {asList && (
        <>
          <ListAudienceFields
            value={audience}
            onChange={setAudience}
            ownerIsReader
          />
          <Field label={t("filters.purpose")}>
            {(control) => (
              <Textarea
                {...control}
                rows={2}
                value={purpose}
                onChange={(event) => setPurpose(event.target.value)}
              />
            )}
          </Field>
        </>
      )}
    </ConfirmModal>
  );
}
