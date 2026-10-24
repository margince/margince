// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../../api/schema";
import { IMPORT_ID } from "../../app/nav";
import { useUnsavedGuard } from "../../components/unsaved";
import { useArrivalFocus } from "../../design-system/arrivalfocus";
import { Callout } from "../../design-system/callout";
import { FileDropzone } from "../../design-system/filedropzone";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { RecordBack } from "../../design-system/recordview";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ACCEPTED_ATTACHMENT_EXTENSIONS } from "../../lib/attachmentupload";
import { postMultipart } from "../../lib/multipart";
import { problemMessageOf } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import "./vcard-import.css";

type VCardReport = components["schemas"]["VCardImportReport"];
type VCardResult = components["schemas"]["VCardImportResult"];

/** What each outcome is called, and how loudly. `needs_review` is the one that
 * asks something of the reader, so it is the only one toned: a card that merely
 * resembles somebody was written nowhere, and nobody finds that out unless the
 * report says so. */
const OUTCOMES: Readonly<
  Record<VCardResult["outcome"], { label: MessageKey; tone?: "warning" }>
> = {
  created: { label: "vcardImport.outcome.created" },
  updated: { label: "vcardImport.outcome.updated" },
  needs_review: { label: "vcardImport.outcome.needsReview", tone: "warning" },
  proposed: { label: "vcardImport.outcome.proposed", tone: "warning" },
  skipped: { label: "vcardImport.outcome.skipped" },
};

/** The photo types a card reading takes: the attachment types that are pictures
 * a model reads, without TIFF, which no binding carries. Each must stay one the
 * attachment mirror accepts, which the unit test holds. */
export const CARD_PHOTO_EXTENSIONS = [
  ".png",
  ".jpg",
  ".jpeg",
  ".gif",
  ".webp",
  ".heic",
  ".heif",
] as const satisfies readonly (typeof ACCEPTED_ATTACHMENT_EXTENSIONS)[number][];

const ACCEPT_ATTR = [".vcf", "text/vcard", ...CARD_PHOTO_EXTENSIONS].join(",");

/** isCardPhoto picks the endpoint from the file itself. A phone's HEIC often
 * arrives with no content type, so the extension decides as well. */
export function isCardPhoto(file: File): boolean {
  const name = file.name.toLowerCase();
  return (
    file.type.startsWith("image/") ||
    CARD_PHOTO_EXTENSIONS.some((extension) => name.endsWith(extension))
  );
}

/** isReport reports whether a body carries the one thing the report renderer
 * needs. Narrow on purpose: this guards against a truncated or wrong answer,
 * not against a server that disagrees with the contract about a field. */
function isReport(body: unknown): body is VCardReport {
  if (typeof body !== "object" || body === null || !("results" in body)) {
    return false;
  }
  return Array.isArray(body.results);
}

/** useImportVCards posts the file and refreshes the contact list behind it. */
function useImportVCards() {
  const client = useQueryClient();
  return useMutation({
    // The File arrives as a variable rather than through a closure: the click
    // belongs to the committed render, so what it hands over cannot be older
    // than the control that carried it.
    mutationFn: async (file: File): Promise<VCardReport> => {
      const payload = await postMultipart(
        isCardPhoto(file)
          ? "/v1/contacts/card-photos"
          : "/v1/contacts/vcard-import",
        { file },
      );
      // A 200 whose body is not a report is a transport fault, not an import
      // of nothing. Rendering it would say "that file held no cards" about a
      // file whose cards may well have been written.
      if (!isReport(payload)) {
        throw new Error(
          "the import answered with something this app cannot read",
        );
      }
      return payload;
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["contacts"] });
    },
  });
}

/**
 * The link a contact follows to import address cards. A handed-over card is
 * first-party data — the contact gave it — which is what justifies storing their
 * details, and a human pressing this is what makes the import WRITE rather than
 * stage. So the verb lives on the contact list beside the other ways a contact
 * comes to exist, not in a settings page.
 */
export function VCardImportAction() {
  const t = useT();
  return (
    <a
      className="btn btn-ghost"
      data-testid="vcard-import"
      href={routeHash({ screen: "contacts", id: IMPORT_ID })}
    >
      {t("vcardImport.action")}
    </a>
  );
}

/** The import page at `#/contacts/import`, and the report of what became of
 * each card. Leaving discards the file and the report. */
export function VCardImportPage() {
  const t = useT();
  const importer = useImportVCards();
  const [picked, setPicked] = useState<File | undefined>(undefined);
  const title = useArrivalFocus<HTMLHeadingElement>();
  // Leaving mid-flight does not stop the write, it only loses the report that
  // says what the write did.
  useUnsavedGuard(importer.isPending);

  return (
    <div className="wrap narrow">
      <RecordBack
        href={routeHash({ screen: "contacts" })}
        label={t("vcardImport.back")}
      />
      <Heading
        size="xlarge"
        className="t-display vcard-import-title"
        ref={title}
        tabIndex={-1}
      >
        {t("vcardImport.title")}
      </Heading>
      <Panel>
        <PanelBody className="form-stack">
          <div data-testid="vcard-import-file">
            {/* Withdrawn while an import is running. A second file chosen mid-
                flight would start a second write, and this page holds ONE
                report — so one set of contact changes would land with nothing
                on screen saying what happened to it. */}
            {importer.isPending ? (
              <p className="t-sub">{t("vcardImport.working")}</p>
            ) : (
              <FileDropzone
                label={t("vcardImport.fileLabel")}
                hint={t("vcardImport.whichFile")}
                emptyLabel={t("vcardImport.choose")}
                accept={ACCEPT_ATTR}
                file={picked}
                onPick={(file) => {
                  setPicked(file);
                  importer.mutate(file);
                }}
              />
            )}
          </div>
          {importer.isError && (
            <div data-testid="vcard-import-error">
              <Callout
                tone="danger"
                kind="outcome"
                title={t("vcardImport.failed")}
              >
                {problemMessageOf(importer.error, t)}
              </Callout>
            </div>
          )}
          {importer.isSuccess && <ImportReport report={importer.data} />}
        </PanelBody>
      </Panel>
    </div>
  );
}

/** ImportReport lists what became of every card, in the order the file listed
 * them. Every card appears, including the ones nothing was written for: a file
 * half-ignored under a success message is worse than a refusal, because nobody
 * can tell who is missing. */
function ImportReport({ report }: Readonly<{ report: VCardReport }>) {
  const t = useT();
  const cards = report.results ?? [];
  if (cards.length === 0) {
    return <p className="t-sub">{t("vcardImport.noCards")}</p>;
  }
  return (
    <ul className="vcard-import-report" data-testid="vcard-import-report">
      {cards.map((card) => {
        const outcome = OUTCOMES[card.outcome];
        return (
          <li key={card.index}>
            <span>{card.full_name}</span>
            <span
              className={
                outcome?.tone === "warning" ? "vcard-import-warning" : "t-sub"
              }
            >
              {/* An outcome this build has no name for is a server newer than
                  this tab. The word it sent is still worth showing. */}
              {outcome ? t(outcome.label) : card.outcome}
            </span>
            {card.reason && <span className="t-sub">{card.reason}</span>}
          </li>
        );
      })}
    </ul>
  );
}
