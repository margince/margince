// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import { AddDocumentDialog } from "../../components/adddocument";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatBytes, formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { attachmentAnchor, attachmentTitle } from "../../lib/attachmentlink";
import { useViewerZone } from "../../lib/viewerzone";
import { type Project360, stateOf } from "./projectsections";

type Attachment = components["schemas"]["Attachment"];

/**
 * The project's own uploads and the files of the activities filed under it,
 * newest first, in the drawer the Contracts card opens. The drawer's head
 * names them, so the card has none.
 */
export function ProjectDocuments({
  view,
  readOnly,
}: Readonly<{ view: Project360; readOnly: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const [adding, setAdding] = useState(false);
  const documents = view.documents?.data ?? [];
  const state = stateOf(
    view,
    "documents",
    Boolean(view.documents),
    documents.length,
  );
  const mayAdd = !readOnly && (state === "ready" || state === "empty");
  return (
    <Panel
      actions={
        mayAdd ? (
          <Button onClick={() => setAdding(true)}>
            {t("docs.add.action")}
          </Button>
        ) : undefined
      }
    >
      <AddDocumentDialog
        anchor={{ record: "project", id: view.project.id }}
        open={adding}
        onClose={() => setAdding(false)}
      />
      {state === "ready" ? (
        documents.map((doc) => (
          <DocumentRow key={doc.id} doc={doc} locale={locale} />
        ))
      ) : (
        <PanelBody>
          <SurfaceState
            state={state}
            emptyLabel={t("project.documents.empty")}
            loadingLabel={t("project.documents.title")}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}

function DocumentRow({
  doc,
  locale,
}: Readonly<{
  doc: Attachment;
  locale: ReturnType<typeof useLocale>["locale"];
}>) {
  const readerZone = useViewerZone();
  return (
    <PanelRow className="project-row">
      {/* The name is the download, as on the company page: one thing to find
          for the one thing the row does. */}
      <a className="project-rowlink" {...attachmentAnchor(doc)}>
        {attachmentTitle(doc)}
      </a>
      <span className="project-row-meta t-caption">
        {doc.byte_size != null && (
          <span className="t-num">{formatBytes(doc.byte_size, locale)}</span>
        )}
        <span>{formatDateAbbrev(doc.created_at, locale, readerZone)}</span>
      </span>
    </PanelRow>
  );
}
