// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useMemo, useState } from "react";

import { Button, Modal, SearchField } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { TagPill } from "../design-system/tagpill";
import { useT } from "../i18n";
import type { RecordTag, TaggableType } from "./tags.queries";
import { useApplyTag, useTagVocabulary } from "./tags.queries";
import "./tagpicker.css";

/**
 * The add-tag dialog: pick a word the workspace already has.
 *
 * It cannot create one. The vocabulary is governed — only Admin and Ops coin a
 * word — so a picker that minted one on a name it did not recognise would hand
 * every seat the authority the governance exists to withhold, and a
 * misspelling would become a permanent second tag nobody chose.
 *
 * When nothing matches, the dialog says who CAN add one. That is the whole of
 * the no-match path: there is no request flow, because a rep who needs a word
 * asks an admin the way they would ask for anything else.
 */
export function AddTagDialog({
  entityType,
  entityID,
  current,
  onClose,
}: Readonly<{
  entityType: TaggableType;
  entityID: string;
  /** What the record already carries, so the list can say so rather than
   * offering a word twice and answering the second try with a conflict. */
  current: readonly RecordTag[];
  onClose: () => void;
}>) {
  const t = useT();
  const titleID = useId();
  const [query, setQuery] = useState("");
  const vocabulary = useTagVocabulary();
  const apply = useApplyTag(entityType, entityID);

  const normalized = query.trim().toLowerCase();
  const applied = useMemo(
    () => new Set(current.map((tag) => tag.tag_id)),
    [current],
  );

  const matches = useMemo(() => {
    const all = vocabulary.data?.tags ?? [];
    if (normalized === "") {
      // Nothing typed: the whole vocabulary, which for a workspace-sized word
      // list is the useful default. A "recently used" section would need a
      // per-seat history nothing records yet, and inventing an order from the
      // words themselves would be a ranking nobody asked for.
      return all;
    }
    return all.filter((tag) => tag.name.toLowerCase().includes(normalized));
  }, [vocabulary.data, normalized]);

  return (
    <Modal open onClose={onClose} labelledBy={titleID}>
      <div className="tagpicker">
        <h2 id={titleID} className="tagpicker-title">
          {t("tags.add")}
        </h2>
        <SearchField
          aria-label={t("tags.pickerLabel")}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <div className="tagpicker-list">
          {matches.map((tag) => {
            const already = applied.has(tag.id);
            return (
              <Button
                key={tag.id}
                variant="ghost"
                className="tagpicker-option"
                disabled={already || apply.isPending}
                onClick={() => {
                  apply.mutate(tag.id, { onSuccess: onClose });
                }}
              >
                <TagPill name={tag.name} tone={tag.color} />
                {already && <span>{t("tags.alreadyAdded")}</span>}
              </Button>
            );
          })}
        </div>
        {normalized !== "" && matches.length === 0 && (
          <Callout>{t("tags.noMatch")}</Callout>
        )}
        {/* The catalog was cut. Say so, because a reader who cannot find a word
            in a SHORT list concludes the workspace lacks it and asks an admin
            to coin the duplicate this dialog exists to prevent. */}
        {vocabulary.data?.truncated && (
          <Callout>{t("tags.catalogTruncated")}</Callout>
        )}
      </div>
    </Modal>
  );
}
