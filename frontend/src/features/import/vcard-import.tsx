// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { useUnsavedGuard } from "../../components/unsaved";
import {
  useVCardImportRefusal,
  VCARD_COMPANY_DIAL,
} from "../../components/vcardimportlink";
import { useArrivalFocus } from "../../design-system/arrivalfocus";
import {
  Badge,
  Button,
  EmptyState,
  PendingBody,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { FileDropzone } from "../../design-system/filedropzone";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ACCEPTED_ATTACHMENT_EXTENSIONS } from "../../lib/attachmentupload";
import { companyContactsListKey } from "../../lib/companycontacts.keys";
import { valueFor } from "../../lib/labelfor";
import { ACTION_PARAM, QUALIFY_ACTION } from "../../lib/leads.address";
import { postMultipart } from "../../lib/multipart";
import { problemMessageOf } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useUrlParams } from "../../lib/urlstate";
import { useMeSettled } from "../../lib/writerefusal";
import "./vcard-import.css";

type VCardReport = components["schemas"]["VCardImportReport"];
type VCardResult = components["schemas"]["VCardImportResult"];
type VCardField = components["schemas"]["VCardImportField"];

/** Each field the report names, under the label the contact page gives it.
 * The report and the record then call one field by one name. */
const FIELD_LABELS: Readonly<Record<VCardField["field"], MessageKey>> = {
  name: "contacts.name",
  email: "contacts.email",
  phone: "contact.enriched.field.phone",
  mobile_phone: "field.phoneMobile",
  title: "contact.enriched.field.title",
  company_name: "contact.enriched.field.company_name",
  address: "contact.enriched.field.address",
  linkedin: "contact.enriched.field.linkedin",
  website: "contact.enriched.field.website",
};

/** What the import did with a field the card stated. A field the card left
 * out is listed apart, under one muted heading. */
const ACTION_LABELS: Readonly<
  Record<Exclude<VCardField["action"], "not_in_card">, MessageKey>
> = {
  imported: "vcardImport.field.imported",
  replaced: "vcardImport.field.replaced",
  kept: "vcardImport.field.kept",
};

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
 * a model reads. TIFF is left out because no binding carries it, and HEIC and
 * HEIF because the server does not decode them. Each must stay one the
 * attachment mirror accepts, which the unit test holds. */
export const CARD_PHOTO_EXTENSIONS = [
  ".png",
  ".jpg",
  ".jpeg",
  ".gif",
  ".webp",
] as const satisfies readonly (typeof ACCEPTED_ATTACHMENT_EXTENSIONS)[number][];

/** HEIC and HEIF are left out of the picker so an iPhone converts the photo to
 * JPEG when it is picked. A file dragged in still goes to the photo endpoint,
 * which refuses it with the instruction to save it as JPEG or PNG. */
const UNSERVED_PHOTO_EXTENSIONS = [".heic", ".heif"] as const;

const ACCEPT_ATTR = [
  ".vcf",
  "text/vcard",
  "image/jpeg",
  "image/png",
  "image/gif",
  "image/webp",
  ...CARD_PHOTO_EXTENSIONS,
].join(",");

/** isCardPhoto picks the endpoint from the file itself. A phone's photo often
 * arrives with no content type, so the extension decides as well. */
