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

// WHAT A HUMAN MAY CHOOSE, which is not the whole vocabulary. The two
// `*_attachment` values record PROVENANCE — capture derives them from the
// transport a file arrived on — and a file being uploaded from this dialog
// arrived on none. Offering them would let the picker mint a claim about where a
// file came from that is false the moment it is chosen, and the document library
// reads that column as an answer to exactly that question.
//
// Derived by subtraction from CATEGORY_KEYS rather than listed, so a value added
// to the contract lands in the picker by default and only a deliberate edit here
// keeps it out. Not a second hand-kept list: the two drift apart.
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
