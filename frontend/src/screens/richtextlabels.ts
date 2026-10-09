// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";

/** The rich-text toolbar's names, in the reader's language. */
export function useRichTextLabels() {
  const t = useT();
  return {
    bold: t("richtext.bold"),
    italic: t("richtext.italic"),
    bulletList: t("richtext.bulletList"),
    numberList: t("richtext.numberList"),
    link: t("richtext.link"),
    linkPrompt: t("richtext.linkPrompt"),
    heading: t("richtext.heading"),
  };
}
