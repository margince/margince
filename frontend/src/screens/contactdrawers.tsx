// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, X } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { useToast } from "../design-system/toast";
import { formatNumber, ordinalNumber } from "../format/format";
import { webUrl } from "../format/weburl";
import { useLocale, usePlural, useT } from "../i18n";
import { throwProblem } from "./common";
import { ContactProviderSection } from "./contactprovider";

// The research drawer the contact page opens over itself.
//
// The WIDE drawer — a rep works in it rather than glancing at it — and it leaves
// the page behind visible, because the record is the context that makes the
// drawer's content mean anything. Writing to the contact used to be a second
// drawer in this file; it is compose.tsx's now, which is the one composer every
// record in the product opens.

type ResearchClaim = components["schemas"]["ContactResearchClaim"];
type SaveClaim = components["schemas"]["SaveContactResearchClaim"];
type FieldKey = SaveClaim["field"];

// The profile fields a prose claim can fill, most-common first. Declared against
// the generated enum, so a field the contract drops stops compiling here rather
// than living on as an option the server would refuse.
const FIELD_KEYS = [
  "title",
  "role",
  "company_name",
  "phone",
  "linkedin",
  "address",
  "website",
] as const satisfies readonly FieldKey[];

// A reader's in-progress mapping of one claim onto a profile field. `field` is
// "" until the reader picks the one thing the closed enum exists to make them
// pick; the evidence is prefilled from the run and stays editable, because a
// claim whose citable source the crawl did not return is still one a reader may
// vouch for — `captured_by` is them.
type Mapping = Readonly<{
  field: FieldKey | "";
  value: string;
  quote: string;
  url: string;
}>;

// The first source a reader could actually click, and the words under it. A
// third-party URL that is not http(s) is inert on screen (javascript:/data:
// execute on click), so it is no basis for a saved citation either.
function citableSource(claim: ResearchClaim): { url: string; quote: string } {
  const cited = claim.sources.find((source) => webUrl(source.url));
  return { url: cited?.url ?? "", quote: cited?.quote ?? "" };
}

function initialMapping(claim: ResearchClaim): Mapping {
  const cited = citableSource(claim);
  return { field: "", value: claim.body, quote: cited.quote, url: cited.url };
}

// A mapping the save endpoint will accept, or null. The endpoint refuses a claim
// missing its value, the words it was read from, or a traceable document — "a
// fact a reader cannot trace back is what the review step exists to stop" — so
// the drawer never offers to post one it knows would 422.
function toSaveClaim(mapping: Mapping): SaveClaim | null {
  if (mapping.field === "") return null;
  const value = mapping.value.trim();
  const quote = mapping.quote.trim();
  const url = mapping.url.trim();
  if (!value || !quote || !webUrl(url)) return null;
  return { field: mapping.field, value, source_quote: quote, source_url: url };
}

function ClaimMapRow({
  claim,
  mapping,
  onMap,
  onDismiss,
}: Readonly<{
  claim: ResearchClaim;
  mapping: Mapping;
  onMap: (patch: Partial<Mapping>) => void;
  onDismiss: () => void;
}>) {
  const t = useT();
  const complete = toSaveClaim(mapping) !== null;
  const badUrl = mapping.url.trim() !== "" && !webUrl(mapping.url);
  return (
    <article className="pe-claim">
      <span className="pe-claim-ordinal">{ordinalNumber(claim.ordinal)}</span>
      <div>
        <p className="pe-claim-body">{claim.body}</p>
        <div className="pe-chiprow">
          {/* A source URL comes from a THIRD-PARTY provider, so it is untrusted:
              an unchecked href admits javascript: and data: schemes, which
              execute on click. Only http(s) becomes a link; anything else
              renders as inert text so the reader still sees what was claimed,
              without a clickable payload. */}
          {claim.sources.map((source) =>
            webUrl(source.url) ? (
              <a
                key={source.url}
                className="pe-memory-channel t-caption"
                href={source.url}
                target="_blank"
                rel="noreferrer"
              >
                {source.label}
                <ExternalLink size={12} aria-hidden="true" />
              </a>
            ) : (
              <span key={source.url} className="pe-memory-channel t-caption">
                {source.label}
              </span>
            ),
          )}
          <Badge tone={claim.confidence === "high" ? "success" : "warn"}>
            {claim.confidence}
          </Badge>
        </div>

        {/* Which profile field this claim fills is the judgement the closed enum
            encodes — the model returned prose, and only a human decides it means
            "Role: X". Picking a field reveals the value and citation it will be
            stored under, prefilled from the run and editable. */}
        <div className="pe-claim-map">
          <Field label={t("contact.research.mapField")}>
            {(control) => (
              <Select
                {...control}
                value={mapping.field}
                placeholder={t("contact.research.mapFieldPlaceholder")}
                options={FIELD_KEYS.map((key) => ({
                  value: key,
                  label: t(`contact.research.field.${key}`),
                }))}
                onChange={(picked) =>
                  onMap({ field: FIELD_KEYS.find((k) => k === picked) ?? "" })
                }
              />
            )}
          </Field>
          {mapping.field !== "" && (
            <>
              <Field label={t("contact.research.mapValue")}>
                {(control) => (
                  <TextInput
                    {...control}
                    value={mapping.value}
                    onChange={(event) => onMap({ value: event.target.value })}
                  />
                )}
              </Field>
              <Field label={t("contact.research.mapQuote")}>
                {(control) => (
                  <TextInput
                    {...control}
                    value={mapping.quote}
                    onChange={(event) => onMap({ quote: event.target.value })}
                  />
                )}
              </Field>
              <Field
                label={t("contact.research.mapUrl")}
                error={badUrl ? t("contact.research.mapUrlInvalid") : undefined}
              >
                {(control) => (
                  <TextInput
                    {...control}
                    value={mapping.url}
                    onChange={(event) => onMap({ url: event.target.value })}
                  />
                )}
              </Field>
              {!complete && !badUrl && (
                <p className="pe-claim-incomplete t-caption">
                  {t("contact.research.mapIncomplete")}
                </p>
              )}
            </>
          )}
        </div>
      </div>
      <Button small onClick={onDismiss}>
        {t("contact.research.dismiss")}
      </Button>
    </article>
  );
}

