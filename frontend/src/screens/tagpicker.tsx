// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Plus } from "lucide-react";
import { useMemo, useState } from "react";

import { Callout } from "../design-system/callout";
import {
  ListPopover,
  type ListPopoverOption,
} from "../design-system/listpopover";
import { TagPill } from "../design-system/tagpill";
import { useT } from "../i18n";
import type { PickedTag } from "./bulktag";
import { problemMessageOf } from "./common";
import type { RecordTag, TaggableType } from "./tags.queries";
import { useApplyTag, useTagVocabulary } from "./tags.queries";

/**
 * The add-tag verb: pick a word the workspace already has.
 *
 * It cannot create one. The vocabulary is governed — only Admin and Ops coin a
 * word — so a picker that minted one on a name it did not recognise would hand
 * every seat the authority the governance exists to withhold, and a
 * misspelling would become a permanent second tag nobody chose.
 *
 * When nothing matches, the list says who CAN add one. That is the whole of
 * the no-match path: there is no request flow, because a rep who needs a word
 * asks an admin the way they would ask for anything else.
 */
export function AddTagPicker({
  entityType,
  entityID,
  current,
  onApplied,
}: Readonly<{
  entityType: TaggableType;
  entityID: string;
  /** What the record already carries, so the list can say so rather than
   * offering a word twice and answering the second try with a conflict. */
  current: readonly RecordTag[];
  /** Told which word landed, so the host can offer to carry it further. */
  onApplied?: (tag: PickedTag) => void;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  // Read on opening: the verb sits on every record a seat may write, and most
  // visits never open it.
  const vocabulary = useTagVocabulary(open);
  const apply = useApplyTag(entityType, entityID);

  const applied = useMemo(
    () => new Set(current.map((tag) => tag.tag_id)),
    [current],
  );
  // The whole vocabulary, in the server's order: a "recently used" section
  // would need a per-seat history nothing records yet.
  const options = useMemo(
    () =>
      vocabulary.data?.tags.map((tag): ListPopoverOption => {
        const already = applied.has(tag.id);
        return {
          id: tag.id,
          name: tag.name,
          face: <TagPill name={tag.name} tone={tag.color} />,
          hint: already ? t("tags.alreadyAdded") : undefined,
          disabled: already,
        };
      }),
    [vocabulary.data, applied, t],
  );

  return (
    <ListPopover
      label={
        <>
          <Plus aria-hidden /> {t("tags.add")}
        </>
      }
      title={t("tags.add")}
      searchLabel={t("tags.pickerLabel")}
      open={open}
      onOpenChange={(next) => {
        // Held open while the write is out, so its refusal has somewhere to land.
        if (!next && apply.isPending) {
          return;
        }
        setOpen(next);
        // A refusal belongs to the attempt it answered, not to the next opening.
        if (!apply.isPending) {
          apply.reset();
        }
      }}
      options={vocabulary.isError ? [] : options}
      empty={
        vocabulary.isError
          ? problemMessageOf(vocabulary.error, t)
          : t("tags.noMatch")
      }
      pending={apply.isPending}
      error={apply.isError ? problemMessageOf(apply.error, t) : undefined}
      onPick={(option, done) =>
        apply.mutate(option.id, {
          onSuccess: () => {
            done();
            onApplied?.({ id: option.id, name: option.name });
          },
        })
      }
      footer={
        // The catalog was cut. Say so, because a reader who cannot find a word
        // in a SHORT list concludes the workspace lacks it and asks an admin
        // to coin the duplicate this picker exists to prevent.
        vocabulary.data?.truncated && (
          <Callout kind="standing" title={t("tags.catalogTruncatedTitle")}>
            {t("tags.catalogTruncated")}
          </Callout>
        )
      }
    />
  );
}
