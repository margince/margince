import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  LatestInboundLine,
  useLatestInbound,
} from "../../components/latestinbound";
import { QueryGate } from "../../design-system/querystates";
import { RecordTabs } from "../../design-system/recordtabs";
import { useRecordTimeline } from "../../design-system/recordtimeline";
import { RecordView } from "../../design-system/recordview";
import { useToast } from "../../design-system/toast";
import { leadIdentityName } from "../../format/leadname";
import { viewerZone } from "../../format/timezone";
import { useT } from "../../i18n";
import { ACTION_PARAM, QUALIFY_ACTION } from "../../lib/leads.address";
import { useOpenEmail } from "../../lib/openemail";
import { unwrap } from "../../lib/problem";
import { scrollPageToTop } from "../../lib/reveal";
import { useUrlParams } from "../../lib/urlstate";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { leadBand } from "./leadband";
import { LeadDetails, useFieldSave } from "./leaddetails";
import { LeadPulse, LeadSubtitle } from "./leadheader";
import { LeadHistoryTab, LeadLatestInbound } from "./leadhistory";
import { LeadDealsTab } from "./leadraillinks";
import "./leads.css";

import { LeadActions, LeadDialogs } from "./leads.actions";
// Leads (B-EP09.10a/b): visually SEGREGATED from the contact graph — the
// lead surface is accent-tinted, lead detail is its own screen (never
// contact.html — gap §3.5), and promote is eligibility-gated. Lead score is
// lead-local; the ≥60 / 40–59 / <40 colour thresholds are pinned by test.
// Search/filter/sort/pagination (P-14), the rich create modal (P-15), the
// If-Match edit form (P-1), and the dedupe view-existing link (P-16) are
// wired in here the same way as contacts (contacts.tsx) — the Promote button
// and score/status/company badges on the lead 360 stay exactly as they
// were. Status-change and score-override are Phase 4, not surfaced here.
import { LeadOverviewPane } from "./leads.overview";
import { useLeadPatch } from "./leads.patch";
import { usePromotionRecord } from "./leads.promotion";

export type Lead = components["schemas"]["Lead"];

export type UpdateLeadRequest = components["schemas"]["UpdateLeadRequest"];

import { leadKey, leadWriteKeys } from "../../lib/leadkeys";
import { useAddressedTab } from "../records/recordtab";
import { WorklistReturnLink } from "../worklist/worklist.return";

/**
 * The shared write, as the panels that call it see it: the mutation's STATE
 * (pending, refused, what it carried) and the two ways to start one. Handed
 * over as one object rather than as a mutation plus a loose function, so no
 * caller can reach past `save` to `mutate` and skip the version it stamps.
 */
export type LeadWriter = ReturnType<typeof useLeadPatch>;

const LEAD_TABS = ["overview", "deals", "history"] as const;

// The lead-360's "overview" pane, split out of LeadScreen so the tab switch
// doesn't push the render-prop closure over the cognitive-complexity budget.
// Every prop here is a value already resolved (or owned as local state) by
// LeadScreen — no new fetches, no behavior change from the pre-tab layout;
// the promote modal's open/trigger/note state stays lifted in the parent so
// it survives a tab switch away and back.
// The status ladder climbs a few seconds AFTER a logged touch: the workflow
// runs off the outbox, in the worker, not inside the POST. Without a delayed
// re-read the stepper keeps the old status until a manual reload — which
// reads as "logging did nothing". Three staggered re-reads cover the
// relay+workflow latency without polling forever; unmounting cancels them.
function useLadderRefresh(id: string): () => void {
  const queryClient = useQueryClient();
  const timers = useRef<number[]>([]);
  useEffect(
    () => () => {
      for (const timer of timers.current) {
        window.clearTimeout(timer);
      }
    },
    [],
  );
  return () => {
    for (const delay of [1500, 4000, 8000]) {
      timers.current.push(
        window.setTimeout(() => {
          for (const key of leadWriteKeys(id)) {
            queryClient.invalidateQueries({ queryKey: key });
          }
        }, delay),
      );
    }
  };
}

/**
 * One lead's page, with the record in hand.
 *
 * Split from `LeadScreen` because the page's ONE write is a hook and the lead
 * it writes only exists inside the query gate: a hook cannot be called from a
 * render prop, and moving the mutation above the gate would have it guarding a
 * record it had not read yet.
 */
