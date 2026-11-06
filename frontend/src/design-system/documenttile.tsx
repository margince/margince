// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { fileKind, useStoredFileClick } from "./filechip";
import "./documenttile.css";

/** A link to the file: a stored one carries `download`, a web address `target`. */
export type DocumentAnchor = Readonly<{
  href: string;
  download?: string;
  target?: string;
  rel?: string;
}>;

/**
 * One document as a tile in a gallery. A page-shaped stage is stamped with
 * its kind, then come its title, one line of facts and the caller's parts. The
 * title is the tile's door, stretched over the whole tile. Pressing the
 * paper opens it, and the caller's own verbs keep their presses.
 */
export function DocumentTile({
  filename,
  title,
  open,
  anchor,
  mark,
  meta,
  lapsed = false,
  layout = "tile",
  body,
  children,
}: Readonly<{
  filename: string;
  title: string;
  /** Opens the document in a dialog over the page. */
  open?: () => void;
  /** Or links the file itself; a stored file previews where it can. */
  anchor?: DocumentAnchor;
  /** A badge in the stage's far corner: the one fact worth seeing first. */
  mark?: ReactNode;
  meta?: ReactNode;
  /** A document no longer shared: its paper recedes. */
  lapsed?: boolean;
  /** A tile in a gallery, or a row in a list with the stage as its lead. */
  layout?: "tile" | "row";
  /** Under the facts, inside the tile's body. */
  body?: ReactNode;
  /** Under the body, each section behind its own hairline. */
  children?: ReactNode;
}>) {
  const kind = fileKind(filename);
  return (
    <article
      className={[
        "board-doc",
        layout === "row" ? "board-doc-row" : null,
        lapsed ? "board-doc-lapsed" : null,
      ]
        .filter(Boolean)
        .join(" ")}
      aria-label={title}
    >
      <div className="board-doc-stage">
        {/* A shape, not a rendering: the bytes are never fetched to draw a
            thumbnail, so the shape promises no page one. */}
        <span className="board-doc-page" aria-hidden>
          <span className="board-doc-page-line board-doc-page-head" />
          <span className="board-doc-page-line" />
          <span className="board-doc-page-line board-doc-page-short" />
          <span className="board-doc-page-line" />
          <span className="board-doc-page-line board-doc-page-short" />
        </span>
        {kind ? (
          <span className="board-doc-kind file-chip-kind" aria-hidden>
            {kind}
          </span>
        ) : null}
        {mark ? <span className="board-doc-mark">{mark}</span> : null}
      </div>
      <div className="board-doc-body">
        <DocumentDoor title={title} open={open} anchor={anchor} />
        {meta ? <p className="t-caption board-doc-meta">{meta}</p> : null}
        {body}
      </div>
      {children}
    </article>
  );
}

// The title is the tile's door. It is a button for a dialog and a link for a
// file, or plain text when there is nothing to open.
function DocumentDoor({
  title,
  open,
  anchor,
}: Readonly<{ title: string; open?: () => void; anchor?: DocumentAnchor }>) {
  if (open) {
    return (
      <button
        type="button"
        className="board-doc-title board-doc-open"
        aria-haspopup="dialog"
        onClick={open}
      >
        {title}
      </button>
    );
  }
  if (anchor?.download) {
    return <StoredFileDoor title={title} anchor={anchor} />;
  }
  if (anchor) {
    return (
      <a className="board-doc-title board-doc-open" {...anchor}>
        {title}
      </a>
    );
  }
  return <p className="board-doc-title">{title}</p>;
}

function StoredFileDoor({
  title,
  anchor,
}: Readonly<{ title: string; anchor: DocumentAnchor }>) {
  const onClick = useStoredFileClick(anchor.href, anchor.download ?? "");
  return (
    <a
      className="board-doc-title board-doc-open"
      href={anchor.href}
      download={anchor.download}
      target={anchor.target}
      rel={anchor.rel}
      onClick={onClick}
    >
      {title}
    </a>
  );
}
