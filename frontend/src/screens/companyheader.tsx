import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import {
  useCanWrite,
  useCanWriteRecord,
  useRecordWriteRefusal,
} from "../app/capability";
import { navigate } from "../app/router";
import { Badge, Button, OverflowMenu } from "../design-system/atoms";
import { InlineChoice } from "../design-system/inlinechoice";
import { useT } from "../i18n";
import { ArchiveAction } from "./archive";
import { useClaimRecord } from "./claimrecord";
import { throwProblem, useViewerId } from "./common";
import { LIFECYCLE_LABELS, LIFECYCLE_OPTIONS } from "./companies";
import { DecisionsChip } from "./companyapprovals";
import { patchCompanyField, searchCompanyTargets } from "./companyform";
import { RELATIONSHIP_TYPE_LABELS, relationshipBadges } from "./companylookups";
import { CompanyRejectAction } from "./companyreject";
import { rosterMissLabel, useRoster, useRosterPartial } from "./entityref";
import { MergeAction } from "./merge";
import { ShareAction } from "./share";

// The account header's editable pieces: lifecycle and owner, the two values a
// rep changes in place (InlineChoice) rather than through an edit modal, plus
// what the account IS (CompanyRelationshipBadges) and the record's own menu
// (CompanyActionBadges). The subtitle and facts strip live in
// companyheaderfacts.tsx and the header's other verbs in
// companyheaderactions.tsx, so this file holds only the pieces that write.
//
// Split out of companies.tsx because that file had grown past 2,700 lines
// carrying the list screen, the enrichment tools, the evidence cards and this
// at once, and the V2 work adds to every one of them.

type Company = components["schemas"]["Company"];
type Company360View = components["schemas"]["Company360"];
type Lifecycle = NonNullable<Company["lifecycle"]>;
type UpdateCompanyRequest = components["schemas"]["UpdateCompanyRequest"];

// patchCompanyField sends one field through the ordinary company PATCH,
// with the record's own version as If-Match. The inline controls share it so a
// lifecycle change and an owner change cannot end up with different conflict,
// refusal or invalidation behaviour.
//
// It throws on failure rather than swallowing: InlineChoice renders what is
// thrown beside the control, and the server's problem detail is a better
// sentence than any this layer could invent.
// useCompanyFieldPatch wires one inline header edit to the query cache: the
// record, the list it appears in and the 360 that summarizes it all read the
// value being changed, so all three are refetched rather than left showing the
// old one until something else happens to invalidate them.
//
// Exported so the rail's own Details grid (companyrail.tsx) wires its inline
// edits to the SAME PATCH shape and the SAME three-key invalidation rather
// than keeping a second copy: one inline company edit and another that
// silently invalidates a different set of caches is the drift this file
// already exists to prevent within its own component.
// Through useMutation rather than a bare async call, so the write is a
// MUTATION as far as the query client is concerned. The policy that refreshes a
// record's open history after any successful write hangs off the mutation
// cache, and an inline edit that bypassed it left the history on screen showing
// the state before the edit.
export function useCompanyFieldPatch(company: Company) {
  const queryClient = useQueryClient();
  const save = useMutation({
    // The record travels WITH the body, for the reason the invalidation below
    // exists: `company.version` is the If-Match this write pins and it moves on
    // every successful write. Read out of the closure, two edits from one
    // render would both send the version that predates the first, and the
    // second would fail a conflict check it should pass.
    mutationFn: ({ company: target, body }: CompanyFieldPress) =>
      patchCompanyField(target, body),
    onSuccess: async (_result, { company: target }) => {
      await queryClient.invalidateQueries({ queryKey: ["companies"] });
      await queryClient.invalidateQueries({
        queryKey: ["company360", target.id],
      });
      // The header renders from the SINGLE-record query, and its version is the
      // If-Match the next inline edit sends. Leaving it stale shows the old value
      // after a successful save and makes the following edit fail on a version
      // the server has already moved past.
      await queryClient.invalidateQueries({
        queryKey: ["company", target.id],
      });
    },
  });
  return (body: UpdateCompanyRequest) =>
    save.mutateAsync({ company, body }).then(() => undefined);
}

