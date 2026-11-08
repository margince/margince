// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { confirmEffect } from "./worklist.domaincopy";
import { DomainReview } from "./worklist.domainreview";
import { type WorklistItem, worklistKey } from "./worklist.queries";

// Answering an undecided domain, from the row that asked.
//
// Its own module rather than more of worklist.row.tsx, the way the briefing
// queue's verbs and the duplicate pair's already are: a source's answer is a
// self-contained piece of work, and the row file decides how work READS rather
// than owning every control it can carry.
//
// The two verbs go to two different stores under two different gates — keeping
// creates the company the triage withheld, discarding writes one seat's own
// capture exclusion — so they are two hooks and not one with a flag.

/**
 * Keeping an undecided domain: the company the triage withheld gets created.
 *
 * The domain is the path, not an id, because that is what identifies an open
 * question — the disposition row's own id never reaches a client.
 */
export function useDomainQuestionKeep(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (domain: string) => {
      return unwrap(
        await api.POST("/capture/domain-questions/{domain}/keep", {
          params: { path: { domain } },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

/**
 * Discarding an undecided domain: the caller's OWN mail from it stops being
 * captured.
 *
 * A personal rule, never the workspace's. Excluding a domain for everybody is a
 * different act behind a different gate, and this hook cannot reach it — which
 * is deliberate, since the row it is pressed from belongs to one reader.
 */
export function useDomainQuestionDiscard(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (domain: string) => {
      return unwrap(
        await api.POST("/capture/domain-questions/{domain}/discard", {
          params: { path: { domain } },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

/**
 * Keeping a vendor verdict: the domain stays a mail or hosting vendor and no
 * company is created. Only a vendor question offers it.
 */
export function useDomainQuestionKeepVendor(
  invalidateKeys: readonly QueryKey[],
) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (domain: string) => {
      return unwrap(
        await api.POST("/capture/domain-questions/{domain}/keep-vendor", {
          params: { path: { domain } },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

/**
 * Confirming a suggested company: the same keep the review's match sends, with
 * the suggestion's id, so the domain and its contacts join that company.
 */
export function useDomainQuestionConfirm(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      domain,
      companyId,
    }: Readonly<{ domain: string; companyId: string }>) => {
      return unwrap(
        await api.POST("/capture/domain-questions/{domain}/keep", {
          params: { path: { domain } },
          body: { company_id: companyId },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

// No verb is drawn as the expected press: the machine could not tell which
// company this is, and a filled button would repeat its guess. A suggestion
// is confirmed by a person, never linked on its own.
//
// The DOMAIN is the row's id: an open question is identified by the domain
// itself.
//
// Each button asks whether the row offers it. The verbs are answered by
// different stores under different gates, and a control drawn past the
// server's offer is a button that 403s. Confirm and Choose need the company
// grant, because both link the domain to a company.
export function DomainQuestionAnswer({
  item,
}: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [review, setReview] = useState(false);
  const toast = useToast();
  const canLink = useCanWrite("company", "update");
  const keep = useDomainQuestionKeep([worklistKey]);
  const confirm = useDomainQuestionConfirm([worklistKey]);
  const discard = useDomainQuestionDiscard([worklistKey]);
  const keepVendor = useDomainQuestionKeepVendor([worklistKey]);
  const busy =
    keep.isPending ||
    confirm.isPending ||
    discard.isPending ||
    keepVendor.isPending;
  const domain = item.id;
  const { actions } = item;
  const context = item.domain_context;
  const suggestion = context?.suggested_company;
  const failed = (key: MessageKey) => () =>
    toast.show(t(key), { tone: "danger" });
  return (
    <>
      {suggestion && canLink && (
        <span className="t-caption">
          {t("worklist.domainQuestion.suggested", { name: suggestion.name })}{" "}
          {confirmEffect(
            { t, plural, locale },
            domain,
            suggestion.name,
            context,
          )}
        </span>
      )}
      {suggestion && canLink && (
        <Button
          pending={confirm.isPending}
          disabled={busy}
          onClick={() =>
            confirm.mutate(
              { domain, companyId: suggestion.id },
              {
                onSuccess: () =>
                  toast.show(t("worklist.domainQuestion.confirmed")),
                onError: failed("worklist.domainQuestion.confirmFailed"),
              },
            )
          }
        >
          {t("worklist.domainQuestion.confirm")}
        </Button>
      )}
      {canLink && (
        <Button disabled={busy} onClick={() => setReview(true)}>
          {t(
            suggestion
              ? "worklist.domainQuestion.chooseAnother"
              : "worklist.domainQuestion.choose",
          )}
        </Button>
      )}
      {review && (
        <DomainReview domain={domain} onClose={() => setReview(false)} />
      )}
      {actions.includes("keep") &&
      item.domain_reason_code !== "forwarded_identity" ? (
        <Button
          pending={keep.isPending}
          disabled={busy}
          onClick={() =>
            keep.mutate(domain, {
              onSuccess: () => toast.show(t("worklist.verb.domainKept")),
              onError: failed("worklist.verb.domainKeepFailed"),
            })
          }
        >
          {t("worklist.verb.keep")}
        </Button>
      ) : null}
      {actions.includes("keep_as_vendor") ? (
        <Button
          pending={keepVendor.isPending}
          disabled={busy}
          onClick={() =>
            keepVendor.mutate(domain, {
              onSuccess: () =>
                toast.show(t("worklist.verb.domainKeptAsVendor")),
              onError: failed("worklist.verb.domainKeepAsVendorFailed"),
            })
          }
        >
          {t("worklist.verb.keepAsVendor")}
        </Button>
      ) : null}
      {actions.includes("discard") ? (
        <Button
          variant="ghost"
          pending={discard.isPending}
          disabled={busy}
          title={t("worklist.domainQuestion.stopCapturingEffect", { domain })}
          onClick={() =>
            discard.mutate(domain, {
              onSuccess: () => toast.show(t("worklist.verb.domainDiscarded")),
              onError: failed("worklist.verb.domainDiscardFailed"),
            })
          }
        >
          {t("worklist.verb.discard")}
        </Button>
      ) : null}
    </>
  );
}
