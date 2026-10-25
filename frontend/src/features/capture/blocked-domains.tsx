// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useId, useState } from "react";
import { api } from "../../api/client";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { CountLine } from "../../design-system/listsurface";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Locale, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import {
  ADMISSION_LABEL,
  BLANK_DECISION,
  type BlockedDomain,
  type Decision,
  DecisionDialog,
  type SetDecision,
  useSetBlockedDomain,
} from "./blocked-domains-decision";
import "./blocked-domains.css";

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

// Who decided a row. An open question has no decider, so its source is the
// reason instead and the column says nobody.
const DECIDER_LABEL: Partial<Record<BlockedDomain["source"], MessageKey>> = {
  verdict: "blockedDomains.source.verdict",
  heuristic: "blockedDomains.source.heuristic",
  human: "blockedDomains.source.human",
};

// Why the domain stands where it does, in the reader's language. `stated`
// carries the decider's own words in `note`; `unknown` keeps the stored sentence.
const REASON_LABEL: Record<
  Exclude<BlockedDomain["reason_code"], "stated" | "bulk_sender" | "unknown">,
  MessageKey
> = {
  company_created: "blockedDomains.reason.companyCreated",
  kept_as_company: "blockedDomains.reason.keptAsCompany",
  unevidenced: "blockedDomains.source.unevidenced",
  stale_evidence: "blockedDomains.source.staleEvidence",
  near_duplicate: "blockedDomains.source.nearDuplicate",
  forwarded_identity: "blockedDomains.source.forwardedIdentity",
  vendor_verdict: "blockedDomains.source.vendorVerdict",
  reserved_domain: "blockedDomains.source.reservedDomain",
};

const BULK_LABEL: Record<
  NonNullable<BlockedDomain["sender_kind"]>,
  MessageKey
> = {
  newsletter: "blockedDomains.reason.bulkNewsletter",
  transactional: "blockedDomains.reason.bulkTransactional",
  spam: "blockedDomains.reason.bulkSpam",
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

  // Revising opens the dialog, because every write owes a reason a one-click
  // flip has nowhere to get.
  const revise = useCallback(
    (entry: BlockedDomain, admission: Decision["admission"]) => {
      setEditing({ domain: entry.domain, admission, reason: "" });
    },
    [],
  );

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
      <QueryGate query={query} pendingLabel={t("blockedDomains.listTitle")}>
        {(list) =>
          list.data.length === 0 ? (
            // `empty`, and only `empty`: no decision has been recorded, which
            // is a fact about the installation rather than a read that failed.
            <PanelBody>
              <PanelIntro>{t("blockedDomains.sub")}</PanelIntro>
              <EmptyState>{t("blockedDomains.none")}</EmptyState>
            </PanelBody>
          ) : (
            <>
              <PanelBody>
                <PanelIntro>{t("blockedDomains.sub")}</PanelIntro>
              </PanelBody>
              <DataTable
                bleed
                fold
                stickyFirst
                label={t("blockedDomains.listTitle")}
                columns={decisionColumns({
                  t,
                  locale,
                  zone,
                  verbs: { revise, refusal, set, reopen },
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

type RowVerbs = Readonly<{
  revise: (entry: BlockedDomain, admission: Decision["admission"]) => void;
  refusal: string | undefined;
  set: SetDecision;
  reopen: ReopenDomain;
}>;

function decisionColumns({
  t,
  locale,
  zone,
  verbs,
}: Readonly<{
  t: ReturnType<typeof useT>;
  locale: Locale;
  zone: string;
  verbs: RowVerbs;
}>): DataTableColumn<BlockedDomain>[] {
  return [
    {
      key: "domain",
      header: t("blockedDomains.col.domain"),
      render: (row) => (
        <CellStack>
          <span className="blocked-domains-domain">{row.domain}</span>
          {row.company_id != null && row.company_name != null && (
            <span className="t-caption">
              <button
                type="button"
                className="cell-link"
                onClick={() =>
                  navigate({ screen: "companies", id: row.company_id ?? "" })
                }
              >
                {row.company_name}
              </button>
            </span>
          )}
        </CellStack>
      ),
    },
    {
      key: "admission",
      header: t("blockedDomains.col.admission"),
      render: (row) => {
        const reason = reasonWords(row, t);
        return (
          <CellStack>
            <Badge tone={admissionTone(row.admission)}>
              {t(ADMISSION_LABEL[row.admission])}
            </Badge>
            {reason && <span className="t-caption">{reason}</span>}
          </CellStack>
        );
      },
    },
    {
      key: "source",
      header: t("blockedDomains.col.source"),
      render: (row) => {
        const decider = DECIDER_LABEL[row.source];
        return (
          <span className="blocked-domains-decider">
            {decider ? t(decider) : "—"}
          </span>
        );
      },
    },
    {
      key: "decided",
      header: t("blockedDomains.col.decided"),
      align: "end",
      render: (row) => (
        <time dateTime={row.decided_at}>
          {formatDate(row.decided_at, locale, zone)}
        </time>
      ),
    },
    {
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (row) => <DecisionMenu row={row} verbs={verbs} />,
    },
  ];
}

function reasonWords(row: BlockedDomain, t: ReturnType<typeof useT>): string {
  switch (row.reason_code) {
    case "stated":
      return row.note ?? "";
    case "unknown":
      return row.reason;
    case "bulk_sender":
      return t(
        row.sender_kind
          ? BULK_LABEL[row.sender_kind]
          : "blockedDomains.reason.bulkSender",
      );
    default: {
      // An older server sends no code: its English sentence beats a blank.
      const label = REASON_LABEL[row.reason_code];
      return label ? t(label) : row.reason;
    }
  }
}

const OPPOSITE: Record<
  BlockedDomain["admission"],
  { admission: Decision["admission"]; verb: MessageKey } | null
> = {
  admitted: { admission: "suppressed", verb: "blockedDomains.rowRefuse" },
  suppressed: { admission: "admitted", verb: "blockedDomains.rowAdmit" },
  undecided: null,
};

// An undecided domain offers only Ask again: nobody has grounds to decide it.
// A decided one offers its opposite, which owes a reason in the dialog.
function DecisionMenu({
  row,
  verbs: { revise, refusal, set, reopen },
}: Readonly<{ row: BlockedDomain; verbs: RowVerbs }>) {
  const t = useT();
  const opposite = OPPOSITE[row.admission];
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: row.domain })}>
        {opposite ? (
          <Button
            aria-haspopup="dialog"
            disabled={set.isPending}
            reasonId={refusal}
            onClick={() => revise(row, opposite.admission)}
          >
            {t(opposite.verb)}
          </Button>
        ) : (
          <Button
            disabled={reopen.isPending}
            reasonId={refusal}
            onClick={() => reopen.mutate(row.domain)}
          >
            {t("blockedDomains.rowReopen")}
          </Button>
        )}
      </OverflowMenu>
    </span>
  );
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
