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
import { PersonProviderSection } from "./personprovider";

// The research drawer the person page opens over itself.
//
// The WIDE drawer — a rep works in it rather than glancing at it — and it leaves
// the page behind visible, because the record is the context that makes the
// drawer's content mean anything. Writing to the contact used to be a second
// drawer in this file; it is compose.tsx's now, which is the one composer every
// record in the product opens.

export function PersonResearchDrawer({
  personId,
  personName,
  providerProfiles,
  open,
  onClose,
}: Readonly<{
  personId: string;
  personName: string;
  // What a licensed provider was PAID to tell us about this person
  // (ADR-0101). Passed in rather than fetched here: the page already holds
  // the assembled 360, and a second read could disagree with what it shows.
  providerProfiles?: components["schemas"]["PersonProviderProfile"][];
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [dismissed, setDismissed] = useState<ReadonlySet<number>>(new Set());

  const run = useQuery({
    enabled: open,
    queryKey: ["personResearch", personId],
    queryFn: async () => {
      const { data, error } = await api.POST("/people/{id}/research", {
        params: { path: { id: personId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const save = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST("/people/{id}/research/save", {
        params: { path: { id: personId } },
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
      labelledBy="person-research-title"
      size="wide"
      placement="right"
    >
      <div className="drawer-head">
        <div className="pe-drawer-title">
          <h2 id="person-research-title">
            {t("person.research.title", { name: personName })}
          </h2>
          <Button
            small
            iconOnly
            onClick={onClose}
            aria-label={t("person.drawer.close")}
          >
            <X aria-hidden="true" />
          </Button>
        </div>
        <Badge>{t("person.research.publicOnly")}</Badge>
      </div>

      <div className="drawer-body">
        {/* What was BOUGHT sits above what a public read found: it cost
            money, it is the firmer of the two, and a rep looking somebody up
            should see it before a page crawl's guesses. */}
        <PersonProviderSection
          personId={personId}
          profiles={providerProfiles}
        />

        {run.isLoading && (
          <p className="pe-prose t-body">{t("person.research.running")}</p>
        )}

        {/* The honest empty state. Nothing was asked and nothing was read, so
            the drawer says so rather than showing an empty result that reads
            as "a provider looked and found nothing". */}
        {run.data?.state === "not_connected" && (
          <p className="pe-prose t-body">{t("person.research.notConnected")}</p>
        )}

        {run.data?.state === "ready" && (
          <>
            <p className="pe-staged-notice">
              {t("person.research.staged", { name: personName })}
            </p>
            <p className="pe-today-foot t-caption">
              {t("person.research.stats", {
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
                  {t("person.research.dismiss")}
                </Button>
              </article>
            ))}
          </>
        )}
      </div>

      <div className="drawer-foot">
        <span className="pe-disclosure t-caption">
          {t("person.research.evidenceOrOmit")}
        </span>
        <div className="pe-drawer-actions">
          <Button onClick={onClose}>{t("person.research.discard")}</Button>
          <Button
            variant="primary"
            disabled={claims.length === 0 || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("person.research.save", {
              count: formatNumber(claims.length, locale),
            })}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
