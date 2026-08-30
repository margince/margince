// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import type { components } from "../api/schema";
import { Button, Modal } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { FileDropzone } from "../design-system/filedropzone";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import "./vcard-import.css";

type VCardReport = components["schemas"]["VCardImportReport"];
type VCardResult = components["schemas"]["VCardImportResult"];

/** What each outcome is called, and how loudly. `needs_review` is the one that
 * asks something of the reader, so it is the only one toned: a card that merely
 * resembles somebody was written nowhere, and nobody finds that out unless the
 * report says so. */
const OUTCOMES: Readonly<
  Record<VCardResult["outcome"], { label: MessageKey; tone?: "warn" }>
> = {
  created: { label: "vcardImport.outcome.created" },
  updated: { label: "vcardImport.outcome.updated" },
  needs_review: { label: "vcardImport.outcome.needsReview", tone: "warn" },
  skipped: { label: "vcardImport.outcome.skipped" },
};

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
      // Sent as multipart by hand rather than through the typed client, which
      // serializes JSON bodies and cannot carry a file part.
      const body = new FormData();
      body.append("file", file);
      // contract-fetch:allow multipart — see the note above
      const response = await fetch("/v1/people/vcard-import", {
        method: "POST",
        body,
        credentials: "include",
      });
      const payload = await response.json().catch(() => undefined);
      if (!response.ok) {
        throwProblem(payload);
      }
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
      await client.invalidateQueries({ queryKey: ["people"] });
    },
  });
}

/**
 * VCardImport is the button a person presses to import address cards, and the
 * report of what became of each one.
 *
 * A handed-over card is first-party data — the person gave it — which is what
 * justifies storing their details, and a human pressing this is what makes the
 * import WRITE rather than stage. So the verb lives on the contact list beside
 * the other ways a contact comes to exist, not in a settings page.
 */
export function VCardImport() {
  const t = useT();
  const importer = useImportVCards();
  const [open, setOpen] = useState(false);
  const [picked, setPicked] = useState<File | undefined>(undefined);
  const titleId = useId();

  function close() {
    setOpen(false);
    setPicked(undefined);
    importer.reset();
  }

  return (
    <>
      <Button data-testid="vcard-import" onClick={() => setOpen(true)}>
        {t("vcardImport.action")}
      </Button>
      <Modal open={open} onClose={close} labelledBy={titleId}>
        <h2 id={titleId}>{t("vcardImport.title")}</h2>
        <div data-testid="vcard-import-file">
          {/* Withdrawn while an import is running. A second file chosen mid-
              flight would start a second write, and this dialog holds ONE
              report — so one set of contact changes would land with nothing on
              screen saying what happened to it. Taking the control away is
              honest; leaving it live and dropping the pick would not be. */}
          {importer.isPending ? (
            <p className="co-muted">{t("vcardImport.working")}</p>
          ) : (
            <FileDropzone
              label={t("vcardImport.fileLabel")}
              hint={t("vcardImport.whichFile")}
              emptyLabel={t("vcardImport.choose")}
              accept=".vcf,text/vcard"
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
            <Callout tone="danger" live="alert">
              {problemMessageOf(importer.error, t)}
            </Callout>
          </div>
        )}
        {importer.isSuccess && <ImportReport report={importer.data} />}

        <div className="form-actions">
          <Button variant="ghost" onClick={close}>
            {t("vcardImport.done")}
          </Button>
        </div>
      </Modal>
    </>
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
    return <p className="co-muted">{t("vcardImport.noCards")}</p>;
  }
  return (
    <ul className="vcard-import-report" data-testid="vcard-import-report">
      {cards.map((card) => {
        const outcome = OUTCOMES[card.outcome];
        return (
          <li key={card.index}>
            <span className="vcard-import-name">{card.full_name}</span>
            <span
              className={
                outcome?.tone === "warn" ? "vcard-import-warn" : "co-muted"
              }
            >
              {/* An outcome this build has no name for is a server newer than
                  this tab. The word it sent is still worth showing. */}
              {outcome ? t(outcome.label) : card.outcome}
            </span>
            {card.reason && <span className="co-muted">{card.reason}</span>}
          </li>
        );
      })}
    </ul>
  );
}
