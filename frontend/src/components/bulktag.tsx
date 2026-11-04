// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Add tag" and "Remove tag" over a selection: pick one of the workspace's live
// tags, then the verb, and the bulk dialog previews and confirms the change
// like any other bulk change.

import { useMemo, useState } from "react";
import { Button } from "../design-system/atoms";
import {
  ListPopover,
  type ListPopoverOption,
} from "../design-system/listpopover";
import { TagPill } from "../design-system/tagpill";
import { useT } from "../i18n";
import { useCan } from "../lib/capability";
import { problemMessageOf } from "../lib/problem";
import { useCreateOrFindTag, useTagVocabulary } from "../lib/tags.queries";
import { CreateTagWay, TagListFooter } from "./tagwayforward";

export type PickedTag = Readonly<{ id: string; name: string }>;

/**
 * The tag a bulk change carries, picked from the same list the record picker
 * shows, with the same way forward when a word is missing. A seat that may
 * coin a word creates it here and it becomes the pick; the change itself still
 * waits for the verb and the preview.
 */
export function TagVerbs({
  disabled,
  onPick,
}: Readonly<{
  disabled: boolean;
  onPick: (verb: "add_tag" | "remove_tag", tag: PickedTag) => void;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const vocabulary = useTagVocabulary();
  const create = useCreateOrFindTag();
  const canCreate = useCan("tag", "create");
  const [picked, setPicked] = useState<PickedTag | null>(null);
  const options = useMemo(
    () =>
      vocabulary.data?.tags.map(
        (tag): ListPopoverOption => ({
          id: tag.id,
          name: tag.name,
          face: <TagPill name={tag.name} tone={tag.color} />,
        }),
      ),
    [vocabulary.data],
  );
  const press = (verb: "add_tag" | "remove_tag") => {
    if (picked) {
      onPick(verb, picked);
    }
  };
  return (
    <>
      <ListPopover
        label={picked?.name ?? t("bulk.tagPick")}
        title={t("bulk.tag")}
        searchLabel={t("tags.pickerLabel")}
        variant="ghost"
        open={open}
        onOpenChange={(next) => {
          if (!next && create.isPending) {
            return;
          }
          setOpen(next);
          if (!create.isPending) {
            create.reset();
          }
        }}
        selected={picked?.id}
        options={vocabulary.isError ? [] : options}
        empty={
          vocabulary.isError
            ? problemMessageOf(vocabulary.error, t)
            : canCreate
              ? t("tags.noMatchYouCanCreate")
              : t("tags.noMatch")
        }
        emptyAction={
          canCreate && !vocabulary.isError
            ? (term, done) => (
                <CreateTagWay
                  term={term}
                  disabled={disabled || create.isPending}
                  onCreate={() =>
                    create.mutate(term, {
                      onSuccess: (tag) => {
                        setPicked({ id: tag.id, name: tag.name });
                        done();
                      },
                    })
                  }
                />
              )
            : undefined
        }
        pending={create.isPending}
        error={create.error ? problemMessageOf(create.error, t) : undefined}
        onPick={(option, done) => {
          setPicked({ id: option.id, name: option.name });
          done();
        }}
        footer={
          <TagListFooter
            canCreate={canCreate}
            truncated={vocabulary.data?.truncated ?? false}
          />
        }
      />
      <Button disabled={disabled || !picked} onClick={() => press("add_tag")}>
        {t("bulk.addTag")}
      </Button>
      <Button
        disabled={disabled || !picked}
        onClick={() => press("remove_tag")}
      >
        {t("bulk.removeTag")}
      </Button>
    </>
  );
}
