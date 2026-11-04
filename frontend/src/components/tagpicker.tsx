// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Plus } from "lucide-react";
import { useMemo, useState } from "react";
import { Button } from "../design-system/atoms";
import {
  ListPopover,
  type ListPopoverOption,
} from "../design-system/listpopover";
import { TagPill } from "../design-system/tagpill";
import { useT } from "../i18n";
import { useCan } from "../lib/capability";
import { useListsAvailable } from "../lib/lists.queries";
import { problemMessageOf } from "../lib/problem";
import type { RecordTag, TaggableType } from "../lib/tags.queries";
import {
  useApplyTag,
  useCreateOrFindTag,
  useTagVocabulary,
} from "../lib/tags.queries";
import { AddToShortlistDialog } from "./addtoshortlist";
import type { PickedTag } from "./bulktag";
import "./tagpicker.css";
import { CreateTagWay, TagListFooter } from "./tagwayforward";

/**
 * The add-tag verb: pick a word the workspace already has.
 *
 * The vocabulary is governed: only Admin and Ops coin a word (#3660). So what
 * a typed word that matches nothing offers depends on the seat:
 * - a seat that may create tags gets "Create tag {name}", which coins the word
 *   and puts it on this record in one step, without a trip to Settings;
 * - every other seat is told who can add one, and is offered a Shortlist for
 *   a group of records only they need, named with the word they typed.
 *
 * The footer says the same whatever the box holds, because a reader who
 * scrolls a short list and closes it never types the word that would tell
 * them the list can grow.
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
  const create = useCreateOrFindTag();
  const canCreate = useCan("tag", "create");
  const listsOn = useListsAvailable();
  const canMakeList = useCan("list", "create");
  const canShortlist = listsOn && canMakeList;
  // The name a new Shortlist opens with, once the reader takes that way out.
  const [shortlistName, setShortlistName] = useState<string | null>(null);
  // The word this picker created and then failed to put on the record. The
  // two writes are separate, so the reader is told the tag exists now.
  const [unapplied, setUnapplied] = useState<string | null>(null);
  const writing = apply.isPending || create.isPending;
  const failure = create.error ?? apply.error;
  const failureText = failure
    ? unapplied
      ? t("tags.createdNotApplied", {
          name: unapplied,
          reason: problemMessageOf(failure, t),
        })
      : problemMessageOf(failure, t)
    : undefined;

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

  // Coin the word, or find it when a stale list offered Create for a word that
  // exists, then put it on this record. A refused apply leaves the word in
  // the vocabulary, where the list now offers it to pick again.
  const createAndApply = (name: string, done: () => void) =>
    create.mutate(name, {
      onSuccess: (tag) =>
        apply.mutate(tag.id, {
          onSuccess: () => {
            done();
            onApplied?.({ id: tag.id, name: tag.name });
          },
          onError: () => setUnapplied(tag.name),
        }),
    });

  return (
    <>
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
          if (!next && writing) {
            return;
          }
          setOpen(next);
          // A refusal belongs to the attempt it answered, not to the next opening.
          if (!writing) {
            apply.reset();
            create.reset();
            setUnapplied(null);
          }
        }}
        options={vocabulary.isError ? [] : options}
        empty={
          vocabulary.isError
            ? problemMessageOf(vocabulary.error, t)
            : canCreate
              ? t("tags.noMatchYouCanCreate")
              : t("tags.noMatch")
        }
        emptyAction={
          vocabulary.isError
            ? undefined
            : (term, done) =>
                canCreate ? (
                  <CreateTagWay
                    term={term}
                    disabled={writing}
                    onCreate={() => createAndApply(term, done)}
                  />
                ) : (
                  canShortlist && (
                    <Button
                      className="tagpicker-way"
                      variant="link"
                      onClick={() => {
                        done();
                        setShortlistName(term);
                      }}
                    >
                      {t("tags.shortlistInstead")}
                    </Button>
                  )
                )
        }
        pending={writing}
        error={failureText}
        onPick={(option, done) => {
          setUnapplied(null);
          apply.mutate(option.id, {
            onSuccess: () => {
              done();
              onApplied?.({ id: option.id, name: option.name });
            },
          });
        }}
        footer={
          <TagListFooter
            canCreate={canCreate}
            truncated={vocabulary.data?.truncated ?? false}
          />
        }
      />
      {shortlistName !== null && (
        <AddToShortlistDialog
          entityType={entityType}
          entityId={entityID}
          newName={shortlistName}
          onClose={() => setShortlistName(null)}
        />
      )}
    </>
  );
}
