// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";

import { useCan, useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Modal,
  OverflowMenu,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import { TagPill } from "../design-system/tagpill";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, QueryStates } from "./common";
import type { Tag } from "./tagadmin.queries";
import {
  useArchiveTag,
  useMergeTags,
  useRestoreTag,
  useTagCatalog,
} from "./tagadmin.queries";
import { TagDialog } from "./tagdialog";
import { useUndoableRemoval } from "./undoableremoval";
import "./tagadmin.css";

type TagVerbs = Readonly<{ canEdit: boolean; canArchive: boolean }>;

/**
 * Settings › Data model: the workspace's tag vocabulary.
 *
 * The one door that coins a word. Applying an existing tag is every seat's,
 * and this card is only Admin's and Ops's, because a vocabulary anybody may
 * extend is not governed — it is a list of everything anybody ever typed.
 *
 * Every verb here is reversible except merge. Archiving retires a word and
 * leaves it on what already carries it; restoring offers it again. Merge folds
 * one word into another and releases the source's name, which is why it asks.
 */
export function TagVocabularyCard() {
  const t = useT();
  // The tab opens on ANY data-model read, so this card is mounted for a seat
  // holding `custom_field:read` and no tag grant at all. Asking anyway is a
  // request whose only possible answer is 403 — and a withheld card says so
  // rather than vanishing, which is the rail's own rule.
  const canRead = useCan("tag", "read");
  const canCreate = useCanWrite("tag", "create");
  const verbs: TagVerbs = {
    canEdit: useCanWrite("tag", "update"),
    canArchive: useCanWrite("tag", "delete"),
  };
  const catalog = useTagCatalog(canRead);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Tag | null>(null);
  const [merging, setMerging] = useState<Tag | null>(null);
  const words = catalog.data?.data ?? [];

  return (
    <Panel
      title={t("tagAdmin.title")}
      titleAction={
        canCreate && (
          <Button onClick={() => setAdding(true)}>{t("tagAdmin.add")}</Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("tagAdmin.sub")}</PanelIntro>
        {!canRead && <p className="tagadmin-note">{t("tagAdmin.withheld")}</p>}
        {/* The catalog is capped and carries no cursor. An admin shown a cut
            list would coin a duplicate of a word past the cap, and merge could
            not name it as a target. */}
        {canRead && catalog.data?.page.has_more && (
          <Callout
            tone="warning"
            kind="standing"
            title={t("tagAdmin.truncatedTitle")}
          >
            {t("tagAdmin.truncated")}
          </Callout>
        )}
      </PanelBody>
      {canRead &&
        (catalog.isSuccess && words.length > 0 ? (
          <TagTable
            words={words}
            verbs={verbs}
            onEdit={setEditing}
            onMerge={setMerging}
          />
        ) : (
          <PanelBody>
            <QueryStates query={catalog} pendingLabel={t("tagAdmin.title")}>
              <EmptyState>{t("tagAdmin.empty")}</EmptyState>
            </QueryStates>
          </PanelBody>
        ))}
      {adding && (
        <TagDialog vocabulary={words} onClose={() => setAdding(false)} />
      )}
      {editing && (
        <TagDialog
          existing={editing}
          vocabulary={words}
          onClose={() => setEditing(null)}
        />
      )}
      {merging && (
        <MergeDialog
          source={merging}
          vocabulary={words}
          onClose={() => setMerging(null)}
        />
      )}
    </Panel>
  );
}

function TagTable({
  words,
  verbs,
  onEdit,
  onMerge,
}: Readonly<{
  words: readonly Tag[];
  verbs: TagVerbs;
  onEdit: (tag: Tag) => void;
  onMerge: (tag: Tag) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const columns: DataTableColumn<Tag>[] = [
    {
      key: "tag",
      header: t("tagAdmin.colTag"),
      fold: "title",
      render: (tag) => <TagName tag={tag} />,
    },
    {
      // Retiring or merging a word many records carry is a different act
      // from retiring one nobody used, so the count sits beside the verbs.
      key: "usage",
      header: t("tagAdmin.colUsage"),
      align: "end",
      render: (tag) =>
        tag.carried_by === undefined
          ? "—"
          : plural("tagAdmin.usedBy", tag.carried_by, {
              count: formatNumber(tag.carried_by, locale),
            }),
    },
  ];
  if (verbs.canEdit || verbs.canArchive) {
    columns.push({
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (tag) => (
        <TagMenu
          tag={tag}
          verbs={verbs}
          onEdit={() => onEdit(tag)}
          onMerge={() => onMerge(tag)}
        />
      ),
    });
  }
  return (
    <DataTable
      bleed
      fold
      label={t("tagAdmin.listLabel")}
      columns={columns}
      rows={[...words]}
      rowKey={(tag) => tag.id}
      rowTestId={(tag) => `tag-${tag.id}`}
    />
  );
}

// A retired word keeps its place so it can be restored, and drops its colour
// because it is no longer offered.
function TagName({ tag }: Readonly<{ tag: Tag }>) {
  const t = useT();
  const archived = Boolean(tag.archived_at);
  return (
    <span className="tagadmin-name">
      <TagPill name={tag.name} tone={archived ? null : tag.color} />
      {archived && <Badge>{t("tagAdmin.retired")}</Badge>}
    </span>
  );
}

function TagMenu({
  tag,
  verbs,
  onEdit,
  onMerge,
}: Readonly<{
  tag: Tag;
  verbs: TagVerbs;
  onEdit: () => void;
  onMerge: () => void;
}>) {
  const t = useT();
  const { archive, restore } = useTagLifecycle(tag.name);
  const archived = Boolean(tag.archived_at);
  const edits = verbs.canEdit && !archived;
  if (!edits && !verbs.canArchive) {
    return null;
  }
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: tag.name })}>
        {edits && (
          <Button aria-haspopup="dialog" onClick={onEdit}>
            {t("tagAdmin.edit")}
          </Button>
        )}
        {edits && (
          <Button aria-haspopup="dialog" onClick={onMerge}>
            {t("tagAdmin.merge")}
          </Button>
        )}
        {verbs.canArchive &&
          (archived ? (
            <Button
              disabled={restore.isPending}
              onClick={() => restore.mutate(tag.id)}
            >
              {t("tagAdmin.restore")}
            </Button>
          ) : (
            <Button
              disabled={archive.isPending}
              onClick={() => archive.mutate(tag.id)}
            >
              {t("tagAdmin.archive")}
            </Button>
          ))}
      </OverflowMenu>
    </span>
  );
}