function LeadRecord({ lead, id }: Readonly<{ lead: Lead; id: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const refreshAfterTouch = useLadderRefresh(id);
  // The one instant every reading on the page is made at. A lead has no
  // server `as_of`, and the page remounts per lead (`key`), so it is fresh.
  const [asOf] = useState(() => new Date().toISOString());
  // ONE sentence about this lead being closed, minted here and pointed at by
  // every control the closure refuses (ADR-0108 §6).
  const terminalReasonId = useId();
  const [tab, setTab] = useAddressedTab("leads", id, LEAD_TABS);
  // The overview reads the whole history, not whatever the History tab's own
  // filter has narrowed. A gap that said "your move" because the reader had
  // hidden emails would be false.
  const threadQuery = useRecordTimeline("lead", id);
  const [openEmail, setOpenEmail] = useOpenEmail();
  // A link from a surface that met this lead while creating a contact opens
  // the Qualify dialog, so the reader converts in one step.
  const [params] = useUrlParams();
  const [dialog, setDialog] = useState<"qualify" | "disqualify" | null>(() =>
    params.get(ACTION_PARAM) === QUALIFY_ACTION && !lead.archived_at
      ? "qualify"
      : null,
  );
  // What the last qualify did, said once: the contact and, when one was
  // opened, the deal. The page stays (ADR-0119) and the outcome panel below
  // carries the links on, so this is the confirmation rather than the record —
  // which is what `useToast` is, withdrawing itself instead of sitting in the
  // column as a second, permanent copy of the same news.
  const toast = useToast();
  // A promoted lead keeps its page (ADR-0119/A170). It no longer redirects to
  // the contact: the redirect said the lead had ceased to exist, which is
  // untrue of a record this product keeps, audits and can reverse — and it
  // left the reversal with nowhere to start from. The page reads the
  // promotion off its own audit row and says what happened.
  const promotion = usePromotionRecord(id, Boolean(lead.promoted_contact_id));
  const writer = useLeadPatch(lead, id, () => {
    for (const key of leadWriteKeys(id)) {
      queryClient.invalidateQueries({ queryKey: key });
    }
  });
  // The header's composer, lifted here so the overview's "Answer" row opens
  // the SAME one rather than a copy of it (RecordEmailVerb's own controlled
  // mode).
  const [composing, setComposing] = useState(false);
  const latest = useLatestInbound("lead", lead.id);
  const saveField = useFieldSave(lead);

  return (
    <RecordView
      back={<WorklistReturnLink />}
      aside={
        <LeadDetails
          lead={lead}
          writer={writer}
          terminalReasonId={terminalReasonId}
        />
      }
      name={leadIdentityName(lead) || t("lead.unnamed")}
      rename={{
        label: t("create.fullName"),
        value: lead.full_name ?? "",
        canEdit: !writer.readOnly,
        readOnlyReason: writer.readOnlyReason,
        onSave: saveField("full_name", true),
      }}
      identity={lead.id}
      nameBadge={<LeadSubtitle lead={lead} />}
      // A reader has to know this is a prospect and not a contact before they
      // read anything else about them.
      pulse={<LeadPulse lead={lead} />}
      latest={
        tab !== "overview" && (
          <LatestInboundLine
            latest={latest}
            onOpen={() => setTab("overview")}
          />
        )
      }
      actions={
        <LeadActions
          lead={lead}
          id={id}
          writer={writer}
          terminalReasonId={terminalReasonId}
          refusedReasonId={writer.readOnly ? terminalReasonId : undefined}
          onQualify={() => setDialog("qualify")}
          onDisqualify={() => setDialog("disqualify")}
          onLogged={refreshAfterTouch}
          emailOpen={composing}
          onEmailOpenChange={setComposing}
        />
      }
      // The shell stamps timeline rows in this zone. The viewer's own is the
      // default for a prospect. A lead carries no workspace location of its
      // own to prefer over where the reader is.
      zone={viewerZone()}
      band={leadBand({ lead, writer, reasonId: terminalReasonId, t })}
      // The same strip every record in the product carries. It is a place a reader
      // navigates, drawn as a rule with the open body underlined, rather than
      // a pill that offers a setting.
      tabs={
        <RecordTabs
          options={LEAD_TABS}
          value={tab}
          onChange={(next) => {
            setTab(next);
            scrollPageToTop();
          }}
          labels={{
            overview: t("tab.overview"),
            deals: t("tab.deals"),
            history: t("tab.history"),
          }}
          // Counted only once the whole history is in hand: a cut page's
          // length is not how many rows the lead has.
          counts={
            threadQuery.isSuccess && !threadQuery.hasNextPage
              ? { history: threadQuery.activities.length }
              : undefined
          }
        />
      }
    >
      {tab === "overview" && (
        <LeadLatestInbound latest={latest} leadId={lead.id} />
      )}
      {tab === "overview" && (
        <LeadOverviewPane
          lead={lead}
          id={id}
          writer={writer}
          promotion={promotion}
          terminalReasonId={terminalReasonId}
          thread={threadQuery}
          asOf={asOf}
          onOpenEmail={setOpenEmail}
          onQualify={() => setDialog("qualify")}
          onDisqualify={() => setDialog("disqualify")}
        />
      )}
      {tab === "deals" && (
        <LeadDealsTab
          lead={lead}
          writer={writer}
          onQualify={() => setDialog("qualify")}
          reasonId={writer.readOnly ? terminalReasonId : undefined}
        />
      )}
      <LeadDialogs
        lead={lead}
        dialog={dialog}
        onClose={() => setDialog(null)}
        onQualified={(done) => {
          setDialog(null);
          toast.show(done);
        }}
      />
      {tab === "history" && (
        <LeadHistoryTab lead={lead} onOpenEmail={setOpenEmail} />
      )}
      {/* One drawer over the lead, whichever tab is open: the Overview
          call's own thread and the History tab's rows both open into it. */}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
      />
    </RecordView>
  );
}

export function LeadScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const leadQuery = useQuery({
    queryKey: leadKey(id),
    queryFn: async () => {
      return unwrap(
        await api.GET("/leads/{id}", {
          params: { path: { id } },
        }),
      );
    },
  });

  return (
    <div className="wrap lead-surface">
      <QueryGate query={leadQuery} pendingLabel={t("nav.leads")}>
        {(lead) => (
          // Keyed by lead: every piece of page state below — the open dialog,
          // the tab, a half-typed score override — is about THIS lead, and
          // this screen stays mounted from one to the next.
          <LeadRecord key={lead.id} lead={lead} id={id} />
        )}
      </QueryGate>
    </div>
  );
}

export { promoteEligible, scoreTone } from "./leadpresentation";
export { LeadsScreen } from "./leads.list";
export { terminalBadge } from "./leadstanding";
