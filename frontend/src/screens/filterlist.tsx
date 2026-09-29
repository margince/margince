// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Save as Live List": the filter being built, saved as a list the team works
// from. A saved view is this reader's own way of looking; a Live List is a
// shared set whose members join and leave as their records change, evaluated
// by the same engine that previewed it.

import { navigate } from "../app/router";
import { NamePrompt } from "../design-system/nameprompt";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import type { FilterResource } from "./filterdata";
import { useCreateList, useListsAvailable } from "./lists.queries";
import { encode, isComplete, type Node } from "./segmentpredicate";

/**
 * Offered once the tree is complete — the server validates the definition on
 * create, so an unfinished one would only earn a refusal — and only while
 * lists are switched on.
 */
export function SaveFilterListAction({
  resource,
  tree,
}: Readonly<{ resource: FilterResource; tree: Node }>) {
  const t = useT();
  const available = useListsAvailable();
  const save = useCreateList();
  if (!available || !isComplete(tree) || resource === "project") {
    return null;
  }
  return (
    <NamePrompt
      trigger={t("filters.saveList")}
      title={t("filters.saveListTitle")}
      label={t("lists.name")}
      confirmLabel={t("filters.saveListConfirm")}
      pending={save.isPending}
      problem={save.isError ? problemMessageOf(save.error, t) : undefined}
      // Encoded at save time, so what is saved is the filter on screen rather
      // than one a stale closure held.
      onSave={(name, done) =>
        save.mutate(
          {
            name,
            entityType: resource,
            listType: "dynamic",
            definition: encode(tree) as Record<string, unknown>,
          },
          {
            onSuccess: (list) => {
              done();
              navigate({ screen: "lists", id: list.id });
            },
          },
        )
      }
    />
  );
}
