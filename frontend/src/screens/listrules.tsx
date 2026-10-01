// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The automation rules that depend on a list, and the archive that pauses
// them. Nothing here blocks a change: a rule whose list is archived pauses
// itself and tells its owner, and archiving only has to say so first.

import { useState } from "react";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import { type List, useArchiveList } from "./lists.queries";

type ListUse = NonNullable<List["dependencies"]>[number];

/** The active automation rules that watch or add to the list. */
export function ruleUsesOf(list: List): ListUse[] {
  return (list.dependencies ?? []).filter((use) => use.kind === "automation");
}

/**
 * Names each rule and what it does with the list. A rule the reader may not
 * open is still counted, so nobody changes a list unaware that something
 * depends on it.
 */
export function ListRuleUses({
  rules,
  lead,
}: Readonly<{ rules: ListUse[]; lead: string }>) {
  const t = useT();
  if (rules.length === 0) {
    return null;
  }
  return (
    <div>
      <p className="t-caption">{lead}</p>
      <ul>
        {rules.map((rule, i) => (
          <li key={rule.automation_id ?? `hidden-${i}`}>
            {t(
              rule.role === "writes"
                ? "lists.rules.writes"
                : "lists.rules.watches",
              { name: rule.automation_name ?? t("lists.rules.hidden") },
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Archives the list. With rules depending on it, the reader first sees which
 * rules the archive will pause.
 */
export function ArchiveListAction({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const archive = useArchiveList();
  const [open, setOpen] = useState(false);
  const rules = ruleUsesOf(list);
  const run = () =>
    archive.mutate(
      { id: list.id, archive: true },
      { onSuccess: () => setOpen(false) },
    );
  return (
    <>
      <Button
        variant="ghost"
        pending={archive.isPending && !open}
        onClick={() => (rules.length > 0 ? setOpen(true) : run())}
      >
        {t("lists.archive")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("lists.rules.archiveTitle")}
        confirmLabel={t("lists.archive")}
        confirmVariant="danger"
        pending={archive.isPending}
        error={archive.isError ? problemMessageOf(archive.error, t) : null}
        onConfirm={run}
      >
        <ListRuleUses rules={rules} lead={t("lists.rules.archiveLead")} />
      </ConfirmModal>
    </>
  );
}
