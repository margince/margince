// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The automation rules that depend on a list, and the archive that pauses
// them. Nothing here blocks a change: a rule whose list is archived pauses
// itself and tells its owner, and archiving only has to say so first.

import { useState } from "react";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { undoAction, useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { type List, useArchiveList } from "../../lib/lists.queries";
import { problemMessageOf } from "../../lib/problem";

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
 * Archives the list. With no rule on it, the archive runs at once and offers
 * Undo, a restore that loses nothing. A restore does not start a paused rule
 * again, so with rules the reader first sees which ones it pauses, and gets
 * no Undo.
 *
 * It is a menu item, where danger keeps the red ink without the fill. The menu
 * closes on the press, so a failed direct archive opens the dialog to say why.
 */
export function ArchiveListAction({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const toast = useToast();
  const archive = useArchiveList();
  const [open, setOpen] = useState(false);
  const rules = ruleUsesOf(list);
  // The promise, not the mutate callbacks: the archived list's page drops this
  // menu, and a callback of an unmounted caller never runs.
  const restore = () =>
    archive
      .mutateAsync({ id: list.id, archive: false })
      .catch((error: unknown) =>
        toast.show(problemMessageOf(error, t), { tone: "danger" }),
      );
  const run = () =>
    archive.mutateAsync({ id: list.id, archive: true }).then(
      () => {
        setOpen(false);
        toast.show(
          t("lists.archivedDone"),
          rules.length > 0
            ? undefined
            : { action: undoAction(t("common.undo"), restore) },
        );
      },
      () => setOpen(true),
    );
  return (
    <>
      <Button
        variant="danger"
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
