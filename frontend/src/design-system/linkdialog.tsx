// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useState } from "react";
import { useT } from "../i18n";
import { Button, Field, TextInput } from "./atoms";
import { ConfirmModal } from "./confirmmodal";
import { linkHref } from "./richtext-html";

/**
 * The address of a link in written text. A bare host gets `https://` and a bare
 * mailbox `mailto:`; any other scheme is refused on the field.
 */
export function LinkDialog({
  open,
  href,
  onApply,
  onRemove,
  onClose,
  returnFocusTo,
}: Readonly<{
  open: boolean;
  /** The link the selection is in; empty for a new one. */
  href: string;
  /** The href the typed address stands for, always one `safeHref` keeps. */
  onApply: (href: string) => void;
  /** Takes the link off its words; given only while editing one. */
  onRemove?: () => void;
  onClose: () => void;
  /** The text the link is in, which takes focus back on close. */
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const t = useT();
  const [draft, setDraft] = useState(href);
  const [refused, setRefused] = useState(false);
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) {
      setDraft(href);
      setRefused(false);
    }
  }
  const apply = () => {
    const next = linkHref(draft);
    if (next === "") {
      setRefused(true);
      return;
    }
    onApply(next);
  };
  const editing = href !== "";
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      returnFocusTo={returnFocusTo}
      title={t(editing ? "richtext.linkEdit" : "richtext.linkAdd")}
      intent="form"
      confirmLabel={t("richtext.linkApply")}
      onConfirm={apply}
      actionsLead={
        onRemove && (
          <Button variant="ghost" onClick={onRemove}>
            {t("richtext.linkRemove")}
          </Button>
        )
      }
    >
      {/* A form of one text box, so Enter applies it. */}
      <form
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          apply();
        }}
      >
        <Field
          label={t("richtext.linkAddress")}
          error={
            refused && linkHref(draft) === ""
              ? t("richtext.linkRefused")
              : undefined
          }
        >
          {(control) => (
            <TextInput
              {...control}
              type="text"
              inputMode="url"
              autoComplete="off"
              spellCheck={false}
              value={draft}
              placeholder={t("richtext.linkPlaceholder")}
              onChange={(event) => setDraft(event.target.value)}
            />
          )}
        </Field>
      </form>
    </ConfirmModal>
  );
}
