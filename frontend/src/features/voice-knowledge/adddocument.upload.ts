import type { QueryKey } from "@tanstack/react-query";
import type { MessageKey } from "../../i18n/en";
import { activityFileStaleKeys } from "../../lib/activitykeys";
import type { AttachmentParent } from "../../lib/attachmentupload";
import type { DocumentAnchor } from "./adddocument";

/**
 * What a landed upload makes stale, per anchor. Each library is read under its
 * own key. The contact 360 carries no attachments, so it is not refetched.
 */
export function staleAfterUpload(anchor: DocumentAnchor): readonly QueryKey[] {
  if (anchor.record === "contact") {
    return [["attachments", "contact", anchor.id]];
  }
  if (anchor.record === "activity") return activityFileStaleKeys(anchor.id);
  if (anchor.record === "deal") {
    return [
      ["deal-documents", anchor.id],
      ["deal-attachments", anchor.id],
    ];
  }
  return [
    ["companyDocuments", anchor.id],
    ["company360", anchor.id],
  ];
}

// Why the upload cannot be offered, in the order the reader can act on. A
// missing file is theirs to fix and a missing grant is not. A request in
// flight is not one of these: it is a wait, not a refusal. The button's
// `pending` handles it, swallowing a second press that would file a second
// copy, while the control stays with the reader.
export function uploadRefusal({
  file,
  parent,
  permitted,
  maxBytes,
}: Readonly<{
  file: File | undefined;
  parent: AttachmentParent | null;
  permitted: boolean;
  maxBytes: number | undefined;
}>): MessageKey | null {
  if (!permitted) {
    return "docs.add.errRefused";
  }
  if (!file) {
    return "docs.add.errNoFile";
  }
  // "A deal" chosen and none picked. This is its own refusal, never a fallback
  // to the account. A document filed where the reader did not choose is worse
  // than one not filed. Only a deal's documents can be read for deal fields.
  if (!parent) {
    return "docs.add.errNoDeal";
  }
  // Only when the server has said what it accepts. Without that answer the
  // server refuses the upload. A too-large file costs one wasted request, while
  // a guessed limit refuses a file the installation would take.
  //
  // The ceiling bounds the whole request, but this compares the file alone. A
  // file within the few hundred bytes of part framing still reaches the
  // server's refusal, which names the same number this message does.
  if (maxBytes !== undefined && file.size > maxBytes) {
    return "docs.add.errTooLarge";
  }
  return null;
}
