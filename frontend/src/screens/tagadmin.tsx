// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";

import { requireVersion } from "../api/version";
import { useCanWrite } from "../app/capability";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { TagPill } from "../design-system/tagpill";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, QueryGate } from "./common";
import { nearMatches } from "./tagadmin.logic";
import type { Tag, TagColor } from "./tagadmin.queries";
import {
  useArchiveTag,
  useCreateTag,
  useMergeTags,
  useRestoreTag,
  useTagCatalog,
  useTagDetail,
  useUpdateTag,
} from "./tagadmin.queries";
import "./tagadmin.css";

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
  const canCreate = useCanWrite("tag", "create");
  const canEdit = useCanWrite("tag", "update");
  const canArchive = useCanWrite("tag", "delete");
  const catalog = useTagCatalog();
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Tag | null>(null);
  const [merging, setMerging] = useState<Tag | null>(null);
  const archive = useArchiveTag();
  const restore = useRestoreTag();
  // Archive and restore have nowhere else to speak: they are one press with no
  // dialog of their own. Create, edit and merge each carry their refusal in
  // the dialog that asked for it, and a sentence in both places reads as two
  // separate failures.
  const failure = [archive, restore].find((m) => m.isError);

  return (
    <Panel
      title={t("tagAdmin.title")}
      titleAction={
        canCreate && (
          <Button small onClick={() => setAdding(true)}>
            {t("tagAdmin.add")}
          </Button>
        )
      }
    >
      <PanelBody>
        <p className="settings-panel-sub">{t("tagAdmin.sub")}</p>
        {failure?.error != null && (
          <Callout tone="danger">{problemMessageOf(failure.error, t)}</Callout>
        )}
        <SettingList>
          <SettingRow
            label={t("tagAdmin.listLabel")}
            layout="stack"
            control={
              <QueryGate query={catalog}>
                {(answer) => {
                  const words = answer.data ?? [];
                  if (words.length === 0) {
                    return (
                      <p className="tagadmin-note">{t("tagAdmin.empty")}</p>
                    );
                  }
                  return (
                    <ul className="tagadmin-list" data-testid="tag-vocabulary">
                      {words.map((tag) => (
                        <TagVocabularyRow
                          key={tag.id}
                          tag={tag}
                          canEdit={canEdit}
                          canArchive={canArchive}
                          onEdit={() => setEditing(tag)}
                          onMerge={() => setMerging(tag)}
                          onArchive={() => archive.mutate(tag.id)}
                          onRestore={() => restore.mutate(tag.id)}
                        />
                      ))}
                    </ul>
                  );
                }}
              </QueryGate>
            }
          />
        </SettingList>
      </PanelBody>
      {adding && (
        <TagDialog
          vocabulary={catalog.data?.data ?? []}
          onClose={() => setAdding(false)}
        />
      )}
      {editing && (
        <TagDialog
          existing={editing}
          vocabulary={catalog.data?.data ?? []}
          onClose={() => setEditing(null)}
        />
      )}
      {merging && (
        <MergeDialog
          source={merging}
          vocabulary={catalog.data?.data ?? []}
          onClose={() => setMerging(null)}
        />
      )}
    </Panel>
  );
}

/**
 * One word, with how much of the workspace carries it and the verbs it takes.
 *
 * The usage count is what makes archiving and merging decisions rather than
 * guesses: retiring a word twelve hundred records carry is a different act
 * from retiring one nobody used.
 */