// What one inline account edit carries: the record it is written against and
// the field values, so neither is read out of the closure at click time.
type CompanyFieldPress = Readonly<{
  company: Company;
  body: UpdateCompanyRequest;
}>;

// companyReadOnlyReason says why this record cannot be edited, when there is
// something worth saying.
//
// Exported for the same reason as useCompanyFieldPatch above: the rail's
// Details grid gates its own edit affordances on `writable`, and the reason an
// archived account is read-only is a fact about the RECORD, not about which
// component happens to be drawing it.
export function useCompanyReadOnlyReason(company: Company): string | undefined {
  const t = useT();
  // The per-ROW question only. The object grant and the seat ceiling are the
  // caller's to apply — every mount point here already ANDs `useCan` with this
  // reason, and folding them in again would answer "no grant" as though it were
  // a fact about the record.
  const mine = company.writable ?? false;
  // Archived first: it is the reason a reader can act on, by restoring the
  // record. Ownership comes last because it is the standing state — a company
  // that is simply somebody else's is not a problem to solve, it is who owns it.
  if (company.archived_at) {
    return t("record.archivedReadOnly");
  }
  // An UNOWNED record is not "somebody else's" — it is nobody's yet, and the
  // claim door is deliberately open to every seat. Reporting it read-only here
  // would shut the one control that makes it writable, which is the opposite of
  // what this reason is for.
  if (!mine && company.owner_id) {
    return t("record.notYoursToChange");
  }
  return undefined;
}

// Exported for its two mount points: the header passes it into RecordView's
// `nameBadge` slot, where the record's standing belongs on the name's own
// line, and the rail's Details grid mounts the SAME control rather than a
// second InlineChoice with its own PATCH. One implementation of how lifecycle
// is written, two places it is drawn, so the two cannot disagree about what
// they last wrote. `hideLabel` is unconditional: both callers name the field
// themselves, the badge beside the name and the grid's own label column.
export function CompanyLifecycleControl({
  company,
}: Readonly<{ company: Company }>) {
  const t = useT();
  // useCanWriteRecord, not useCanWrite: the grant and the seat say this ROLE
  // may change accounts, and the row says whether this one is theirs to change.
  // Gating on the grant alone offers an active control whose save is rejected.
  const canUpdate = useCanWriteRecord("company", company);
  const readOnlyReason = useCompanyReadOnlyReason(company);
  const patch = useCompanyFieldPatch(company);
  return (
    <InlineChoice
      label={t("company.lifecycle")}
      // The badge already reads as the account's standing beside its name —
      // a "Lifecycle: " prefix in front of it would be the one value on the
      // line saying its own name twice. `label` still drives the accessible
      // name (aria-label, sr-only form label), so a screen reader hears
      // "Lifecycle" regardless.
      hideLabel
      value={company.lifecycle ?? "unknown"}
      options={LIFECYCLE_OPTIONS.map((value) => ({
        value,
        label: t(LIFECYCLE_LABELS[value]),
      }))}
      canEdit={canUpdate && !readOnlyReason}
      readOnlyReason={readOnlyReason}
      // The account's standing is the one value beside its name a reader
      // looks for first. Tinted rather than filled: it marks the one value
      // here a reader can set, without reading as the page's primary action.
      render={(value) => (
        <Badge tone="accent">{t(LIFECYCLE_LABELS[value as Lifecycle])}</Badge>
      )}
      onSave={(next) =>
        patch({
          lifecycle: next as NonNullable<UpdateCompanyRequest["lifecycle"]>,
        })
      }
    />
  );
}

// What to call an owner the roster's answer does not name. "No longer in the
// user list" is a claim about a read that came back WITHOUT them, so it is the
// only reading this screen supplies; the three that are not about an owner at
// all — still reading, read failed, walk stopped short — belong to the roster
// and are spelled once there. Shared by every control here that names the
// current owner, so one of them cannot go on making the claim after the others
// stopped.
function unresolvedOwnerLabel(
  roster: Readonly<{ isPending: boolean; isError: boolean }>,
  partial: boolean,
  t: ReturnType<typeof useT>,
): string {
  return rosterMissLabel(roster, partial, t, t("ref.notInRoster"));
}

