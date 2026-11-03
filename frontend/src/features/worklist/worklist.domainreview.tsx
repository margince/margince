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
import { AppLink } from "../../design-system/applink";
import {
  Button,
  EmptyState,
  Modal,
  PendingBody,
} from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { formatDate, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { searchByEntity } from "../../lib/relationshipcandidates";
import { routeHash } from "../../lib/router";
import {
  confirmEffect,
  type DomainQuestionContext,
  domainQuestionHeading,
  domainQuestionReason,
  suggestionBasis,
} from "./worklist.domaincopy";
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

// With a domain, the drawer holds that one question, the one its row asked.
// Without one it is the whole review queue, opened from the batch row.
export function DomainReview({
  domain,
  onClose,
}: Readonly<{ domain?: string; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const query = useInfiniteQuery({
    queryKey: [...domainReviewKey, domain ?? ""],
    initialPageParam: "",
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/capture/domain-questions", {
          params: {
            query: { limit: 50, cursor: pageParam || undefined, domain },
          },
        }),
      ),
    getNextPageParam: (page) => page.page.next_cursor ?? undefined,
  });
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId}>
          {t(
            domain
              ? "worklist.domainReview.focusedTitle"
              : "worklist.domainReview.title",
          )}
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

// One sentence per action the panel draws, so each press says what it does.
function actionEffects(row: DomainQuestion): MessageKey[] {
  const effects: MessageKey[] = ["worklist.domainReview.matchEffect"];
  if (row.reason_code !== "forwarded_identity")
    effects.push("worklist.domainReview.createEffect");
  effects.push(
    row.reason_code === "vendor_verdict"
      ? "worklist.domainReview.keepVendorEffect"
      : "worklist.domainReview.retryEffect",
  );
  effects.push("worklist.domainReview.deferEffect");
  return effects;
}

function useAnswer() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: postAnswer,
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: domainReviewKey }),
        client.invalidateQueries({ queryKey: worklistKey }),
      ]);
    },
  });
}

function DomainReviewRow({ row }: Readonly<{ row: DomainQuestion }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const client = useQueryClient();
  const canEdit = useCanWrite("company", "update");
  const defer = useMarkRowsDone();
  const deferred = row.deferred;
  const [company, setCompany] = useState<RecordPickerCandidate | null>(null);
  const answer = useAnswer();
  const context = row.domain_context;
  const suggestion = context?.suggested_company;
  const vendor = row.reason_code === "vendor_verdict";
  const send = (verb: Answer["verb"], companyId?: string) =>
    answer.mutate({ domain: row.domain, verb, companyId });
  return (
    <Panel title={domainQuestionHeading(t, row.domain, context)}>
      <PanelBody>
        <DomainQuestionOrigin domain={row.domain} context={context} />
        <p>{domainQuestionReason(t, row.reason_code, context)}</p>
        {suggestion && (
          <>
            <p>
              {t("worklist.domainQuestion.suggested", {
                name: suggestion.name,
              })}{" "}
              {suggestionBasis(t, suggestion)}
            </p>
            <ActionRow>
              <Button
                variant="primary"
                disabled={!canEdit || answer.isPending}
                onClick={() => send("keep", suggestion.id)}
              >
                {t("worklist.domainQuestion.confirm")}
              </Button>
            </ActionRow>
            <p className="t-caption">
              {confirmEffect(
                { t, plural, locale },
                row.domain,
                suggestion.name,
                context,
              )}
            </p>
          </>
        )}
        <RecordPicker
          label={t(
            suggestion
              ? "worklist.domainQuestion.chooseAnother"
              : "worklist.domainQuestion.choose",
          )}
          searchTargets={searchCompanies}
          selected={company}
          onPick={setCompany}
          disabled={!canEdit || answer.isPending}
        />
        <ErrorLine error={answer.error ?? defer.error} />
        <ActionRow>
          <Button
            disabled={!canEdit || !company || answer.isPending}
            onClick={() => send("keep", company?.id)}
          >
            {t("worklist.domainReview.match")}
          </Button>
          {row.reason_code !== "forwarded_identity" && (
            <Button
              disabled={!canEdit || answer.isPending}
              onClick={() => send("keep")}
            >
              {t("worklist.domainReview.create")}
            </Button>
          )}
          {vendor ? (
            <Button
              disabled={!canEdit || answer.isPending}
              onClick={() => send("keep_vendor")}
            >
              {t("worklist.domainReview.keepVendor")}
            </Button>
          ) : (
            <Button
              disabled={!canEdit || answer.isPending}
              onClick={() => send("reopen")}
            >
              {t("worklist.domainReview.retry")}
            </Button>
          )}
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
        </ActionRow>
        <ul className="t-caption">
          {actionEffects(row).map((key) => (
            <li key={key}>{t(key)}</li>
          ))}
        </ul>
        <ActionRow>
          <Button
            variant="ghost"
            disabled={answer.isPending}
            onClick={() => send("discard")}
          >
            {t("worklist.verb.discard")}
          </Button>
        </ActionRow>
        <p className="t-caption">
          {t("worklist.domainQuestion.stopCapturingEffect", {
            domain: row.domain,
          })}
        </p>
        <WebsiteDetails row={row} />
      </PanelBody>
    </Panel>
  );
}

// Diagnostics the task does not need, closed by default. They are the triage's
// note, the answering site, the redirects and the evidence date.
function WebsiteDetails({ row }: Readonly<{ row: DomainQuestion }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Disclosure summary={t("worklist.domainReview.details")}>
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
    </Disclosure>
  );
}

// Who raised the question, as links to the contacts the reader may open. A
// sender the reader may not see is said to exist, never named.
function DomainQuestionOrigin({
  domain,
  context,
}: Readonly<{ domain: string; context: DomainQuestionContext | undefined }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const contacts = context?.contacts ?? [];
  const more = (context?.contact_count ?? 0) - contacts.length;
  let lead: string;
  if (contacts.length > 0)
    lead = t("worklist.domainQuestion.originWith", { domain });
  else if (context?.hidden_senders)
    lead = t("worklist.domainQuestion.originHidden", { domain });
  else if (context?.sender_address)
    lead = t("worklist.domainQuestion.originFrom", {
      domain,
      address: context.sender_address,
    });
  else lead = t("worklist.domainQuestion.originUnknown", { domain });
  return (
    <p>
      {lead}{" "}
      {contacts.map((contact, index) => (
        <span key={contact.id}>
          {index > 0 && ", "}
          <AppLink href={routeHash({ screen: "contacts", id: contact.id })}>
            {contact.name}
          </AppLink>
        </span>
      ))}
      {more > 0 &&
        ` ${plural("worklist.domainQuestion.andMore", more, { count: formatNumber(more, locale) })}`}
    </p>
  );
}