function TagVocabularyRow({
  tag,
  canEdit,
  canArchive,
  onEdit,
  onMerge,
  onArchive,
  onRestore,
}: Readonly<{
  tag: Tag;
  canEdit: boolean;
  canArchive: boolean;
  onEdit: () => void;
  onMerge: () => void;
  onArchive: () => void;
  onRestore: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const detail = useTagDetail(tag.id);
  const usage = detail.data?.usage;
  const carried =
    usage === undefined
      ? undefined
      : usage.people + usage.companies + usage.deals;
  const archived = Boolean(tag.archived_at);

  return (
    <li className="tagadmin-row">
      <TagPill name={tag.name} tone={tag.color} archived={archived} />
      <span className="tagadmin-usage t-small">
        {carried === undefined
          ? // Not "0": the count has not arrived, and a zero drawn while it is
            // in flight is a claim about the vocabulary nobody made.
            t("tagAdmin.usagePending")
          : t("tagAdmin.usage", { count: formatNumber(carried, locale) })}
      </span>
      <span className="tagadmin-verbs">
        {canEdit && !archived && (
          <Button small variant="ghost" onClick={onEdit}>
            {t("tagAdmin.edit")}
          </Button>
        )}
        {canEdit && !archived && (
          <Button small variant="ghost" onClick={onMerge}>
            {t("tagAdmin.merge")}
          </Button>
        )}
        {canArchive &&
          (archived ? (
            <Button small variant="ghost" onClick={onRestore}>
              {t("tagAdmin.restore")}
            </Button>
          ) : (
            <Button small variant="ghost" onClick={onArchive}>
              {t("tagAdmin.archive")}
            </Button>
          ))}
      </span>
    </li>
  );
}

const PALETTE: readonly TagColor[] = ["teal", "amber", "rose", "slate"];

/**
 * Coining a word, or correcting one.
 *
 * The near-match warning is a warning and not a refusal: an admin coining "EV"
 * beside "EV programme" may mean exactly that, and a rule that stopped them
 * would be this tier deciding what the vocabulary may contain. What it prevents
 * is the accident — the admin who looked for a word, did not see it, and is one
 * press away from a second spelling of one that already exists.
 */
function TagDialog({
  existing,
  vocabulary,
  onClose,
}: Readonly<{
  existing?: Tag;
  vocabulary: readonly Tag[];
  onClose: () => void;
}>) {
  const t = useT();
  const [name, setName] = useState(existing?.name ?? "");
  const [color, setColor] = useState<string>(existing?.color ?? "");
  const create = useCreateTag();
  const update = useUpdateTag();
  const pending = create.isPending || update.isPending;
  const failure = create.error ?? update.error;
  // Never the word being edited: a rename that keeps most of its own spelling
  // would otherwise warn that it collides with itself.
  const others = vocabulary.filter((tag) => tag.id !== existing?.id);
  const near = nearMatches(name, others);

  const submit = () => {
    const trimmed = name.trim();
    if (trimmed === "") {
      return;
    }
    if (existing) {
      update.mutate(
        {
          id: existing.id,
          // Never a default: a row read back without a version takes no write,
          // because an unpinned PATCH is last-write-wins and lands on top of an
          // edit it never saw while reporting success to both editors.
          version: requireVersion(existing.version),
          name: trimmed,
          // "none" clears, because an absent field and a null field decode to
          // the same thing in the request type.
          color: color === "" ? "none" : (color as TagColor),
        },
        { onSuccess: onClose },
      );
      return;
    }
    create.mutate(
      { name: trimmed, color: color === "" ? undefined : (color as TagColor) },
      { onSuccess: onClose },
    );
  };

  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={existing ? t("tagAdmin.editTitle") : t("tagAdmin.addTitle")}
      confirmLabel={existing ? t("tagAdmin.save") : t("tagAdmin.create")}
      confirmDisabled={name.trim() === ""}
      pending={pending}
      error={failure != null ? problemMessageOf(failure, t) : undefined}
      onConfirm={submit}
    >
      <Field label={t("tagAdmin.nameLabel")}>
        {(control) => (
          <TextInput
            {...control}
            value={name}
            maxLength={64}
            onChange={(event) => setName(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("tagAdmin.colorLabel")}>
        {(control) => (
          <Select
            {...control}
            value={color}
            onChange={(next) => setColor(next)}
            options={[
              { value: "", label: t("tagAdmin.colorNone") },
              ...PALETTE.map((tone) => ({ value: tone, label: tone })),
            ]}
          />
        )}
      </Field>
      {near.length > 0 && (
        <Callout tone="warn">
          {t("tagAdmin.nearMatch", {
            names: near.map((tag) => tag.name).join(", "),
          })}
        </Callout>
      )}
    </ConfirmModal>
  );
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
      <Modal open onClose={onClose} labelledBy="tagadmin-merged">
        <div className="tagadmin-merged">
          <h2 id="tagadmin-merged">{t("tagAdmin.mergedTitle")}</h2>
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
      <Callout tone="warn">
        {t("tagAdmin.mergeWarning", { name: source.name })}
      </Callout>
    </ConfirmModal>
  );
}