// Exported for the same reason as useCompanyFieldPatch/useCompanyReadOnlyReason
// above: the rail's Details grid edits the SAME field through the SAME
// roster read, the SAME not-in-roster fallback and the SAME
// unowned-only-while-unowned rule, rather than a second picker that could
// silently diverge from any of the three. `hideLabel` lets the rail's own
// FieldRow label column say "Owner" once instead of this control saying it
// again — the header call site omits it and keeps its current prose.
export function CompanyOwnerControl({
  company,
  hideLabel,
}: Readonly<{ company: Company; hideLabel?: boolean }>) {
  const t = useT();
  // Two different questions, deliberately.
  //
  // Changing the owner of an OWNED account is a write to that row, so it takes
  // the row's own answer. Claiming an UNOWNED one cannot: `writable` is false
  // precisely because nobody owns it yet, and gating the claim on it would
  // close the only door out of that state. The claim is its own authority and
  // the server holds it — this is the grant plus the seat, which is what the
  // claim endpoint itself requires.
  const canClaim = useCanWrite("company", "update");
  const canUpdate =
    useCanWriteRecord("company", company) || (!company.owner_id && canClaim);
  const readOnlyReason = useCompanyReadOnlyReason(company);
  const patch = useCompanyFieldPatch(company);
  const claim = useClaimRecord("company", company.id, company.version);
  const viewerId = useViewerId();
  const roster = useRoster("user", true);
  const rosterPartial = useRosterPartial("user", true);
  const owners = (roster.data ?? []).flatMap((entry) =>
    "display_name" in entry
      ? [{ value: entry.id, label: entry.display_name }]
      : [],
  );
  // The account's current owner may sit outside what the roster read — a
  // deactivated user, or a workspace deeper than the walk reaches — and a select
  // whose current value is not an option renders blank. Naming them keeps the
  // control honest about who owns it today even when it cannot resolve them;
  // which sentence is honest is `unresolvedOwnerLabel`'s question, not this
  // one's.
  if (
    company.owner_id &&
    !owners.some((user) => user.value === company.owner_id)
  ) {
    owners.unshift({
      value: company.owner_id,
      label: unresolvedOwnerLabel(roster, rosterPartial, t),
    });
  }
  // "Unowned" is offered only while the account IS unowned. `owner_id` cannot
  // carry "unassign" on the wire — a null is indistinguishable from an omitted
  // field — so offering it on an owned account would take the answer and drop
  // it. Present as the truthful current state, absent as an edit we cannot make.
  const options = company.owner_id
    ? owners
    : [{ value: "", label: t("co.pulse.unowned") }, ...owners];
  return (
    <InlineChoice
      label={t("co.pulse.owner")}
      // The caller names the field: the header's meta line prints "Owner"
      // immediately before this control, and the grid has its own label
      // column. `label` still drives the accessible name either way.
      hideLabel={hideLabel}
      value={company.owner_id ?? ""}
      options={options}
      canEdit={canUpdate && !readOnlyReason}
      readOnlyReason={readOnlyReason}
      // The closed control reads off the SAME labels the open one offers, so
      // the header cannot name the owner one way and the editor another. That
      // is also what keeps the uuid out: reading the owner through the generic
      // record reference painted the raw id for the first moments of every page
      // load, and a uuid is not a weaker name — it is a non-answer spelled so
      // that no reader can use it.
      render={(value) => {
        if (!value) {
          return t("co.pulse.unowned");
        }
        return (
          owners.find((user) => user.value === value)?.label ??
          unresolvedOwnerLabel(roster, rosterPartial, t)
        );
      }}
      // An unowned account is nobody's to change until somebody claims it, so
      // a reader taking it on goes through the claim — the door the write arm
      // leaves open to every seat — while naming a colleague stays a patch,
      // which an unbounded seat may make and a bounded one may not.
      onSave={(next) =>
        !company.owner_id && next === viewerId
          ? claim()
          : patch({ owner_id: next })
      }
    />
  );
}

