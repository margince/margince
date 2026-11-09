// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext, useState } from "react";
import type { components } from "../api/schema";
import { Badge, SegmentedControl } from "../design-system/atoms";
import { DocumentTile } from "../design-system/documenttile";
import { activityDay } from "../format/activitywhen";
import { formatDateAbbrev } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { attachmentAnchor } from "../lib/attachmentlink";
import { labelFor } from "../lib/labelfor";
import { readStored, STORAGE_KEYS, writeStored } from "../lib/storage";
import { useViewerZone } from "../lib/viewerzone";
import { FileOrigin } from "./fileorigin";
import "./attachmenttile.css";

type Attachment = components["schemas"]["Attachment"];
export type DocumentCategory = NonNullable<Attachment["category"]>;
type DocState = NonNullable<Attachment["doc_state"]>;

/** The one wording of a file's category, on every record's Documents tab. */
export const CATEGORY_LABELS: Record<DocumentCategory, MessageKey> = {
  contract: "docs.category.contract",
  offer: "docs.category.offer",
  legal: "docs.category.legal",
  email_attachment: "docs.category.email",
  message_attachment: "docs.category.message",
  other: "docs.category.other",
  link: "docs.category.link",
};

type DocumentsView = "grid" | "list";
const VIEWS: readonly DocumentsView[] = ["grid", "list"];

// The gallery's choice, read by every tile in it.
const DocumentsViewContext = createContext<DocumentsView>("grid");

const STATE_LABELS: Record<DocState, MessageKey> = {
  draft: "docs.state.draft",
  current: "docs.state.current",
  final: "docs.state.final",
  superseded: "docs.state.superseded",
};

/**
 * A record's file as a tile. The name opens it, and the stage says its kind
 * and the state somebody asserted. One line says its category and when it
 * arrived. Superseded is the one state that changes how a file reads: it is
 * history, so it warns and its paper recedes.
 */
export function AttachmentTile({
  file,
  onOpenEmail,
  verbs,
  after,
  withSource = false,
  note,
  receded = false,
}: Readonly<{
  file: Attachment;
  /** Opens a mail in the page's drawer, from the line naming a file's mail. */
  onOpenEmail?: (activityId: string) => void;
  verbs?: ReactNode;
  /** Under the tile's body, behind its own hairline. */
  after?: ReactNode;
  /** Names where the file came from, for a library fed by several sources. */
  withSource?: boolean;
  /** A line under the facts the record adds, such as a hidden file's badge. */
  note?: ReactNode;
  /** Set aside by a reader, as a hidden file is: its paper recedes. */
  receded?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const view = useContext(DocumentsViewContext);
  const superseded = file.doc_state === "superseded";
  const title = file.title || file.filename;
  return (
    <DocumentTile
      filename={file.filename}
      title={title}
      anchor={attachmentAnchor(file)}
      lapsed={superseded || receded}
      layout={view === "list" ? "row" : "tile"}
      mark={
        file.doc_state ? (
          <Badge tone={superseded ? "warning" : undefined}>
            {t(labelFor(STATE_LABELS, file.doc_state))}
          </Badge>
        ) : null
      }
      meta={[
        // The name it saves as, when a human gave it another title.
        title === file.filename ? null : file.filename,
        file.category ? t(labelFor(CATEGORY_LABELS, file.category)) : null,
        withSource ? file.source : null,
        // A mail's file arrived when the mail did, not when it was filed.
        file.origin
          ? activityDay(
              file.origin.occurred_at,
              file.origin.occurred_precision,
              locale,
              zone,
              formatDateAbbrev,
            )
          : formatDateAbbrev(file.created_at, locale, zone),
      ]
        .filter(Boolean)
        .join(" · ")}
      body={
        <>
          <FileOrigin origin={file.origin} onOpenEmail={onOpenEmail} />
          {note}
          {verbs ? <div className="attachment-tile-verbs">{verbs}</div> : null}
        </>
      }
    >
      {after ? <div className="attachment-tile-after">{after}</div> : null}
    </DocumentTile>
  );
}

/**
 * The gallery the tiles sit in: as many to a row as fit, or one file to a
 * row. The reader's choice holds for every Documents tab on this device.
 */
export function AttachmentGrid({
  children,
}: Readonly<{ children: ReactNode }>) {
  const t = useT();
  const [view, setView] = useState<DocumentsView>(() =>
    readStored(STORAGE_KEYS.documentsView) === "list" ? "list" : "grid",
  );
  const choose = (next: DocumentsView) => {
    setView(next);
    writeStored(STORAGE_KEYS.documentsView, next);
  };
  return (
    <DocumentsViewContext.Provider value={view}>
      <div className="attachment-gallery-bar">
        <SegmentedControl
          label={t("docs.view.label")}
          options={VIEWS}
          value={view}
          onChange={choose}
          labels={{ grid: t("docs.view.grid"), list: t("docs.view.list") }}
        />
      </div>
      <ul
        className={
          view === "list"
            ? "attachment-grid attachment-list"
            : "attachment-grid"
        }
      >
        {children}
      </ul>
    </DocumentsViewContext.Provider>
  );
}
