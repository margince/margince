// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { throwProblem } from "./common";
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
      const { data, error } = await api.POST(
        "/capture/domain-questions/{domain}/keep",
        { params: { path: { domain } } },
      );
      if (error) {
        throwProblem(error, t);
      }
      return data;
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
      const { data, error } = await api.POST(
        "/capture/domain-questions/{domain}/discard",
        { params: { path: { domain } } },
      );
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

// TWO VERBS OF EQUAL WEIGHT, so neither is the row's primary. The machine could
// not tell whether this domain is a company, and two colleagues sharing an
// installation may answer it opposite ways — so drawing one of them as the
// expected press would be the same guess that left the question open.
//
// The DOMAIN is the row's id: an open question is identified by the domain
// itself.
//
// EACH BUTTON ASKS WHETHER THE ROW OFFERS IT, rather than the pair being drawn
// because the source is this one. The verbs travel together today, but they are
// answered by different stores under different gates, so a reader who may do one
// and not the other is a real case — and a control drawn past the server's offer
// is a button that 403s.
export function DomainQuestionAnswer({
  item,
}: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const keep = useDomainQuestionKeep([worklistKey]);
  const discard = useDomainQuestionDiscard([worklistKey]);
  const busy = keep.isPending || discard.isPending;
  const domain = item.id;
  const { actions } = item;
  return (
    <>
      {actions.includes("keep") ? (
        <Button
          pending={keep.isPending}
          disabled={busy}
          onClick={() =>
            keep.mutate(domain, {
              onSuccess: () => toast.show(t("worklist.verb.domainKept")),
              onError: () =>
                toast.show(t("worklist.verb.domainKeepFailed"), {
                  tone: "danger",
                }),
            })
          }
        >
          {t("worklist.verb.keep")}
        </Button>
      ) : null}
      {actions.includes("discard") ? (
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
          {t("worklist.verb.discard")}
        </Button>
      ) : null}
    </>
  );
}
