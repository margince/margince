// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useId, useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { CountLine } from "../design-system/listsurface";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  ADMISSION_LABEL,
  BLANK_DECISION,
  type BlockedDomain,
  type Decision,
  DecisionDialog,
  type SetDecision,
  useSetBlockedDomain,
} from "./blocked-domains-decision";
import { problemMessageOf, QueryGate, throwProblem, unwrap } from "./common";

// The domains this installation refuses a company (ADR-0072). A vendor the
// business merely USES has a real corporate website, so every piece of evidence
// a crawl can gather says "company" — only a standing decision says otherwise,
// which is why the refusal lives on the domain rather than on any sender or
// read.
//
// The card exists for one question an operator cannot otherwise answer: a
// company that never appeared — was it refused, and by whom? So `source` is a
// first-class column, not a detail: a bulk-sender verdict and somebody's
// deliberate call look identical in the outcome and are completely different
// facts. Every human role reads the list (`company:read`); changing an
// entry demands `company:update`, so the verb is refused rather than
// hidden, like the capture cards beside it.
//
// The write is a PUT that is idempotent on the normalized domain: there is no
// version to quote and none to send, so no `ifMatch` here — an entry for a
// domain already on the list REPLACES it, which is also how a refusal is undone.

// What decided a row — or, for an open question, what stopped the machine
// deciding. The two are one column because they answer one thing a reader wants
// to know: why this domain stands where it does.
const SOURCE_LABEL: Record<BlockedDomain["source"], MessageKey> = {
  verdict: "blockedDomains.source.verdict",
  heuristic: "blockedDomains.source.heuristic",
  human: "blockedDomains.source.human",
  unevidenced: "blockedDomains.source.unevidenced",
  stale_evidence: "blockedDomains.source.staleEvidence",
  near_duplicate: "blockedDomains.source.nearDuplicate",
};

