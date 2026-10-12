// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Add tag" and "Remove tag" over a selection: pick one of the workspace's live
// tags, then the verb, and the bulk dialog previews and confirms the change
// like any other bulk change.

import { useState } from "react";
import { Button } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useTagVocabulary } from "./tags.queries";

export type PickedTag = Readonly<{ id: string; name: string }>;

export function TagVerbs({
  disabled,
  onPick,
}: Readonly<{
  disabled: boolean;
  onPick: (verb: "add_tag" | "remove_tag", tag: PickedTag) => void;
}>) {
  const t = useT();
  const vocabulary = useTagVocabulary();
  const [tagId, setTagId] = useState("");
  const tags = vocabulary.data?.tags ?? [];
  const picked = tags.find((tag) => tag.id === tagId);
  if (tags.length === 0) {
    return null;
  }
  const press = (verb: "add_tag" | "remove_tag") => {
    if (picked) {
      onPick(verb, { id: picked.id, name: picked.name });
    }
  };
  return (
    <>
      <Select
        aria-label={t("bulk.tag")}
        value={tagId}
        placeholder={t("bulk.tagPick")}
        disabled={disabled}
        onChange={setTagId}
        options={tags.map((tag) => ({ value: tag.id, label: tag.name }))}
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