// useCompanyVerbRefusal answers why the record's own verbs — edit, merge,
// archive, share — are refused, or undefined when they are pressable.
//
// It is the shared record answer: the record is archived, or this caller may
// not write it — no grant, a read seat, or a row that is somebody else's. Each
// is a fact about the RECORD as this reader holds it, so each takes STATE-4a's
// answer: the verb stays visible and says why, because a missing button reads
// as a build without the feature.
//
// An UNOWNED record is refused like any other the server marks unwritable:
// the write gate treats an ownerless row as nobody's to change, so Edit on it
// could only fail. The way IN stays open regardless — the owner control keeps
// its claim door on its own predicate (CompanyOwnerControl), and that is the
// verb an unowned account offers.
export function useCompanyVerbRefusal(company: Company): string | undefined {
  const t = useT();
  return useRecordWriteRefusal("company", company, {
    archived: t("record.archivedReadOnly"),
    notYours: t("record.notYoursToChange"),
  });
}

// Which relationship types the LIFECYCLE already speaks for.
//
// The two fields answer different questions — what a company IS to us, and
// where it STANDS with us — but they overlap on one word. An account whose
// lifecycle is `former_customer` still carries the `customer` relationship
// type, because that is what it was; printing both put "Former customer" and
// "Customer" side by side on one header, which is not two facts but one fact
// and its own contradiction.
//
// A map rather than a string comparison: `customer` and `former_customer`
// render different words, so matching on the rendered label caught the
// duplicate and missed the contradiction — which is the worse of the two,
// because a reader can see a repeat for what it is.
/**
 * What the company IS to us, beside its name.
 *
 * A tag ON the record, so it belongs with the record's name rather than among
 * the verbs — set among the buttons it read as a control that does nothing,
 * and it was the one thing in that row a reader could not press.
 *
 * A type the lifecycle beside it already speaks for is dropped by
 * `relationshipBadges`, so the header states a relationship once and in its
 * current tense.
 */
export function CompanyRelationshipBadges({
  company,
}: Readonly<{ company: Company }>) {
  const t = useT();
  return (
    <>
      {relationshipBadges(company, t).map((relType) => (
        <Badge key={relType} tone="accent">
          {t(RELATIONSHIP_TYPE_LABELS[relType])}
        </Badge>
      ))}
    </>
  );
}

