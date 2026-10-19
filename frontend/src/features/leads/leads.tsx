import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { PageAsideToggle, usePageAside } from "../../components/pageaside";
import { QueryGate } from "../../design-system/querystates";
import { RecordTabs } from "../../design-system/recordtabs";
import { useRecordTimeline } from "../../design-system/recordtimeline";
import { RecordView } from "../../design-system/recordview";
import { useToast } from "../../design-system/toast";
import { leadIdentityName } from "../../format/leadname";
import { viewerZone } from "../../format/timezone";
import { useT } from "../../i18n";
import { useOpenEmail } from "../../lib/openemail";
import { unwrap } from "../../lib/problem";
import { scrollPageToTop } from "../../lib/reveal";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { leadBand } from "./leadband";
import { LeadFacts, LeadPulse, LeadSubtitle } from "./leadheader";
import { LeadHistoryTab } from "./leadhistory";
import { LeadDealsProjectsTab, LeadRail } from "./leadraillinks";
import { LeadStepper } from "./leads.stepper";
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
import { LeadIdentityFields } from "./leads.identity";
import { LeadOverviewPane } from "./leads.overview";
import { useLeadPatch } from "./leads.patch";
import { usePromotionRecord } from "./leads.promotion";

export type Lead = components["schemas"]["Lead"];

export type UpdateLeadRequest = components["schemas"]["UpdateLeadRequest"];

import { leadKey, leadWriteKeys } from "../../lib/leadkeys";
import { useAddressedTab } from "../../screens/recordtab";
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
  const details = usePageAside();
  // ONE sentence about this lead being closed, minted here and pointed at by
  // every control the closure refuses (ADR-0108 §6).
  const terminalReasonId = useId();
  const [tab, setTab] = useAddressedTab("leads", id, LEAD_TABS);
  // The thread under the call reads the WHOLE history, not whatever the
  // History tab's own filter has narrowed. A filter is a view of that tab; a
  // call that said "no reply since" because the reader had hidden emails
  // would be false.
  const threadQuery = useRecordTimeline("lead", id);
  const [openEmail, setOpenEmail] = useOpenEmail();
  const [dialog, setDialog] = useState<"qualify" | "disqualify" | null>(null);
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

  return (
    <div className="record-sheet">
      <RecordView
        back={<WorklistReturnLink />}
        // One rung under the record scale: the name is still the largest thing
        // on the page, but beside a work column that opens on the agent's ask
        // it no longer needs to be the size of a masthead, the same rung the
        // contact page reads at (contactpage.tsx).
        scale="compact"
        // What a rep CONSULTS — who owns this and why it scores what it scores
        // — in the details pane beside the work, so the work column stays the
        // work and the context does not move when the tab does. The same pane,
        // fold and memory of it as every other record page.
        // At every width, and told whether it is showing, so the column
        // folds rather than vanishing when the toggle shuts it.
        aside={
          <LeadRail
            lead={lead}
            writer={writer}
            onQualify={() => setDialog("qualify")}
            reasonId={writer.readOnly ? terminalReasonId : undefined}
            details={<LeadIdentityFields lead={lead} writer={writer} />}
          />
        }
        asideOpen={details.open}
        name={leadIdentityName(lead) || t("lead.unnamed")}
        identity={lead.id}
        // The role and the company, on the name's own line: the contact
        // page's register for the same two facts (ContactSubtitle). A lead
        // carries no company FK, so unlike the contact's this is never a link.
        nameBadge={<LeadSubtitle lead={lead} />}
        // The pills under the name: that this is a lead, and where it stands
        // on the ladder. A reader has to know this is a prospect and not a
        // contact BEFORE they read anything else about them (ADR-0108 §1).
        pulse={<LeadPulse lead={lead} />}
        // The facts strip: how to reach this lead, where it came from, and who
        // holds it. Owner keeps its Assign control live in the cell rather than
        // reading only: it used to live in the pulse row for the same reason
        // it lives here now: ownership was reachable only from the details
        // pane, which REMEMBERS being folded, so the one control that takes a
        // lead out of the unassigned queue was, for anyone who had ever
        // collapsed the pane, behind a toggle they had to remember. Rendered
        // here and nowhere else: a second copy in the pane made every owner
        // query on the page ambiguous, which is two controls disagreeing
        // waiting to happen.
        badges={
          <LeadFacts
            lead={lead}
            id={id}
            writer={writer}
            terminalReasonId={terminalReasonId}
          />
        }
        actions={
          <LeadActions
            lead={lead}
            id={id}
            terminalReasonId={terminalReasonId}
            refusedReasonId={writer.readOnly ? terminalReasonId : undefined}
            onQualify={() => setDialog("qualify")}
            onDisqualify={() => setDialog("disqualify")}
            onLogged={refreshAfterTouch}
            emailOpen={composing}
            onEmailOpenChange={setComposing}
          />
        }
        actionsInline
        // The shell stamps timeline rows in this zone. The viewer's own is the
        // honest default for a prospect: a lead carries no workspace location of
        // its own to prefer over where the reader is.
        zone={viewerZone()}
        // Where the lead stands, at the foot of its head — the same place a
        // deal's own ladder stands, for the question both records are asked
        // first.
        standing={
          <LeadStepper
            lead={lead}
            pending={writer.patch.isPending}
            readOnlyReason={writer.readOnlyReason}
            onStep={(status) => {
              // Same one-write-at-a-time rule as the inline rows: a status
              // sent while another save is in flight races it for If-Match.
              if (!writer.patch.isPending && !writer.readOnly) {
                writer.save({ status });
              }
            }}
            onQualify={() => setDialog("qualify")}
            onDisqualify={() => setDialog("disqualify")}
          />
        }
        band={leadBand({ lead, writer, reasonId: terminalReasonId, id, t })}
        // The same strip every record in the product carries: a place a reader
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
              // The same key the account's own Deals & projects tab carries:
              // both hold one deal and one project, read the same way.
              deals: t("tab.dealsProjects"),
              history: t("tab.history"),
            }}
            // The switch for the details pane, at the end of the tab row: it
            // chooses what the page shows beside the work, so it stands with
            // the controls that choose what the work column shows. Drawn as a
            // link in the row rather than a boxed control, the same quiet
            // reading the contact page's tab strip gives it.
            trailing={
              <PageAsideToggle
                quiet
                labels={{
                  show: t("record.panel.showDetails"),
                  hide: t("record.panel.hideDetails"),
                }}
              />
            }
          />
        }
      >
        {tab === "overview" && (
          <LeadOverviewPane
            lead={lead}
            id={id}
            writer={writer}
            promotion={promotion}
            terminalReasonId={terminalReasonId}
            thread={threadQuery}
            onOpenEmail={setOpenEmail}
          />
        )}
        {tab === "deals" && (
          <LeadDealsProjectsTab
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
    </div>
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
