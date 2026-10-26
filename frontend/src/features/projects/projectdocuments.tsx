// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatBytes, formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { type Project360, stateOf } from "./projectsections";

type Attachment = components["schemas"]["Attachment"];

/**
 * The files attached to the project itself, newest first, in the drawer the
 * Contracts card opens. The drawer's head names them, so the card has none.
 */
export function ProjectDocuments({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const documents = view.documents?.data ?? [];
  const state = stateOf(
    view,
    "documents",
    Boolean(view.documents),
    documents.length,
  );
  return (
    <Panel>
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
  const recordZone = useRecordZone();
  return (
    <PanelRow className="project-row">
      {/* The name is the download, as on the company page: one thing to find
          for the one thing the row does. */}
      <a
        className="project-rowlink"
        href={`/v1/attachments/${doc.id}`}
        download={doc.filename}
      >
        {doc.title || doc.filename}
      </a>
      <span className="project-row-meta t-caption">
        {doc.byte_size != null && (
          <span className="t-num">{formatBytes(doc.byte_size, locale)}</span>
        )}
        <span>{formatDateAbbrev(doc.created_at, locale, recordZone)}</span>
      </span>
    </PanelRow>
  );
}