export function isCardPhoto(file: File): boolean {
  const name = file.name.toLowerCase();
  return (
    file.type.startsWith("image/") ||
    [...CARD_PHOTO_EXTENSIONS, ...UNSERVED_PHOTO_EXTENSIONS].some((extension) =>
      name.endsWith(extension),
    )
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

/** The vCard import route; a company's Contacts tab names the company in the query. */
function vcardImportPath(companyId: string | undefined): string {
  return companyId === undefined
    ? "/v1/contacts/vcard-import"
    : `/v1/contacts/vcard-import?company_id=${encodeURIComponent(companyId)}`;
}

/** useImportVCards posts the file and refreshes the contact list behind it. */
function useImportVCards(companyId: string | undefined) {
  const client = useQueryClient();
  return useMutation({
    // The File arrives as a variable rather than through a closure: the click
    // belongs to the committed render, so what it hands over cannot be older
    // than the control that carried it.
    mutationFn: async (file: File): Promise<VCardReport> => {
      const payload = await postMultipart(
        isCardPhoto(file)
          ? "/v1/contacts/card-photos"
          : vcardImportPath(companyId),
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
      if (companyId !== undefined) {
        // The company's own Contacts tab and its contact counts read the new
        // employment edges; their keys sit under the company's.
        await client.invalidateQueries({
          queryKey: [companyContactsListKey(companyId)],
        });
        await client.invalidateQueries({ queryKey: ["companies"] });
      }
    },
  });
}

/** The import page at `#/contacts/import`, and the report of what became of
 * each card. Leaving discards the file and the report. */
export function VCardImportPage() {
  const t = useT();
  const [dials] = useUrlParams();
  // Set when the import came from one company's Contacts tab: its new
  // contacts work for that company.
  const companyId = dials.get(VCARD_COMPANY_DIAL);
  const refusal = useVCardImportRefusal();
  // No file field before the seat's access is read: a file picked then would
  // be sent at once.
  const settled = useMeSettled();
  const importer = useImportVCards(companyId);
  const [photoRefused, setPhotoRefused] = useState(false);
  const [picked, setPicked] = useState<File | undefined>(undefined);
  const title = useArrivalFocus<HTMLHeadingElement>();
  const fileField = useRef<HTMLDivElement>(null);
  // The next card starts from an empty page: the last file and its report go.
  // Focus returns to the file field, so the reader can pick at once.
  const startNext = () => {
    setPicked(undefined);
    setPhotoRefused(false);
    importer.reset();
    fileField.current?.querySelector("input")?.focus();
  };
  // Leaving mid-flight does not stop the write, it only loses the report that
  // says what the write did.
  useUnsavedGuard(importer.isPending);

  return (
    <div className="wrap narrow page-pane">
      <Heading
        size="xlarge"
        className="t-display vcard-import-title arrival-focus"
        ref={title}
        tabIndex={-1}
      >
        {t("vcardImport.title")}
      </Heading>
      {companyId !== undefined && (
        <p className="t-sub" data-testid="vcard-import-company">
          {t("vcardImport.forCompany")}{" "}
          <EntityRef kind="company" id={companyId} />
        </p>
      )}
      {!settled ? (
        <PendingBody label={t("vcardImport.title")} />
      ) : (
        <Panel>
          <PanelBody className="form-stack">
            <div data-testid="vcard-import-file" ref={fileField}>
              {/* Withdrawn while an import is running. A second file chosen mid-
                flight would start a second write, and this page holds ONE
                report — so one set of contact changes would land with nothing
                on screen saying what happened to it. */}
              {/* A refusal takes the file field's place only: an import already
                  sent keeps its report, whatever /me says since. */}
              {importer.isPending ? (
                <p className="t-sub">{t("vcardImport.working")}</p>
              ) : refusal !== undefined ? (
                <EmptyState>{refusal}</EmptyState>
              ) : (
                <FileDropzone
                  label={t("vcardImport.fileLabel")}
                  hint={t("vcardImport.whichFile")}
                  emptyLabel={t("vcardImport.choose")}
                  accept={ACCEPT_ATTR}
                  file={picked}
                  onPick={(file) => {
                    setPicked(file);
                    // A photo goes to the approval queue, which files its
                    // contacts under no company. Saying so beats a company
                    // tab import that silently ignores the company.
                    const refused =
                      companyId !== undefined && isCardPhoto(file);
                    setPhotoRefused(refused);
                    if (!refused) {
                      importer.mutate(file);
                    }
                  }}
                />
              )}
            </div>
            {photoRefused && (
              <div data-testid="vcard-import-photo-refused">
                <Callout
                  tone="warning"
                  kind="outcome"
                  title={t("vcardImport.companyPhotoTitle")}
                >
                  {t("vcardImport.companyPhoto")}
                </Callout>
              </div>
            )}
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
            {(importer.isSuccess || importer.isError || photoRefused) &&
              refusal === undefined && (
                <div>
                  <Button data-testid="vcard-import-next" onClick={startNext}>
                    {t("vcardImport.next")}
                  </Button>
                </div>
              )}
          </PanelBody>
        </Panel>
      )}
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
    <>
      <StagedForReview count={report.staged_for_review ?? 0} />
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
              {card.lead_id && <HeldLeadVerbs leadId={card.lead_id} />}
              {card.fields && card.fields.length > 0 && (
                <CardFields fields={card.fields} />
              )}
            </li>
          );
        })}
      </ul>
    </>
  );
}

