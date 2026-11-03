// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { ActionRow } from "../../design-system/actionrow";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { useToast } from "../../design-system/toast";
import { usePlural, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { recordRoute } from "../../lib/entity";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import {
  confirmEffect,
  type DomainQuestionView,
  domainQuestionHeading,
  domainQuestionReasonKey,
  SUGGESTION_BASIS,
} from "./worklist.domaincopy";
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
 * Keeping an undecided domain: the company the triage withheld gets created,
 * or, with a company, the domain and its contacts join that company.
 *
 * The domain is the path, not an id, because that is what identifies an open
 * question — the disposition row's own id never reaches a client.
 */
export function useDomainQuestionKeep(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      domain,
      companyId,
    }: Readonly<{ domain: string; companyId?: string }>) => {
      return unwrap(
        await api.POST("/capture/domain-questions/{domain}/keep", {
          params: { path: { domain } },
          body: companyId ? { company_id: companyId } : undefined,
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

// The question under the row: who wrote, why Margince is asking, the company
// the evidence points to, and what each answer does.
//
// Each button asks whether the row offers it, because a control drawn past the
// server's offer is a button that 403s. Confirm and Choose another post keep
// with a company, which a redirected question accepts too.
export function DomainQuestionAnswer({
  item,
}: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const plural = usePlural();
  const [review, setReview] = useState(false);
  const toast = useToast();
  const canEdit = useCanWrite("company", "update");
  const keep = useDomainQuestionKeep([worklistKey]);
  const discard = useDomainQuestionDiscard([worklistKey]);
  const keepVendor = useDomainQuestionKeepVendor([worklistKey]);
  const busy = keep.isPending || discard.isPending || keepVendor.isPending;
  const domain = item.id;
  const { actions } = item;
  const view: DomainQuestionView = {
    domain,
    reasonCode: item.domain_reason_code,
    context: item.domain_context,
  };
  const suggestion = item.domain_context?.suggested_company;
  const answerable =
    actions.includes("keep") ||
    actions.includes("reopen") ||
    actions.includes("keep_as_vendor");
  const create =
    actions.includes("keep") &&
    item.domain_reason_code !== "forwarded_identity";
  return (
    <div className="form-stack">
      <Heading size="small" as="h3">
        {domainQuestionHeading(view, t, plural)}
      </Heading>
      <DomainQuestionOrigin view={view} />
      <p>{t(domainQuestionReasonKey(view), { domain })}</p>
      {suggestion ? (
        <p>
          {t("worklist.domainQuestion.suggested")}{" "}
          <AppLink href={recordHref("company", suggestion.id)}>
            {suggestion.name}
          </AppLink>
          <br />
          <span className="t-caption">
            {t(SUGGESTION_BASIS[suggestion.basis])}
          </span>
        </p>
      ) : (
        <p className="t-caption">{t("worklist.domainQuestion.noSuggestion")}</p>
      )}
      <ActionRow
        primary={
          suggestion &&
          answerable &&
          canEdit && (
            <Button
              variant="primary"
              pending={keep.isPending}
              disabled={busy}
              onClick={() =>
                keep.mutate(
                  { domain, companyId: suggestion.id },
                  {
                    onSuccess: () =>
                      toast.show(
                        t("worklist.domainQuestion.confirmed", {
                          domain,
                          company: suggestion.name,
                        }),
                      ),
                    onError: () =>
                      toast.show(t("worklist.domainQuestion.confirmFailed"), {
                        tone: "danger",
                      }),
                  },
                )
              }
            >
              {t("worklist.domainQuestion.confirm")}
            </Button>
          )
        }
      >
        {answerable && (
          <Button disabled={busy} onClick={() => setReview(true)}>
            {t(
              suggestion
                ? "worklist.domainQuestion.chooseOther"
                : "worklist.domainQuestion.choose",
            )}
          </Button>
        )}
        {create && (
          <Button
            pending={keep.isPending}
            disabled={busy || !canEdit}
            onClick={() =>
              keep.mutate(
                { domain },
                {
                  onSuccess: () => toast.show(t("worklist.verb.domainKept")),
                  onError: () =>
                    toast.show(t("worklist.verb.domainKeepFailed"), {
                      tone: "danger",
                    }),
                },
              )
            }
          >
            {t("worklist.verb.keep")}
          </Button>
        )}
        {actions.includes("keep_as_vendor") && (
          <Button
            pending={keepVendor.isPending}
            disabled={busy}
            onClick={() =>
              keepVendor.mutate(domain, {
                onSuccess: () =>
                  toast.show(t("worklist.verb.domainKeptAsVendor")),
                onError: () =>
                  toast.show(t("worklist.verb.domainKeepAsVendorFailed"), {
                    tone: "danger",
                  }),
              })
            }
          >
            {t("worklist.verb.keepAsVendor")}
          </Button>
        )}
        {actions.includes("discard") && (
          <Button
            pending={discard.isPending}
            disabled={busy}
            onClick={() =>
              discard.mutate(domain, {
                onSuccess: () => toast.show(t("worklist.verb.domainDiscarded")),
                onError: () =>
                  toast.show(t("worklist.verb.domainDiscardFailed"), {
                    tone: "danger",
                  }),
              })
            }
          >
            {t("worklist.domainQuestion.stop")}
          </Button>
        )}
      </ActionRow>
      {suggestion && answerable && canEdit && (
        <p className="t-caption">
          {confirmEffect(view, suggestion.name, t, plural)}
        </p>
      )}
      {actions.includes("discard") && (
        <p className="t-caption">
          {t("worklist.domainQuestion.stopEffect", { domain })}
        </p>
      )}
      {review && (
        <DomainReview domain={domain} onClose={() => setReview(false)} />
      )}
    </div>
  );
}

// Where the question came from: the contacts the reader may open, or a plain
// statement that the sender is not one of them.
function DomainQuestionOrigin({
  view,
}: Readonly<{ view: DomainQuestionView }>) {
  const t = useT();
  const contacts = view.context?.contacts ?? [];
  if (contacts.length === 0) {
    return (
      <p className="t-caption">
        {t(
          view.context?.hidden_senders
            ? "worklist.domainQuestion.originHidden"
            : "worklist.domainQuestion.origin",
          { domain: view.domain },
        )}
      </p>
    );
  }
  return (
    <p className="t-caption">
      {t("worklist.domainQuestion.originContacts", { domain: view.domain })}{" "}
      {contacts.map((contact, index) => (
        <span key={contact.id}>
          {index > 0 && ", "}
          <AppLink href={recordHref("contact", contact.id)}>
            {contact.name}
          </AppLink>
        </span>
      ))}
    </p>
  );
}

function recordHref(kind: "company" | "contact", id: string) {
  const route = recordRoute(kind, id);
  return route && routeHash(route);
}
