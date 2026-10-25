// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import {
  Button,
  EmptyState,
  Modal,
  PendingBody,
} from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { searchByEntity } from "../../lib/relationshipcandidates";
import { domainReasonLabels } from "./worklist.domainreason";
import { useMarkRowsDone } from "./worklist.donequeries";
import { worklistKey } from "./worklist.queries";

const domainReviewKey = ["domain-questions"];

function searchCompanies(query: string) {
  return searchByEntity("company", query);
}

export function DomainReviewLauncher() {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        {t("worklist.domainReview.title")}
      </Button>
      {open && <DomainReview onClose={() => setOpen(false)} />}
    </>
  );
}

export function DomainReview({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const query = useInfiniteQuery({
    queryKey: domainReviewKey,
    initialPageParam: "",
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/capture/domain-questions", {
          params: { query: { limit: 50, cursor: pageParam || undefined } },
        }),
      ),
    getNextPageParam: (page) => page.page.next_cursor ?? undefined,
  });
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId}>
          {t("worklist.domainReview.title")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        {query.isPending && (
          <PendingBody label={t("worklist.domainReview.loading")} />
        )}
        <ErrorLine error={query.error} />
        {!query.isPending && !query.error && rows.length === 0 && (
          <EmptyState>{t("worklist.domainReview.empty")}</EmptyState>
        )}
        <div className="form-stack">
          {rows.map((row) => (
            <DomainReviewRow key={row.domain} row={row} />
          ))}
        </div>
        {query.hasNextPage && (
          <Button
            pending={query.isFetchingNextPage}
            onClick={() => query.fetchNextPage()}
          >
            {t("worklist.domainReview.more")}
          </Button>
        )}
      </DrawerBody>
    </Modal>
  );
}

type DomainQuestion = components["schemas"]["DomainQuestion"];
type Answer = {
  domain: string;
  verb: "keep" | "keep_vendor" | "discard" | "reopen";
  companyId?: string;
};

async function postAnswer({ domain, verb, companyId }: Answer) {
  if (verb === "reopen")
    return unwrap(
      await api.POST("/capture/blocked-domains/{domain}/reopen", {
        params: { path: { domain } },
      }),
    );
  if (verb === "keep_vendor")
    return unwrap(
      await api.POST("/capture/domain-questions/{domain}/keep-vendor", {
        params: { path: { domain } },
      }),
    );
  if (verb === "discard")
    return unwrap(
      await api.POST("/capture/domain-questions/{domain}/discard", {
        params: { path: { domain } },
      }),
    );
  return unwrap(
    await api.POST("/capture/domain-questions/{domain}/keep", {
      params: { path: { domain } },
      body: { company_id: companyId },
    }),
  );
}

function effectsOf(reasonCode: string) {
  if (reasonCode === "forwarded_identity")
    return "worklist.domainReview.forwardedEffects";
  if (reasonCode === "vendor_verdict")
    return "worklist.domainReview.vendorEffects";
  return "worklist.domainReview.effects";
}

function DomainReviewRow({ row }: Readonly<{ row: DomainQuestion }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const canEdit = useCanWrite("company", "update");
  const defer = useMarkRowsDone();
  const deferred = row.deferred;
  const [company, setCompany] = useState<RecordPickerCandidate | null>(null);
  const answer = useMutation({
    mutationFn: postAnswer,
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: domainReviewKey }),
        client.invalidateQueries({ queryKey: worklistKey }),
      ]);
    },
  });
  const reason = domainReasonLabels[row.reason_code];
  const vendor = row.reason_code === "vendor_verdict";
  return (
    <Panel title={row.domain}>
      <PanelBody>
        {reason && <p>{t(reason)}</p>}
        {row.note && <p className="t-caption">{row.note}</p>}
        {row.effective_url && (
          <p>
            {t("worklist.domainReview.forwarded", { url: row.effective_url })}
          </p>
        )}
        {row.redirect_chain && (
          <p className="t-caption">{row.redirect_chain.join(" → ")}</p>
        )}
        <p className="t-caption">
          {row.evidence_at
            ? t("worklist.domainReview.evidenceAt", {
                date: formatDate(row.evidence_at, locale, viewerZone()),
              })
            : t("worklist.domainReview.noEvidenceDate")}
        </p>
        <RecordPicker
          label={t("worklist.domainReview.match")}
          searchTargets={searchCompanies}
          selected={company}
          onPick={setCompany}
          disabled={!canEdit || answer.isPending}
        />
        <p className="t-caption">{t("worklist.domainReview.matchEffect")}</p>
        <ErrorLine error={answer.error ?? defer.error} />
        <ActionRow>
          <Button
            pending={defer.isPending}
            onClick={() =>
              defer.mutate(
                {
                  rows: [{ source: "domain_question", row_id: row.domain }],
                  reopen: deferred,
                },
                {
                  onSuccess: () =>
                    client.invalidateQueries({ queryKey: domainReviewKey }),
                },
              )
            }
          >
            {t(
              deferred
                ? "worklist.domainReview.restore"
                : "worklist.domainReview.defer",
            )}
          </Button>
          <Button
            disabled={!canEdit || !company || answer.isPending}
            onClick={() =>
              answer.mutate({
                domain: row.domain,
                verb: "keep",
                companyId: company?.id,
              })
            }
          >
            {t("worklist.domainReview.match")}
          </Button>
          {row.reason_code !== "forwarded_identity" && (
            <Button
              disabled={!canEdit || answer.isPending}
              onClick={() =>
                answer.mutate({ domain: row.domain, verb: "keep" })
              }
            >
              {t("worklist.domainReview.create")}
            </Button>
          )}
          {vendor ? (
            <Button
              disabled={!canEdit || answer.isPending}
              onClick={() =>
                answer.mutate({ domain: row.domain, verb: "keep_vendor" })
              }
            >
              {t("worklist.domainReview.keepVendor")}
            </Button>
          ) : (
            <Button
              disabled={!canEdit || answer.isPending}
              onClick={() =>
                answer.mutate({ domain: row.domain, verb: "reopen" })
              }
            >
              {t("worklist.domainReview.retry")}
            </Button>
          )}
          <Button
            disabled={answer.isPending}
            onClick={() =>
              answer.mutate({ domain: row.domain, verb: "discard" })
            }
          >
            {t("worklist.domainReview.exclude")}
          </Button>
        </ActionRow>
        <p className="t-caption">{t(effectsOf(row.reason_code))}</p>
        <p className="t-caption">{t("worklist.domainReview.deferEffect")}</p>
      </PanelBody>
    </Panel>
  );
}
