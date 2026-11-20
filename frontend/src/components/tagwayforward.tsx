// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Plus } from "lucide-react";
import type { ReactNode } from "react";
import { AppLink } from "../design-system/applink";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import "./tagpicker.css";

// What the record tag picker and the bulk tag picker offer past the words the
// workspace has. Both pickers draw these, so a seat sees one way forward.

const TAGS_SETTINGS_HREF = routeHash(settingsHref("records", "tags"));

/**
 * The footer under a tag list, the same whatever the box holds: a reader who
 * scrolls a short list never types the word that would say the list can grow.
 */
export function TagListFooter({
  canCreate,
  truncated,
}: Readonly<{ canCreate: boolean; truncated: boolean }>) {
  const t = useT();
  return (
    <>
      {/* The catalog was cut. Say so, because a reader who cannot find a
          word in a SHORT list concludes the workspace lacks it and asks an
          admin to coin the duplicate this picker exists to prevent. */}
      {truncated && (
        <Callout kind="standing" title={t("tags.catalogTruncatedTitle")}>
          {t("tags.catalogTruncated")}
        </Callout>
      )}
      <p className="tagpicker-foot t-caption">
        {canCreate ? (
          <AppLink href={TAGS_SETTINGS_HREF}>{t("tags.manage")}</AppLink>
        ) : (
          t("tags.whoAdds")
        )}
      </p>
    </>
  );
}

/**
 * What a tag list with no rows says. A failed read gives its reason. Otherwise
 * the list holds no tags yet, or no tag carries the typed name.
 */
export function useEmptyTagWords(
  vocabulary: Readonly<{ isError: boolean; error: unknown }>,
  canCreate: boolean,
): Readonly<{ empty: ReactNode; none: ReactNode | undefined }> {
  const t = useT();
  if (vocabulary.isError) {
    return { empty: problemMessageOf(vocabulary.error, t), none: undefined };
  }
  return canCreate
    ? { empty: t("tags.noMatchYouCanCreate"), none: t("tags.noneYouCanCreate") }
    : { empty: t("tags.noMatch"), none: t("tags.noneAskForOne") };
}

/** "Create tag {name}", for a seat that may coin a word. */
export function CreateTagWay({
  term,
  disabled,
  onCreate,
}: Readonly<{ term: string; disabled: boolean; onCreate: () => void }>) {
  const t = useT();
  return (
    <Button
      className="tagpicker-way"
      variant="ghost"
      disabled={disabled}
      onClick={onCreate}
    >
      <Plus aria-hidden /> {t("tags.createNamed", { name: term })}
    </Button>
  );
}
