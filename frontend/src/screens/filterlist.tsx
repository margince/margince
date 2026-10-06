// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Save as Live List": the filter being built, saved as a list the team works
// from. A saved view is this reader's own way of looking; a Live List is a
// shared set whose members join and leave as their records change, evaluated
// by the same engine that previewed it.

import { useState } from "react";
import { navigate } from "../app/router";
import { Button, Field, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import type { FilterResource } from "./filterdata";
import { useCreateList, useListsAvailable } from "./lists.queries";
import {
  DEFAULT_AUDIENCE,
  type ListAudience,
  ListAudienceFields,
} from "./listsharing";
import { encode, isComplete, type Node } from "./segmentpredicate";

/**
 * Offered once the tree is complete — the server validates the definition on
 * create, so an unfinished one would only earn a refusal — and only while
 * lists are switched on. It asks who can find the list as well as its name, so
 * nobody saves a list without seeing who it is shared with.
 */
export function SaveFilterListAction({
  resource,
  tree,
}: Readonly<{ resource: FilterResource; tree: Node }>) {
  const t = useT();
  const available = useListsAvailable();
  const save = useCreateList();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [audience, setAudience] = useState<ListAudience>(DEFAULT_AUDIENCE);
  if (!available || !isComplete(tree) || resource === "project") {
    return null;
  }
  // Nothing is held between openings, so a reopened dialog never offers the
  // last name again, which on a create is how a duplicate gets made.
  const close = () => {
    setOpen(false);
    setName("");
    setAudience(DEFAULT_AUDIENCE);
  };
  const trimmed = name.trim();
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("filters.saveList")}</Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("filters.saveListTitle")}
        confirmLabel={t("filters.saveListConfirm")}
        confirmDisabled={trimmed === ""}
        pending={save.isPending}
        error={save.isError ? problemMessageOf(save.error, t) : null}
        // Encoded at save time, so what is saved is the filter on screen rather
        // than one a stale closure held.
        onConfirm={() =>
          save.mutate(
            {
              name: trimmed,
              entityType: resource,
              listType: "dynamic",
              definition: encode(tree) as Record<string, unknown>,
              sharing: audience.sharing,
              teamId: audience.sharing === "team" ? audience.teamId : null,
            },
            {
              onSuccess: (list) => {
                close();
                navigate({ screen: "lists", id: list.id });
              },
            },
          )
        }
      >
        <Field label={t("lists.name")}>
          {(control) => (
            <TextInput
              {...control}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </Field>
        <ListAudienceFields
          value={audience}
          onChange={setAudience}
          ownerIsReader
        />
      </ConfirmModal>
    </>
  );
}
