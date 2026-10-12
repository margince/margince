// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";

import { Checkbox, Field, Textarea, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select, type SelectOption } from "../design-system/select";
import { isTagTone, TAG_TONES } from "../design-system/tagpill";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import { nearMatches } from "./tagadmin.logic";
import type { Tag, TagColor } from "./tagadmin.queries";
import { useCreateTag, useUpdateTag } from "./tagadmin.queries";

// The dialog that coins a tag or corrects one, from Settings › Data model.

/** How many close words a warning names before it stops listing them. */
const NEAR_MATCHES_NAMED = 5;

/**
 * A Select's answer as a colour, or none.
 *
 * The control hands back a string. Asserted, a value the palette does not hold
 * would reach the API and be refused. The reader could not act on that refusal.
 */
function asTagColor(value: string): TagColor | "" {
  return isTagTone(value) ? value : "";
}

/**
 * The colour options, each carrying the same dot the tag itself will draw.
 *
 * An admin picks by the swatch, because the tone names mean nothing to them. The
 * swatch is decorative, so the label still names the colour.
 */
function colorOptions(t: ReturnType<typeof useT>): readonly SelectOption[] {
  return [
    { value: "", label: t("tagAdmin.colorNone") },
    ...TAG_TONES.map((tone) => ({
      value: tone,
      label: t(`tagAdmin.color.${tone}`),
      adornment: (
        <span className={`tagpill-dot tagpill-dot-${tone}`} aria-hidden />
      ),
    })),
  ];
}

/**
 * Coining a word, or correcting one.
 *
 * The near-match warning does not refuse. An admin coining "EV" beside "EV
 * programme" may mean both. The warning catches the admin who missed an
 * existing word and is about to coin a second spelling of it.
 */
export function TagDialog({
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
  // Narrowed, not asserted: a string outside the palette would reach the API as
  // a colour the server refuses.
  const [color, setColor] = useState<TagColor | "">(existing?.color ?? "");
  const [description, setDescription] = useState(existing?.description ?? "");
  const [suggestible, setSuggestible] = useState(
    existing?.suggestible ?? false,
  );
  const create = useCreateTag();
  const update = useUpdateTag();
  const pending = create.isPending || update.isPending;
  const failure = create.error ?? update.error;
  // A row that came back with no version cannot be written. It gets its own
  // sentence, because the generic failure gives no way to fix it. Reopening
  // the page does.
  const unversioned = existing !== undefined && existing.version === undefined;
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
          version: existing.version,
          name: trimmed,
          // "none" clears, because an absent field and a null field decode to
          // the same thing in the request type.
          color: color === "" ? "none" : color,
          description: description.trim(),
          suggestible,
        },
        { onSuccess: onClose },
      );
      return;
    }
    create.mutate(
      { name: trimmed, color: color === "" ? undefined : color },
      { onSuccess: onClose },
    );
  };

  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={existing ? t("tagAdmin.editTitle") : t("tagAdmin.addTitle")}
      intent="form"
      confirmLabel={existing ? t("tagAdmin.save") : t("tagAdmin.create")}
      confirmDisabled={
        name.trim() === "" ||
        unversioned ||
        undescribedSuggestion(suggestible, description)
      }
      pending={pending}
      error={
        unversioned
          ? t("tagAdmin.noVersion")
          : failure != null
            ? problemMessageOf(failure, t)
            : undefined
      }
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
            onChange={(next) => setColor(asTagColor(next))}
            options={colorOptions(t)}
          />
        )}
      </Field>
      {/* Suggestions are switched on for a word that exists, so a new word is
          coined first and described when it is edited. */}
      {existing && (
        <SuggestionFields
          description={description}
          onDescription={setDescription}
          suggestible={suggestible}
          onSuggestible={setSuggestible}
        />
      )}
      {near.length > 0 && (
        <Callout
          tone="warning"
          kind="standing"
          title={t("tagAdmin.nearMatchTitle")}
        >
          {t("tagAdmin.nearMatch", {
            // Capped: a warning naming forty words is one an admin scrolls
            // past, which costs the near-duplicate it exists to catch.
            names: near
              .slice(0, NEAR_MATCHES_NAMED)
              .map((tag) => tag.name)
              .join(", "),
          })}
        </Callout>
      )}
    </ConfirmModal>
  );
}

// The server refuses a suggested tag with no description to match against, so
// the dialog does too.
function undescribedSuggestion(suggestible: boolean, description: string) {
  return suggestible && description.trim() === "";
}

/** What interest in a tag looks like, and whether captured mail may suggest it. */
function SuggestionFields({
  description,
  onDescription,
  suggestible,
  onSuggestible,
}: Readonly<{
  description: string;
  onDescription: (next: string) => void;
  suggestible: boolean;
  onSuggestible: (next: boolean) => void;
}>) {
  const t = useT();
  return (
    <>
      <Field
        label={t("tagAdmin.descriptionLabel")}
        hint={t("tagAdmin.descriptionHint")}
      >
        {(control) => (
          <Textarea
            {...control}
            value={description}
            rows={3}
            onChange={(event) => onDescription(event.target.value)}
          />
        )}
      </Field>
      <Checkbox
        label={t("tagAdmin.suggestibleLabel")}
        checked={suggestible}
        onChange={(event) => onSuggestible(event.currentTarget.checked)}
      />
    </>
  );
}