// Retiring runs at once and offers Undo, because restoring is the exact inverse.
function useTagLifecycle(name: string) {
  const t = useT();
  const toasts = useUndoableRemoval<string>({
    removed: t("tagAdmin.retiredToast", { name }),
    restored: t("tagAdmin.restoredToast", { name }),
  });
  const restore = useRestoreTag(toasts.restored);
  const archive = useArchiveTag(toasts.removed((id) => restore.mutate(id)));
  return { archive, restore };
}

/**
 * Folding one word into another.
 *
 * The one verb here that cannot be undone, and it says so. The source's name
 * is RELEASED — somebody can coin it again tomorrow, and the new word will
 * carry none of the records this one did, which is the part an admin does not
 * expect and the reason this dialog spells it out.
 */
function MergeDialog({
  source,
  vocabulary,
  onClose,
}: Readonly<{
  source: Tag;
  vocabulary: readonly Tag[];
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [target, setTarget] = useState("");
  const merge = useMergeTags();
  const result = merge.data;
  // A live word other than this one: the endpoint refuses an archived target
  // and refuses merging a tag into itself, and offering either would be a
  // control whose only outcome is a refusal.
  const candidates = vocabulary.filter(
    (tag) => tag.id !== source.id && !tag.archived_at,
  );

  if (result) {
    return (
      <Modal
        open
        onClose={onClose}
        labelledBy="tagadmin-merged"
        intent="confirm"
      >
        <Heading size="large" id="tagadmin-merged" className="t-h2 modal-title">
          {t("tagAdmin.mergedTitle")}
        </Heading>
        {/* Moved and collapsed are counted apart because they are different
            facts: a record that carried only the source now carries the
            target, while a record that carried both simply loses a duplicate
            — and adding them would report records the target did not gain. */}
        <p>
          {t("tagAdmin.mergedBody", {
            moved: formatNumber(result.moved, locale),
            collapsed: formatNumber(result.collapsed, locale),
          })}
        </p>
        <div className="actions">
          <Button onClick={onClose}>{t("tagAdmin.done")}</Button>
        </div>
      </Modal>
    );
  }

  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("tagAdmin.mergeTitle", { name: source.name })}
      confirmLabel={t("tagAdmin.mergeConfirm")}
      confirmVariant="danger"
      confirmDisabled={target === ""}
      pending={merge.isPending}
      error={merge.error != null ? problemMessageOf(merge.error, t) : undefined}
      onConfirm={() => merge.mutate({ id: source.id, intoTagID: target })}
    >
      <Field label={t("tagAdmin.mergeIntoLabel")}>
        {(control) => (
          <Select
            {...control}
            value={target}
            onChange={(next) => setTarget(next)}
            options={[
              { value: "", label: t("tagAdmin.mergeIntoNone") },
              ...candidates.map((tag) => ({ value: tag.id, label: tag.name })),
            ]}
          />
        )}
      </Field>
      {/* `danger` and not `warning`: the act is irreversible, which is what the
          dialog's own confirm button says with its tone. */}
      <Callout
        tone="danger"
        kind="standing"
        title={t("tagAdmin.mergeWarningTitle")}
      >
        {t("tagAdmin.mergeWarning", { name: source.name })}
      </Callout>
    </ConfirmModal>
  );
}