function useBlockedDomains() {
  return useQuery({
    queryKey: ["blocked-domains"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/capture/blocked-domains",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * Asking again about a domain the machine gave up on.
 *
 * No body and no reason: a re-ask asserts nothing about what the domain IS, so
 * there is nothing for a reader to review. That is what separates it from the
 * decision write above, which demands a sentence precisely because it settles
 * something.
 */
function useReopenDomain() {
  const queryClient = useQueryClient();
  return useMutation({
    // The domain arrives as a variable rather than through a closure, for the
    // reason every other write on this card does: the handler belongs to the
    // committed render, so what it sends cannot be older than the row the
    // operator pressed.
    mutationFn: async (domain: string) => {
      return unwrap(
        await api.POST("/capture/blocked-domains/{domain}/reopen", {
          params: { path: { domain } },
        }),
      );
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["blocked-domains"] });
    },
  });
}

/** Likewise for the re-ask, so the column and the card agree on its shape. */
type ReopenDomain = ReturnType<typeof useReopenDomain>;

export function BlockedDomainsCard() {
  const t = useT();
  const { locale } = useLocale();
  // An operator investigating a company that never appeared reads the decision
  // time on their own wall clock, the same choice the audit trail makes — a
  // fixed installation zone would put the moment they are correlating against
  // an hour they were not working.
  const zone = viewerZone();
  const canManage = useCanWrite("company", "update");
  const query = useBlockedDomains();
  const set = useSetBlockedDomain();
  const reopen = useReopenDomain();
  // The decision being written, and the dialog's own open state: one piece of
  // state rather than two, because a dialog that is open with nothing in it is
  // a state this card cannot be in.
  const [editing, setEditing] = useState<Decision | null>(null);
  // The denial, said once and POINTED AT — `Button`'s `reasonId` refuses the
  // control and names the one sentence already on the page, which is what a
  // screen reader hears from the control it left them on. The id is minted
  // unconditionally, because a hook may not depend on a permission.
  const denialId = useId();
  const refusal = canManage ? undefined : denialId;

  // Revising a standing decision starts on its row and finishes in the dialog.
  // The contract requires a reason for every write, so a one-click flip has
  // nowhere to get one — and a refusal nobody can explain is one nobody can
  // review, which is the whole point of the column. The row therefore opens
  // the dialog on the domain with the OPPOSITE decision and no reason, so what
  // is left to do is type the sentence: that is the McKinsey case, a newsletter
  // publisher that became a client.
  const revise = useCallback((entry: BlockedDomain) => {
    setEditing({
      domain: entry.domain,
      admission: entry.admission === "suppressed" ? "admitted" : "suppressed",
      reason: "",
    });
  }, []);

  const hasOutcome =
    !canManage ||
    set.data !== undefined ||
    reopen.data !== undefined ||
    reopen.isError;

  return (
    <Panel
      title={t("blockedDomains.title")}
      // The card's one create verb rides in the header, not in a row: a row
      // states a setting and its answer, and this row's LABEL was its own
      // button's words. It also keeps the verb still while the list below it
      // grows. Refused rather than hidden, like every other control on this
      // card — the sentence under the list is what `reasonId` names.
      titleAction={
        <Button reasonId={refusal} onClick={() => setEditing(BLANK_DECISION)}>
          {t("blockedDomains.recordOpen")}
        </Button>
      }
    >
      <PanelBody>
        <PanelIntro>{t("blockedDomains.sub")}</PanelIntro>
      </PanelBody>
      <PanelGroupHead title={t("blockedDomains.listTitle")} level="h3" />
      <QueryGate query={query} pendingLabel={t("blockedDomains.listTitle")}>
        {(list) =>
          list.data.length === 0 ? (
            // `empty`, and only `empty`: no decision has been recorded, which
            // is a fact about the installation rather than a read that failed.
            <EmptyState>{t("blockedDomains.none")}</EmptyState>
          ) : (
            <>
              <DataTable
                bleed
                label={t("blockedDomains.listTitle")}
                columns={decisionColumns({
                  t,
                  locale,
                  zone,
                  revise,
                  refusal,
                  set,
                  reopen,
                })}
                rows={list.data}
                rowKey={(row) => row.domain}
              />
              {/* The server pages the list, so the count tells "not refused"
                  from "past the end of this page". */}
              <PanelBody>
                <p className="t-caption">
                  <CountLine
                    unit={t("blockedDomains.unit")}
                    first={1}
                    last={list.data.length}
                    total={list.total}
                  />
                </p>
              </PanelBody>
            </>
          )
        }
      </QueryGate>
      {hasOutcome && (
        <PanelBody className="form-stack">
          {!canManage && <p id={denialId}>{t("blockedDomains.adminOnly")}</p>}
          {/* The server stores the registrable domain and replaces any entry on
              it, so the card names what landed after the dialog has gone. */}
          {set.data && (
            <Callout
              tone="success"
              kind="outcome"
              // One short sentence, so the heading says all of it.
              title={t("blockedDomains.stored", {
                domain: set.data.domain,
                admission: t(ADMISSION_LABEL[set.data.admission]),
              })}
            />
          )}
          {/* The row stays undecided until a crawl answers, so this says the
              press landed. */}
          {reopen.data && (
            <Callout
              tone="success"
              kind="outcome"
              title={t("blockedDomains.reopened", {
                domain: reopen.data.domain,
              })}
            />
          )}
          {reopen.isError && (
            <Callout
              tone="danger"
              kind="outcome"
              title={t("blockedDomains.reopenFailed")}
            >
              {problemMessageOf(reopen.error, t)}
            </Callout>
          )}
        </PanelBody>
      )}
      {editing !== null && (
        <DecisionDialog
          initial={editing}
          set={set}
          onClose={() => setEditing(null)}
        />
      )}
    </Panel>
  );
}

/**
 * The table's columns.
 *
 * A function of what they need rather than a constant, because two of them
 * render translated copy and one renders a control whose refusal is the
 * reader's — none of which is knowable at module scope.
 */
function decisionColumns({
  t,
  locale,
  zone,
  revise,
  refusal,
  set,
  reopen,
}: Readonly<{
  t: ReturnType<typeof useT>;
  locale: Locale;
  zone: string;
  revise: (entry: BlockedDomain) => void;
  refusal: string | undefined;
  set: SetDecision;
  reopen: ReopenDomain;
}>) {
  return [
    {
      key: "domain",
      header: t("blockedDomains.col.domain"),
      render: (row: BlockedDomain) => (
        <>
          <span>{row.domain}</span>
          {/* The company an admitted domain produced, when there is one. A
              link rather than the id it is built from: the payload carries no
              name, and printing a UUID at an operator is not a fact they can
              use. */}
          {row.company_id != null && (
            <>
              {" "}
              <a href={`#/companies/${row.company_id}`}>
                {t("blockedDomains.openCompany")}
              </a>
            </>
          )}
        </>
      ),
    },
    {
      key: "admission",
      header: t("blockedDomains.col.admission"),
      render: (row: BlockedDomain) => (
        <Badge tone={admissionTone(row.admission)}>
          {t(ADMISSION_LABEL[row.admission])}
        </Badge>
      ),
    },
    {
      key: "source",
      header: t("blockedDomains.col.source"),
      // A human decision is the one an operator is hunting for, so it is the
      // one the eye finds: the machine sources read as plain pills beside it.
      render: (row: BlockedDomain) => (
        <Badge tone={row.source === "human" ? "accent" : undefined}>
          {t(SOURCE_LABEL[row.source])}
        </Badge>
      ),
    },
    {
      key: "reason",
      header: t("blockedDomains.col.reason"),
      render: (row: BlockedDomain) => row.reason,
    },
    {
      key: "decided",
      header: t("blockedDomains.col.decided"),
      render: (row: BlockedDomain) => (
        <time dateTime={row.decided_at}>
          {formatDate(row.decided_at, locale, zone)}
        </time>
      ),
    },
    {
      key: "revise",
      header: t("blockedDomains.col.revise"),
      // An undecided row gets a different verb, because it is a different act:
      // the other two REPLACE a decision and owe a reason, while this one only
      // asks the crawl to look again. Offering "Allow this one" on a domain
      // nobody has judged would invite a decision the operator has no grounds
      // for — the whole reason the row is here is that nothing knows yet.
      render: (row: BlockedDomain) =>
        row.admission === "undecided" ? (
          <Button
            variant="ghost"
            disabled={reopen.isPending}
            reasonId={refusal}
            onClick={() => reopen.mutate(row.domain)}
          >
            {t("blockedDomains.rowReopen")}
          </Button>
        ) : (
          <Button
            variant="ghost"
            disabled={set.isPending}
            reasonId={refusal}
            onClick={() => revise(row)}
          >
            {t(
              row.admission === "suppressed"
                ? "blockedDomains.rowAdmit"
                : "blockedDomains.rowRefuse",
            )}
          </Button>
        ),
    },
  ];
}

/**
 * How each standing reads at a glance. An open question is neither an outcome
 * nor a warning — nothing went wrong and nothing was settled — so it takes the
 * neutral tone and leaves the eye to the two that were decided.
 */
function admissionTone(
  admission: BlockedDomain["admission"],
): "success" | "warning" | undefined {
  switch (admission) {
    case "admitted":
      return "success";
    case "suppressed":
      return "warning";
    default:
      return undefined;
  }
}
