// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery } from "@tanstack/react-query";
import { ExternalLink, X } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, Modal } from "../design-system/atoms";
import { formatNumber, ordinalNumber } from "../format/format";
import { webUrl } from "../format/weburl";
import { useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import { ContactProviderSection } from "./contactprovider";

// The research drawer the contact page opens over itself.
//
// The WIDE drawer — a rep works in it rather than glancing at it — and it leaves
// the page behind visible, because the record is the context that makes the
// drawer's content mean anything. Writing to the contact used to be a second
// drawer in this file; it is compose.tsx's now, which is the one composer every
// record in the product opens.

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
  const { locale } = useLocale();
  const [dismissed, setDismissed] = useState<ReadonlySet<number>>(new Set());

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
    mutationFn: async () => {
      const { error } = await api.POST("/contacts/{id}/research/save", {
        params: { path: { id: contactId } },
        body: { claims: [] },
      });
      if (error) {
        throwProblem(error);
      }
    },
  });

  const claims = (run.data?.claims ?? []).filter(
    (claim) => !dismissed.has(claim.ordinal),
  );

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
          <h2 id="contact-research-title">
            {t("contact.research.title", { name: contactName })}
          </h2>
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
              <article className="pe-claim" key={claim.ordinal}>
                <span className="pe-claim-ordinal">
                  {ordinalNumber(claim.ordinal)}
                </span>
                <div>
                  <p className="pe-claim-body">{claim.body}</p>
                  <div className="pe-chiprow">
                    {/* A source URL comes from a THIRD-PARTY provider, so it
                        is untrusted: an unchecked href admits javascript: and
                        data: schemes, which execute on click. Only http(s)
                        becomes a link; anything else renders as inert text so
                        the reader still sees what was claimed, without a
                        clickable payload. */}
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
                        <span
                          key={source.url}
                          className="pe-memory-channel t-caption"
                        >
                          {source.label}
                        </span>
                      ),
                    )}
                    <Badge
                      tone={claim.confidence === "high" ? "success" : "warn"}
                    >
                      {claim.confidence}
                    </Badge>
                  </div>
                </div>
                <Button
                  small
                  onClick={() =>
                    setDismissed((prior) => new Set(prior).add(claim.ordinal))
                  }
                >
                  {t("contact.research.dismiss")}
                </Button>
              </article>
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
            disabled={claims.length === 0 || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("contact.research.save", {
              count: formatNumber(claims.length, locale),
            })}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