export function CompanyActionBadges({
  company,
  view,
  onOpenHistory,
  onSetUpPartner,
  onOpenDecisions,
  archivedReasonId,
}: Readonly<{
  company: Company;
  view?: Company360View;
  onOpenHistory: () => void;
  onSetUpPartner: () => void;
  onOpenDecisions?: () => void;
  // Stated by the caller for the whole strip; see companyheaderactions.tsx's
  // CompanyHeaderActions, which reads the same reason for the row beside
  // this menu.
  archivedReasonId?: string;
}>) {
  const t = useT();
  // An archived record is read-only: the backend rejects edit/merge/archive
  // on a non-live row (there is no unarchive path). The verbs stay VISIBLE
  // and refused rather than disappearing (STATE-4a) — a control blocked by
  // the record's STATE says why, because the reason is the information and a
  // missing button reads as a build without the feature. Its history stays
  // readable — what happened to a record is exactly what a reader wants after
  // it has been put away.
  //
  // Undefined on a live account, which is what leaves those verbs pressable.
  // See companyheaderactions.tsx's CompanyHeaderActions: the id is an
  // override, never what decides whether these verbs are refused.
  const ownReasonId = useId();
  const menuReasonId = archivedReasonId ?? ownReasonId;
  const refusedReason = useCompanyVerbRefusal(company);
  const refusedByState = refusedReason ? menuReasonId : undefined;
  return (
    <>
      {/* What the company IS to us is drawn beside its NAME, by
          CompanyRelationshipBadges — a tag on the record belongs with the
          record. Drawn here as well it was the same badge in two places on one
          screen, and a reader who found both had to satisfy themselves the two
          agreed. */}
      {company.archived_at && <Badge tone="warn">{t("record.archived")}</Badge>}
      {/* The trigger is unconditional because the menu always holds something
          to say: an archived account's verbs are refused rather than dropped,
          and the sentence refusing them travels with them. Only a panel with
          no items at all would be worth hiding. */}
      <OverflowMenu label={t("record.moreActions")}>
        {refusedReason && !archivedReasonId && (
          <p id={ownReasonId}>{refusedReason}</p>
        )}

        <MergeAction
          disabledReasonId={refusedByState}
          label={t("merge.company")}
          sourceId={company.id}
          sourceName={company.display_name}
          searchTargets={searchCompanyTargets}
          merge={async (targetId) => {
            const { data, error } = await api.POST("/companies/{id}/merge", {
              params: {
                path: { id: company.id },
                ...ifMatch(requireVersion(company.version)),
              },
              body: { target_id: targetId },
            });
            if (error) {
              throwProblem(error, t);
            }
            return data;
          }}
          invalidate="companies"
          recordKey="company"
          survivorRoute={(targetId) => ({
            screen: "companies",
            id: targetId,
          })}
        />
        <ShareAction
          recordType="company"
          recordId={company.id}
          disabledReasonId={refusedByState}
        />
        {/* The audit spine: who changed this record and when. It reads as an
            inspection of the record rather than part of its story, so it sits
            with the other rare verbs instead of beside the account's own
            timeline. */}
        <Button data-testid="company-full-history" onClick={onOpenHistory}>
          {t("record.fullHistory")}
        </Button>
        {/* The way in to the partner programme for an account that has none.
            The tab only shows once there IS one, so without this the first
            partner row would be unreachable — this is the same form, asked
            for rather than offered. Below Full history rather than beside
            Merge: every row above is a verb EVERY record carries, in the order
            they all carry them, and every row below is this account's own. */}
        {!(company.relationship_types ?? []).includes("partner") && (
          <Button reasonId={refusedByState} onClick={onSetUpPartner}>
            {t("company.partnerSetUp")}
          </Button>
        )}
        {/* The account's own waiting decisions. It reads as a count in the
              header, which is a state, and this is the verb that answers it —
              so it sits with the other rare verbs rather than as a chip beside
              the account's name. Absent when nothing waits. */}
        {onOpenDecisions && (
          <DecisionsChip view={view} onOpen={onOpenDecisions} />
        )}
        {/* Beside Archive because it IS an archive, and separate from it
            because archiving alone does not settle the question: this record
            came from mail, so the same domain mints it again next week. Drawn
            only where there is a domain to refuse and only for a seat holding
            both halves — CompanyRejectAction decides both, and returns nothing
            when either says no. */}
        <CompanyRejectAction
          company={company}
          disabledReasonId={refusedByState}
        />
        {/* Last, and set apart by the panel's own seam (atoms.css). This is
            the one verb here a reader cannot walk back from the header, so it
            does not sit in the run of routine ones where a slipped pointer
            reaches it. */}
        <ArchiveAction
          disabledReasonId={refusedByState}
          label={t("record.archive")}
          confirmText={t("record.archiveConfirm")}
          archivedMessage={t("record.archiveDone", {
            name: company.display_name,
          })}
          archive={async () => {
            const { data, error } = await api.DELETE("/companies/{id}", {
              params: {
                path: { id: company.id },
                ...ifMatch(requireVersion(company.version)),
              },
            });
            if (error) {
              throwProblem(error);
            }
            return data;
          }}
          invalidate="companies"
          recordKey="company"
          onArchived={() => navigate({ screen: "companies" })}
        />
      </OverflowMenu>
    </>
  );
}

// The scheme is noise in a chip: every one of these is https, and "https://"
// costs eight characters of a row that has little space to fit it in. A URL
// we cannot parse is shown whole rather than silently dropped.
export function displayHost(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

// `website_url` is derived server-side from the primary domain row, and a
// company can carry the domain without it. Falling back to the row keeps the
// domain on those records rather than silently dropping the one identifying
// fact the reader had before. Shared by every reader of the company's web
// presence, so the fallback lives in one place rather than being re-derived
// per caller.
export function companyWebsite(company: Company): string | undefined {
  const primaryDomain = (company.domains ?? []).find(
    (d) => d.is_primary,
  )?.domain;
  return (
    company.website_url ??
    (primaryDomain ? `https://${primaryDomain}` : undefined)
  );
}

// useAccountChronology assembles the middle column's history: what happened
// with this account, what changed about the record, or both in one order.
//
// The two feeds page independently, so "both" is not a concatenation — the
// merge is cut where it stops being provably complete (mergeChronology), and
// the cut is stated rather than left to look like the end of the history.