export function ContactResearchDrawer({
  contactId,
  contactName,
  providerProfiles,
  open,
  onClose,
}: Readonly<{
  contactId: string;
  contactName: string;
  // What a licensed provider was PAID to tell us about this contact
  // (ADR-0101). Passed in rather than fetched here: the page already holds
  // the assembled 360, and a second read could disagree with what it shows.
  providerProfiles?: components["schemas"]["ContactProviderProfile"][];
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [dismissed, setDismissed] = useState<ReadonlySet<number>>(new Set());
  const [edits, setEdits] = useState<Record<number, Mapping>>({});

  // The drawer stays mounted while the page toggles `open`, so its edits and
  // dismissals would otherwise outlive the run they were made against — and a
  // later run reusing a claim's ordinal would inherit them, saving a stale value
  // or hiding a claim nobody dismissed. Clearing on close hands every reopen a
  // clean slate, whichever way it was closed (save, discard or Escape).
  useEffect(() => {
    if (!open) {
      setEdits({});
      setDismissed(new Set());
    }
  }, [open]);

  const run = useQuery({
    enabled: open,
    queryKey: ["contactResearch", contactId],
    queryFn: async () => {
      const { data, error } = await api.POST("/contacts/{id}/research", {
        params: { path: { id: contactId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const save = useMutation({
    // Takes the mapped claims as a variable rather than closing over render
    // state: the click belongs to the committed render, so what it carries
    // cannot be older than the button that carried it.
    mutationFn: async (claims: SaveClaim[]) => {
      const { data, error } = await api.POST("/contacts/{id}/research/save", {
        params: { path: { id: contactId } },
        body: { claims },
      });
      if (error) {
        throwProblem(error);
      }
      return data?.saved ?? claims.length;
    },
    onSuccess: async (saved) => {
      toast.show(
        plural("contact.research.saved", saved, {
          count: formatNumber(saved, locale),
        }),
      );
      await queryClient.invalidateQueries({
        queryKey: ["contactResearch", contactId],
      });
      onClose();
    },
  });

  const claims = (run.data?.claims ?? []).filter(
    (claim) => !dismissed.has(claim.ordinal),
  );
  const mappingFor = (claim: ResearchClaim): Mapping =>
    edits[claim.ordinal] ?? initialMapping(claim);
  const toSave = claims
    .map((claim) => toSaveClaim(mappingFor(claim)))
    .filter((claim): claim is SaveClaim => claim !== null);

  const patch = (claim: ResearchClaim, next: Partial<Mapping>) =>
    setEdits((prior) => ({
      ...prior,
      [claim.ordinal]: {
        ...(prior[claim.ordinal] ?? initialMapping(claim)),
        ...next,
      },
    }));
  const dismiss = (ordinal: number) =>
    setDismissed((prior) => new Set(prior).add(ordinal));

  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy="contact-research-title"
      size="wide"
      placement="right"
    >
      <div className="drawer-head">
        <div className="pe-drawer-title">
          <Heading size="large" id="contact-research-title">
            {t("contact.research.title", { name: contactName })}
          </Heading>
          <Button
            small
            iconOnly
            onClick={onClose}
            aria-label={t("contact.drawer.close")}
          >
            <X aria-hidden="true" />
          </Button>
        </div>
        <Badge>{t("contact.research.publicOnly")}</Badge>
      </div>

      <div className="drawer-body">
        {/* What was BOUGHT sits above what a public read found: it cost
            money, it is the firmer of the two, and a rep looking somebody up
            should see it before a page crawl's guesses. */}
        <ContactProviderSection
          contactId={contactId}
          profiles={providerProfiles}
        />

        {run.isLoading && (
          <p className="pe-prose t-body">{t("contact.research.running")}</p>
        )}

        {/* The honest empty state. Nothing was asked and nothing was read, so
            the drawer says so rather than showing an empty result that reads
            as "a provider looked and found nothing". */}
        {run.data?.state === "not_connected" && (
          <p className="pe-prose t-body">
            {t("contact.research.notConnected")}
          </p>
        )}

        {run.data?.state === "ready" && (
          <>
            <p className="pe-staged-notice">
              {t("contact.research.staged", { name: contactName })}
            </p>
            <p className="pe-today-foot t-caption">
              {t("contact.research.stats", {
                sources: formatNumber(run.data.sources_read ?? 0, locale),
                claims: formatNumber(claims.length, locale),
              })}
            </p>
            {claims.map((claim) => (
              <ClaimMapRow
                key={claim.ordinal}
                claim={claim}
                mapping={mappingFor(claim)}
                onMap={(next) => patch(claim, next)}
                onDismiss={() => dismiss(claim.ordinal)}
              />
            ))}
          </>
        )}
      </div>

      <div className="drawer-foot">
        <span className="pe-disclosure t-caption">
          {t("contact.research.evidenceOrOmit")}
        </span>
        <div className="pe-drawer-actions">
          <Button onClick={onClose}>{t("contact.research.discard")}</Button>
          <Button
            variant="primary"
            disabled={toSave.length === 0}
            pending={save.isPending}
            onClick={() => save.mutate(toSave)}
          >
            {plural("contact.research.save", toSave.length, {
              count: formatNumber(toSave.length, locale),
            })}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