/** StagedForReview says how many cards wait for an answer in the inbox, and
 * opens it. A card the import did not write waits there for 30 days. */
function StagedForReview({ count }: Readonly<{ count: number }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (count === 0) return null;
  return (
    <p data-testid="vcard-import-staged">
      {plural("vcardImport.staged", count, {
        count: formatNumber(count, locale),
      })}{" "}
      <Button
        type="button"
        variant="link"
        onClick={() => navigate({ screen: "worklist" })}
      >
        {t("vcardImport.staged.open")}
      </Button>
    </p>
  );
}

/** HeldLeadVerbs answers a card skipped because its contact is already a lead.
 * Nothing was created. The reader converts the lead in its Qualify dialog, or
 * opens it. */
function HeldLeadVerbs({ leadId }: Readonly<{ leadId: string }>) {
  const t = useT();
  return (
    <>
      <Button
        type="button"
        onClick={() =>
          navigate(
            { screen: "leads", id: leadId },
            new Map([[ACTION_PARAM, QUALIFY_ACTION]]),
          )
        }
      >
        {t("contact.leadHeld.convert")}
      </Button>
      <Button
        type="button"
        variant="link"
        onClick={() => navigate({ screen: "leads", id: leadId })}
      >
        {t("contact.leadHeld.open")}
      </Button>
    </>
  );
}

/** CardFields shows what the import did with each field of one card. First
 * what it took, replaced or kept, then what the card did not carry. A field or
 * action this build has no name for shows the word sent. */
function CardFields({ fields }: Readonly<{ fields: readonly VCardField[] }>) {
  const t = useT();
  const label = (field: VCardField["field"]) => {
    const key = valueFor(FIELD_LABELS, field, undefined);
    return key ? t(key) : field;
  };
  const stated = fields.filter((f) => f.action !== "not_in_card");
  const absent = fields.filter((f) => f.action === "not_in_card");
  return (
    <div className="vcard-import-fields" data-testid="vcard-import-fields">
      {stated.length > 0 && (
        <ul>
          {stated.map((f) => {
            const action =
              f.action === "not_in_card"
                ? undefined
                : valueFor(ACTION_LABELS, f.action, undefined);
            return (
              <li key={`${f.field}:${f.value ?? ""}`} data-action={f.action}>
                <span className="vcard-import-field-label">
                  {label(f.field)}
                </span>
                <span>{f.value}</span>
                <Badge>{action ? t(action) : f.action}</Badge>
              </li>
            );
          })}
        </ul>
      )}
      {absent.length > 0 && (
        <div
          className="vcard-import-absent t-sub"
          data-testid="vcard-import-absent"
        >
          <span>{t("vcardImport.field.notInCard")}</span>
          <ul>
            {absent.map((f) => (
              <li key={f.field}>{label(f.field)}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

export {
  useVCardImportRefusal,
  VCardImportAction,
} from "../../components/vcardimportlink";
