// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useT } from "../i18n";
import "./fileorigin.css";

type AttachmentOrigin = components["schemas"]["AttachmentOrigin"];

/**
 * The line under a Documents row whose file came with a mail: who sent it and
 * what it was about. The server sends an origin only for a mail the reader may
 * read, so a row without one draws nothing here. An email opens in the page's
 * drawer; the drawer reads only email, so another kind is plain text.
 */
export function FileOrigin({
  origin,
  onOpenEmail,
}: Readonly<{
  origin?: AttachmentOrigin;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  if (!origin) return null;
  const who =
    origin.sender?.display_name?.trim() ||
    origin.sender?.address ||
    t("files.originUnknown");
  const subject = origin.subject?.trim();
  const label = subject
    ? t("files.origin.mail", { who, subject })
    : t("dealfiles.origin.from", { who });
  if (!onOpenEmail || origin.kind !== "email") {
    return <span className="file-origin t-caption">{label}</span>;
  }
  return (
    <button
      type="button"
      className="file-origin link-button t-caption"
      onClick={() => onOpenEmail(origin.activity_id)}
    >
      {label}
    </button>
  );
}
