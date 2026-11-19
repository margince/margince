import type { components } from "../api/schema";

type Attachment = components["schemas"]["Attachment"];

type AttachmentAnchor =
  | { href: string; target: "_blank"; rel: "noopener noreferrer" }
  | { href: string; download: string };

const WEB_ADDRESS = /^https?:\/\//i;

/** The name a file goes by on screen: the title someone gave it, or its filename. */
export function attachmentTitle(
  file: Readonly<{ title?: string | null; filename?: string | null }>,
): string {
  return file.title || file.filename || "";
}

// Where a document row's name points. A file downloads from the API. A link row
// has no bytes and opens its address in a new tab. An address that is not
// http(s) falls back to the download route, which answers 404 for a link. So a
// scheme such as javascript: can never become an href.
export function attachmentAnchor(file: Attachment): AttachmentAnchor {
  if (file.category === "link" && file.url && WEB_ADDRESS.test(file.url)) {
    return { href: file.url, target: "_blank", rel: "noopener noreferrer" };
  }
  return { href: `/v1/attachments/${file.id}`, download: file.filename };
}

// The same rule in the shape `FileChip` takes: a stored file's route, or the
// address a link row opens in a new tab.
export function fileChipTarget(
  file: Attachment,
): { href: string } | { link: string } {
  const anchor = attachmentAnchor(file);
  return "target" in anchor ? { link: anchor.href } : { href: anchor.href };
}
