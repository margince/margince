// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";

type Attachment = components["schemas"]["Attachment"];

// A document link arrives with a mail and is never filed from this dialog.
type Category = Exclude<NonNullable<Attachment["category"]>, "link">;

export const CATEGORY_KEYS: Record<Category, MessageKey> = {
  contract: "docs.category.contract",
  offer: "docs.category.offer",
  legal: "docs.category.legal",
  email_attachment: "docs.category.email",
  message_attachment: "docs.category.message",
  other: "docs.category.other",
};

// What a user may choose, which is not the whole vocabulary. The two
// `*_attachment` values record where a file arrived from, which capture derives.
// A file added here arrived on no channel, so offering them would be a false claim.
// The list is CATEGORY_KEYS minus these two, so a new contract value is offered.
const CAPTURED_ONLY: readonly Category[] = [
  "email_attachment",
  "message_attachment",
];

export type Choosable = Exclude<
  Category,
  "email_attachment" | "message_attachment"
>;

export const UPLOADABLE_CATEGORIES = (
  Object.keys(CATEGORY_KEYS) as Category[]
).filter((key): key is Choosable => !CAPTURED_ONLY.includes(key));

// What is being added: bytes, or the address of a document kept elsewhere.
// A link takes one request, so unlike an upload it has no partial success.
export type Source = "file" | "link";
export const SOURCES: readonly Source[] = ["file", "link"];

export const SOURCE_WORDS: Record<
  Source,
  Readonly<{ nameHint: MessageKey; busy: MessageKey; submit: MessageKey }>
> = {
  file: {
    nameHint: "docs.add.nameHint",
    busy: "docs.add.uploading",
    submit: "docs.add.submit",
  },
  link: {
    nameHint: "docs.add.linkNameHint",
    busy: "docs.add.adding",
    submit: "docs.add.submitLink",
  },
};
