import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Sparkles } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate, routeHash } from "../app/router";
import {
  Avatar,
  Badge,
  Button,
  EmptyState,
  Field,
  Modal,
  Skeleton,
  StatCard,
} from "../design-system/atoms";
import { AvatarStack } from "../design-system/avatarstack";
import { type TimelineEntry, TimelineRow } from "../design-system/composed";
import { EvidenceMark } from "../design-system/evidencemark";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import {
  liveProjects,
  type PickableProject,
  ProjectPicker,
  useClearVanishedChoice,
  useSoleProjectDefault,
} from "../design-system/projectpicker";
import { Select } from "../design-system/select";
import {
  omitted,
  SurfaceState,
  sectionState,
} from "../design-system/surfacestate";
import {
  formatDate,
  formatMoney,
  formatMoneyCompact,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  problemCodeOf,
  problemMessageOf,
  throwProblem,
  useFinanceSummary,
  useViewerId,
} from "./common";
import type { CompanyTab } from "./companytab";
import "./company360.css";
import { activityTimeline } from "../design-system/activitytimeline";
import { FactList } from "../design-system/factlist";
import {
  HEALTH_DIMENSION_LABEL,
  HEALTH_RATING_LABEL,
  useAccountStanding,
} from "./companylookups";
import {
  routesTo,
  type StrengthBucket,
  useOrganizationGraph,
} from "./connections";
import {
  byReach,
  type CommitteeRole,
  countGaps,
  missingRolesByDeal,
  reachLabelKey,
  reachOf,
  roleLabelKey,
} from "./coverage";
import { CoverageExplorer } from "./coverageexplorer";
import { EntityRef } from "./entityref";
import {
  Citations,
  dealRoleLabel,
  incompleteGraph,
  RailPanel,
  SentenceList,
  WrittenBy,
} from "./record360";
import { TaskCompleteCheck, type useTaskUpdate } from "./taskactions";

// The company view's data layer and its right-rail cards.
//
// One read (GET /organizations/{id}/360) serves the whole page, and its
// `sections_omitted` is the thing that makes the page honest: a section the
// caller's role cannot read is ABSENT from the payload and named there, so
// every card below can say "hidden from you" instead of drawing an empty
// list that reads as "there is none".

type Organization360 = components["schemas"]["Organization360"];
type Contact = components["schemas"]["Organization360Contact"];
type Deal360 = components["schemas"]["Organization360Deal"];
type NextStep = components["schemas"]["Organization360NextStep"];
const OVERLAY_REFUSAL = "unsupported_in_overlay_mode";

export type Org360Result =
  | { state: "ready"; view: Organization360 }
  | { state: "overlay" };

/**
 * useOrganization360 reads the whole company page in one round trip.
 *
 * `enabled` exists for callers that are not the page: chrome mounted on every
 * screen has to hold the hook unconditionally and ask for nothing when there is
 * no record under it — an empty id is a 422, not an empty answer.
 */
export function useOrganization360(id: string, enabled = true) {
  return useQuery<Org360Result>({
    queryKey: ["organization360", id],
    enabled: enabled && id !== "",
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/organizations/{id}/360",
        { params: { path: { id } } },
      );
      if (error) {
        if (response.status === 422 && isOverlayRefusal(error)) {
          return { state: "overlay" };
        }
        throwProblem(error);
      }
      return { state: "ready", view: data };
    },
  });
}

// VIEW_ACK_DWELL_MS is how long the account must stay open before the visit
// counts. Opening a record and bouncing straight back out is not reading it,
// and an ack from that would mark unread activity as seen.
const VIEW_ACK_DWELL_MS = 5_000;

/**
 * useAcknowledgeOrganizationView advances THIS reader's "last seen" baseline
 * for the account — the thing that makes "N new since your last visit" mean
 * anything on the next visit. Without it the server keeps answering with no
 * baseline at all, so every visit reads as the first one.
 *
 * The 360 deliberately does not advance the baseline itself (a prefetch must
 * not be indistinguishable from a visit), so this is the only caller. Leaving
 * before the dwell elapses cancels the timer: the baseline moves only for a
 * visit that actually happened, and when in doubt it stays where it is —
 * showing an item twice is a smaller wrong than hiding one.
 *
 * Success does NOT invalidate the 360. The "new since your last visit" line
 * describes the visit in progress; refetching it out from under the reader
 * would erase the very thing they opened the page to see.
 */
export function useAcknowledgeOrganizationView(id: string, visited: boolean) {
  const ack = useMutation({
    mutationFn: async (organizationId: string) => {
      const { error } = await api.POST("/organizations/{id}/view-ack", {
        params: { path: { id: organizationId } },
      });
      if (error) {
        throwProblem(error);
      }
    },
  });
  // The mutation's own error state holds a failure; nothing renders it. A
  // baseline that did not move costs the reader one repeated line next time,
  // which is not worth an error banner over the account they came to read.
  const fire = ack.mutate;
  useEffect(() => {
    if (!visited) {
      return;
    }
    const timer = window.setTimeout(() => fire(id), VIEW_ACK_DWELL_MS);
    return () => window.clearTimeout(timer);
  }, [id, visited, fire]);
}

// isOverlayRefusal distinguishes "this workspace reads elsewhere" from every
// other 422 (a malformed id, say), which stays an error the caller sees.
//
// It narrows by checking rather than asserting: a problem body that is not
// the shape we expect — null, a string, an older server's payload — is not
// an overlay refusal, and must read as one failure rather than throwing a
// second one on the way to saying so.
function isOverlayRefusal(problem: unknown): boolean {
  const errors = asRecord(asRecord(problem)?.details)?.errors;
  if (!Array.isArray(errors)) {
    return false;
  }
  return errors.some((entry) => asRecord(entry)?.code === OVERLAY_REFUSAL);
}

// asRecord narrows an unknown to a readable object, or gives up. Truthiness
// first, because typeof null is "object" — the one case that would otherwise
// pass the guard and throw on the next property read.
function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : undefined;
}

// The roles one deal is short of, in the reader's words. One spelling, because
// the two callout forms below differ only in whether they name the deal.
function roleList(
  missing: readonly CommitteeRole[],
  t: ReturnType<typeof useT>,
): string {
  return missing.map((role) => t(roleLabelKey(role))).join(" / ");
}

// The coverage line the mockup draws above the contact rows: how many
// contacts, how many nobody has written to, how many roles are unfilled.
//
// ONLY counts this page can total. The mockup also carries "11 connected
// colleagues", which would mean summing each contact's routes — and those are
// capped at the top three by strength, so the sum would count the same
// colleague once per contact they know and report a bigger team than exists.
// A number nobody can check is worse than a shorter line.
//
// Silent when the contact list was truncated: "7 contacts" off a capped page
// is a count of the page, not of the account.
function CoverageSummary({
  contacts,
  untried,
  gaps,
  truncated,
  routesReadable,
}: Readonly<{
  contacts: readonly Contact[];
  untried: number;
  gaps: number;
  truncated: boolean;
  // Whether the routes this page read carry an answer at all. "Never written
  // to" is derived from who has exchanged messages with whom, so a reader
  // without that access must not be handed the aggregate — an activity-derived
  // count is still activity data.
  routesReadable: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (contacts.length === 0) {
    return null;
  }
  // A truncated page still knows how many contacts it is showing, and saying
  // "at least 25" beats saying nothing: the reader learns both that the
  // account is large and that this is not the whole of it.
  const parts = [
    truncated
      ? t("co.coverage.contactsAtLeast", {
          count: formatNumber(contacts.length, locale),
        })
      : plural("co.coverage.contacts", contacts.length, {
          count: formatNumber(contacts.length, locale),
        }),
  ];
  if (routesReadable && untried > 0) {
    parts.push(
      t("co.coverage.untried", { count: formatNumber(untried, locale) }),
    );
  }
  // The gap count is only meaningful over a complete picture: a capped page
  // hides the contacts who might hold the roles it would report as missing.
  if (!truncated && gaps > 0) {
    parts.push(
      plural("co.coverage.gaps", gaps, { count: formatNumber(gaps, locale) }),
    );
  }
  return <p className="surfacestate-empty">{parts.join(" · ")}</p>;
}

/**
 * PeopleCard lists the account's contacts with their relationship strength,
 * their role on the open deals, and whether they may be contacted.
 *
 * The two callouts are the ones a rep acts on: an account carried by a
 * single contact, and open deals with nobody named as champion.
 */
export function PeopleCard({
  view,
  // Whether this account takes writes at all. An archived record is read-only
  // — the page hides every other verb on one — so the role control goes too.
  writable = false,
  // The account whose connection graph a route-in asks about. Absent on the
  // loading skeleton, where there is no account to ask about yet.
  orgId,
  // The composite read's own pending flag, so an undefined `view` reads as
  // "still loading" rather than "could not be loaded" for as long as the
  // read is actually running.
  loading = false,
}: Readonly<{
  view?: Organization360;
  writable?: boolean;
  orgId?: string;
  loading?: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const contacts = [...(view?.people?.data ?? [])].sort(byReach);
  const truncated = Boolean(view?.people?.page.has_more);
  const dealsReadable =
    Boolean(view?.deals) && view != null && !omitted(view, "deals");
  const openDeals: OpenDeal[] = dealsReadable
    ? (view?.deals?.data ?? []).map((deal) => ({
        id: deal.deal_id,
        name: deal.name,
      }))
    : [];
  // Every way the committee picture can be partial, in one flag. An empty
  // `contacts` means "nobody" only when the section was actually READ: a
  // people section the grants withheld, or one this response never carried,
  // leaves the same empty array and would otherwise report both roles missing
  // from data the page never had. Deals past their first page hide the roles
  // held on them the same way.
  const committeeIncomplete =
    truncated ||
    !view?.people ||
    omitted(view, "people") ||
    Boolean(view?.deals?.page.has_more);
  const gaps = missingRolesByDeal(contacts, openDeals, committeeIncomplete);
  const untried = contacts.filter((c) => reachOf(c) === "untried");
  return (
    <RailPanel
      title={t("co.people.title")}
      state={sectionState(
        view,
        "people",
        Boolean(view?.people),
        contacts.length,
        loading,
      )}
      emptyLabel={t("co.people.empty")}
      footer={
        <>
          {contacts.length > 0 && (
            <AvatarStack
              people={contacts.map((contact) => ({
                name: contact.full_name,
              }))}
            />
          )}
          <CoverageSummary
            contacts={contacts}
            untried={untried.length}
            gaps={countGaps(gaps)}
            truncated={truncated}
            routesReadable={contacts.some((each) => each.routes)}
          />
          {/* The per-row coverage says who to call. The explorer answers the
              other question — where are we thin — for a handful of
              colleagues the reader picks, rather than a column per person
              on a forty-strong team. */}
          {orgId && contacts.length > 0 && (
            <CoverageExplorer orgId={orgId} contacts={contacts} />
          )}
        </>
      }
    >
      {/* The per-contact chips read as all-time claims — "Not approached"
          above a timeline showing last year's outbound email is the page
          arguing with itself. They are computed over the server's 90-day
          window (PO-F-3), so the window is stated once here rather than
          repeated on every row. */}
      {contacts.length > 0 && (
        <PanelBody>
          <p className="t-caption">{t("co.reach.window")}</p>
        </PanelBody>
      )}
      {contacts.map((contact) => (
        <PanelRow key={contact.person_id} className="co-person-row">
          <ContactRow
            contact={contact}
            openDeals={openDeals}
            writable={writable}
            orgId={orgId}
          />
        </PanelRow>
      ))}
      {contacts.length === 1 && !truncated && (
        <PanelBody>
          <p className="co-callout">
            <Badge tone="warn">{t("co.people.singleThread")}</Badge>
          </p>
        </PanelBody>
      )}
      {/* Who is missing, not only who is present. On an account where every
          known contact has gone quiet, the person nobody has written to is the
          only move left that is not a fourth follow-up. */}
      {(untried.length > 0 || gaps.length > 0) && (
        <PanelBody>
          {untried.length > 0 && (
            <p className="co-callout">
              <Badge tone="accent">
                {plural("co.people.untriedHint", untried.length, {
                  count: formatNumber(untried.length, locale),
                })}
              </Badge>
            </p>
          )}
          {/* One line per deal that is short, because a role is missing from
              the deal that lacks it. The deal is NAMED only when there is more
              than one open deal to be on: on a single-deal account "on Renewal"
              is the same word the reader already has, and the row's own roles
              line omits it for the same reason. */}
          {gaps.map((gap) => {
            // plural-rule:allow naming the deal or not is a choice between two
            // sentences, not between two forms of one, so no plural rule decides it
            const wording =
              openDeals.length === 1
                ? "co.people.missing"
                : "co.people.missingOnDeal";
            return (
              <p className="co-callout" key={gap.dealId}>
                <Badge tone="warn">
                  {t(wording, {
                    roles: roleList(gap.missing, t),
                    deal: gap.dealName,
                  })}
                </Badge>
              </p>
            );
          })}
        </PanelBody>
      )}
    </RailPanel>
  );
}

// OpenDeal is the slice of an open deal a role can be attached to.
type OpenDeal = { id: string; name: string };

// everyRoleHeld reports whether this contact already holds every assignable
// role on every open deal, which is when the verb has nothing left to write.
function everyRoleHeld(
  contact: Contact,
  openDeals: readonly OpenDeal[],
): boolean {
  return openDeals.every((deal) => {
    const held = new Set(
      contact.deal_roles
        .filter((entry) => entry.deal_id === deal.id)
        .map((entry) => entry.role),
    );
    return ASSIGNABLE_ROLES.every((role) => held.has(role));
  });
}

// The stakeholder roles offered here. `role` is free text on the wire until
// DEAL-EXT-5 mints the enum upstream, so this list is the UI's own vocabulary
// — the five the spec names, in the order a rep thinks of them.
const ASSIGNABLE_ROLES = [
  "champion",
  "economic_buyer",
  "influencer",
  "blocker",
  "user",
] as const;

/**
 * SetRoleAction records who this person is on a deal.
 *
 * The page told a reader "nobody here is your champion" and gave them nowhere
 * to say who is: the roles live on `relationship` rows written from the deal
 * screen, which is a different page and a different task. So the warning was
 * true, unactionable, and permanent.
 *
 * The role is recorded HUMAN-set, never inferred. Every CRM surveyed keeps
 * buyer roles human-tagged — AI may suggest one, but a champion nobody named
 * is a guess about a relationship, and the whole committee reading is built
 * on top of it.
 */
function SetRoleAction({
  contact,
  openDeals,
}: Readonly<{ contact: Contact; openDeals: readonly OpenDeal[] }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [dealId, setDealId] = useState("");
  const [role, setRole] = useState<string>(ASSIGNABLE_ROLES[0]);
  const titleId = useId();

  // A role this contact already holds on the selected deal is not on offer:
  // the write creates an edge, so picking it again asks the server for a
  // second copy of a fact that is already recorded.
  const held = new Set(
    contact.deal_roles
      .filter((entry) => entry.deal_id === dealId)
      .map((entry) => entry.role),
  );
  const offered: readonly string[] = ASSIGNABLE_ROLES.filter(
    (candidate) => !held.has(candidate),
  );
  // Changing the deal changes what is left to pick, so the selection follows
  // the list rather than the list following a stale selection.
  const picked = offered.includes(role) ? role : offered[0];

  const save = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/relationships", {
        body: {
          kind: "deal_stakeholder",
          person_id: contact.person_id,
          deal_id: dealId,
          role: picked,
          // Not sent: the flag is an EMPLOYMENT fact and this edge is a deal
          // stakeholder, so the literal said nothing and only stood in the way
          // of a gate that can now assert no screen states it by hand.
          source: "manual",
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: async () => {
      setOpen(false);
      // The committee reading, the missing-role warning and the row's own
      // chips all come off the 360, so the account is re-read rather than
      // patched in place.
      await queryClient.invalidateQueries({ queryKey: ["organization360"] });
    },
  });

  // A role belongs to a deal. With no open deal there is nothing to be a
  // champion OF, and the card already says so in its own words.
  //
  // Nothing left to offer is the same answer: a contact already holding every
  // role on every open deal would otherwise open a dialog with an empty list
  // and a dead Save button.
  if (openDeals.length === 0 || everyRoleHeld(contact, openDeals)) {
    return null;
  }
  return (
    <>
      <Button
        small
        onClick={() => {
          setDealId(openDeals[0].id);
          setOpen(true);
        }}
      >
        {t("co.role.set")}
      </Button>
      <Modal open={open} onClose={() => setOpen(false)} labelledBy={titleId}>
        <h2 id={titleId} className="t-h2 modal-title">
          {t("co.role.setOn", { name: contact.full_name })}
        </h2>
        {/* What the two words mean, once, where they are being chosen. The
            page used them as though everyone shares one definition. */}
        <p className="t-caption">{t("co.role.explain")}</p>
        <div className="form-stack">
          <Field label={t("co.role.onDeal")}>
            {(control) => (
              <Select
                {...control}
                value={dealId}
                onChange={setDealId}
                options={openDeals.map((deal) => ({
                  value: deal.id,
                  label: deal.name,
                }))}
              />
            )}
          </Field>
          <Field label={t("co.role.role")}>
            {(control) => (
              <Select
                {...control}
                value={picked ?? ""}
                onChange={setRole}
                options={offered.map((candidate) => ({
                  value: candidate,
                  label: dealRoleLabel(candidate, t),
                }))}
              />
            )}
          </Field>
          {save.isError && (
            <p className="t-caption form-error">
              {problemMessageOf(save.error, t)}
            </p>
          )}
          <div className="form-actions">
            <Button
              variant="primary"
              onClick={() => save.mutate()}
              disabled={save.isPending || dealId === "" || !picked}
            >
              {t("record.save")}
            </Button>
            <Button onClick={() => setOpen(false)}>{t("common.close")}</Button>
          </div>
        </div>
      </Modal>
    </>
  );
}

function ContactRow({
  contact,
  openDeals,
  writable,
  orgId,
}: Readonly<{
  contact: Contact;
  // The account this contact belongs to, for the route-in read.
  orgId?: string;
  // The open deals a role can be recorded against. A role belongs to a DEAL,
  // not to a person: this contact may be the champion on the renewal and
  // nobody on the new business.
  openDeals: readonly OpenDeal[];
  // Read-only accounts still NAME the roles held on them; they just offer no
  // way to change one.
  writable: boolean;
}>) {
  const t = useT();
  // Only roles on the deals this card is showing. `deal_roles` carries a
  // contact's role on CLOSED deals too, and a champion badge read off a deal
  // that was lost last year describes a pipeline that no longer exists.
  const openDealIds = new Set(openDeals.map((deal) => deal.id));
  const roles = contact.deal_roles.filter(
    (entry) => entry.role && openDealIds.has(entry.deal_id),
  );
  // Which deal a role is on only matters when there is more than one to
  // confuse: a person can be champion on the renewal and nobody on the new
  // business, and two identical badges would say neither.
  const nameOfDeal = (dealId: string) =>
    openDeals.length > 1
      ? openDeals.find((deal) => deal.id === dealId)?.name
      : undefined;
  const reach = reachOf(contact);
  // Title and role read as one quiet line under the name, not as a run of
  // badges — "Sales Manager · Champion on Pilot" is one fact about who this
  // person is, and a badge per clause said the same thing louder than it
  // needed to.
  // A purchased title fills the gap where nobody typed one (PO-EXT-9). The
  // server decides that — it sends provider_title only where the canonical
  // one is empty — so this reads the field rather than re-deciding the
  // precedence, which is how the two would come to disagree.
  const subline = [
    contact.title ?? contact.provider_title,
    ...roles.map((entry) => {
      const deal = nameOfDeal(entry.deal_id);
      return deal
        ? `${dealRoleLabel(entry.role, t)} · ${deal}`
        : dealRoleLabel(entry.role, t);
    }),
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <>
      <span className="co-person-avatar">
        <Avatar name={contact.full_name} />
      </span>
      <span className="co-person-body">
        {/* A real href, and it covers the whole row: a name is a small target
            beside the face and the badges that describe the same person, and
            a reader aiming at any of them means "open this contact". The
            verbs at the row's end sit above the cover, so they still act on
            the row rather than opening it. */}
        <a
          className="co-rowlink co-person-name co-rowcover"
          href={routeHash({ screen: "contacts", id: contact.person_id })}
        >
          {contact.full_name}
        </a>
        {subline && (
          <span className="co-person-sub">
            {/* ONE mark, on the whole line, and only where the title was
                bought. A mark per clause would turn a quiet subline into a
                run of badges, which is what this line was written to avoid. */}
            {contact.title_source === "provider" ? (
              <EvidenceMark
                value={subline}
                source={{
                  provenance: { kind: "connector", connector: "provider" },
                }}
              />
            ) : (
              subline
            )}
          </span>
        )}
      </span>
      <span className="co-person-end">
        {/* Where this person stands with us. "No reply" and "never asked"
            looked identical in this list and call for opposite next moves. */}
        <Badge tone={reach === "answered" ? "success" : undefined}>
          {t(reachLabelKey(reach))}
        </Badge>
        {/* Who here can actually reach them, inline rather than a click away.
            A forty-person team makes a contact x colleague matrix unreadable,
            so the row names the few worth naming and counts the rest. */}
        <ContactRoutes routes={contact.routes} />
        <ConsentChip consent={contact.consent} />
        {/* The page said "nobody here is your champion" and gave no way to
            say who is: the roles are set on the deal screen, which is a
            different page and a different task. */}
        {writable && <SetRoleAction contact={contact} openDeals={openDeals} />}
        {orgId && <RouteInAction orgId={orgId} contact={contact} />}
      </span>
    </>
  );
}

// The strongest few colleagues who have actually exchanged messages with this
// contact, and how many more there are.
//
// UNTRIED IS NOT COLD, and the distinction is the whole reason this renders a
// sentence rather than an empty space: "nobody has tried" tells a rep to pick up
// the phone, where a cold band tells them somebody already did and got nowhere.
// A page that shows the same thing for both gives the opposite instruction half
// the time.
function ContactRoutes({ routes }: Readonly<{ routes?: Contact["routes"] }>) {
  const t = useT();
  const { locale } = useLocale();
  // Absent, not empty: the caller has no roster grant, so naming a colleague is
  // a read they may not make. Silence is the honest answer — an "untried" badge
  // here would be a claim about the account rather than about the reader.
  if (!routes) {
    return null;
  }
  if (routes.untried) {
    return <Badge>{t("co.routes.untried")}</Badge>;
  }
  return (
    <span className="co-routes">
      {routes.top.map((route) => (
        <Badge
          key={route.user_id}
          tone={route.strength_bucket === "strong" ? "success" : undefined}
        >
          {route.display_name}
        </Badge>
      ))}
      {routes.remainder > 0 && (
        // The count, not a silent truncation: three names with nothing after
        // them reads as "these are the only three".
        <span className="t-caption">
          {t("co.routes.more", {
            count: formatNumber(routes.remainder, locale),
          })}
        </span>
      )}
    </span>
  );
}

/**
 * RouteInAction answers one question about one person: who here already talks
 * to them.
 *
 * It replaces the standing connections card, which asked nobody and answered
 * everybody — a staff directory in the rail of every account, costing a graph
 * read on every page load. This reads the same endpoint and only when someone
 * asks, and it asks the question from the end a rep actually has: not "who is
 * Lars in contact with" but "how do I reach Dana".
 *
 * "Nobody yet" is an answer worth giving, so it is given. The alternative — a
 * button that opens onto nothing — makes the reader wonder whether the read
 * failed.
 */
// The server's own bands, in this surface's words. Derived nowhere: the score
// behind them is the black box AC-company-3 took off this page.
const ROUTE_BAND_LABELS: Record<StrengthBucket, MessageKey> = {
  strong: "co.routeIn.band.strong",
  moderate: "co.routeIn.band.some",
  weak: "co.routeIn.band.faint",
  none: "co.routeIn.band.unknown",
};

function RouteInAction({
  orgId,
  contact,
}: Readonly<{ orgId: string; contact: Contact }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const titleId = useId();
  // The read is armed by opening, so a page nobody asks costs no graph query.
  const query = useOrganizationGraph(orgId, open);
  const graph = query.data;
  const readable = Array.isArray(graph?.nodes) ? graph : undefined;
  const routes = readable ? routesTo(readable, contact.person_id) : [];

  return (
    <>
      <button
        type="button"
        className="link-button"
        onClick={() => setOpen(true)}
      >
        {t("co.routeIn.open")}
      </button>
      {open && (
        <Modal open onClose={() => setOpen(false)} labelledBy={titleId}>
          <h2 id={titleId} className="t-h2 modal-title">
            {t("co.routeIn.title", { name: contact.full_name })}
          </h2>
          {query.isPending && <Skeleton width="100%" height={64} />}
          {/* A failed read is unavailable, never "nobody knows them": the two
              call for opposite next moves, and only a read that succeeded can
              say the second. */}
          {(query.isError || (!query.isPending && !readable)) && (
            <p className="surfacestate-withheld">
              {t("co.section.unavailable")}
            </p>
          )}
          {/* "Nobody" is a claim about the account, and only a COMPLETE read
              can make it. The graph caps its contact ring and withholds whole
              groups the caller may not read, so a page showing 25 contacts can
              hold a graph that saw 15 — and a rep told "nobody here has
              written to them" would stop looking. Partial says so instead. */}
          {readable && routes.length === 0 && (
            <EmptyState>
              {t(
                incompleteGraph(readable)
                  ? "co.routeIn.partial"
                  : "co.routeIn.none",
              )}
            </EmptyState>
          )}
          {/* The same incompleteness, said over a list that DID find someone.
              Stated only when the list is empty, a page that read 15 of 25
              contacts presented the routes it happened to see as all of them,
              and a rep who found one stopped looking for a better one. */}
          {readable && routes.length > 0 && incompleteGraph(readable) && (
            <p className="t-caption">{t("co.routeIn.mayBeMore")}</p>
          )}
          {readable && routes.length > 0 && (
            <ul className="co-list">
              {routes.map((route) => (
                <li key={route.id} className="co-row">
                  <span>{route.label}</span>
                  {/* The strength band, not the number: the score itself is
                      the black box AC-company-3 took off this page. */}
                  <span className="co-row-meta">
                    {t(ROUTE_BAND_LABELS[route.bucket])}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Modal>
      )}
    </>
  );
}

/**
 * ConsentChip reports the contact's outbound consent. Consent is per
 * purpose and default-deny, so the chip reads GRANTED only when at least one
 * purpose is granted, and an empty map reads "none on file" rather than
 * silently looking permissive.
 */
function ConsentChip({ consent }: Readonly<{ consent: Contact["consent"] }>) {
  const t = useT();
  const states = Object.values(consent);
  if (states.some((state) => state === "granted")) {
    return <Badge tone="success">{t("co.people.consentGranted")}</Badge>;
  }
  if (states.some((state) => state === "withdrawn")) {
    return <Badge tone="danger">{t("co.people.consentWithdrawn")}</Badge>;
  }
  return <Badge>{t("co.people.consentUnknown")}</Badge>;
}

/** DealsCard lists the open pipeline plus the two lifetime figures. */
export function DealsCard({
  view,
  actions,
  extra,
  loading = false,
}: Readonly<{
  view?: Organization360;
  // The verbs that change this section, rendered under it. Absent on an
  // archived record, which takes no new deals.
  actions?: ReactNode;
  // Whatever else belongs beside this account's deals — the Deals tab hands
  // in the last offer read here rather than drawing it as a second card, so
  // the two readings that both start from "this account's open deals" stop
  // reading as two different sections.
  extra?: ReactNode;
  // The composite read's own pending flag — see sectionState's own doc. The
  // Deals tab already gates its own skeleton on `!view && !failed` before
  // this ever renders, so `view` is always defined by the time this call
  // runs; passed anyway so the card is correct on its own terms rather than
  // depending on a caller's guard it cannot see.
  loading?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const deals = view?.deals;
  const won = deals?.won_lifetime;
  const state = sectionState(
    view,
    "deals",
    Boolean(deals),
    deals?.data.length ?? 0,
    loading,
  );
  const present = state === "ready" || state === "empty";
  return (
    <Panel
      title={t("co.deals.title")}
      titleAction={present ? actions : undefined}
      footer={
        deals && (
          <p className="co-row-meta">
            <span>
              {t("co.deals.wonLifetime")}{" "}
              {formatMoneyOrAbsent(won?.amount_minor, won?.currency, locale)}
            </span>
            <span>
              {t("co.deals.lostCount", {
                count: formatNumber(deals.lost_count, locale),
              })}
            </span>
          </p>
        )
      }
    >
      {present ? (
        <>
          {(deals?.data ?? []).map((deal) => (
            <DealRow key={deal.deal_id} deal={deal} />
          ))}
          {state === "empty" && (
            <PanelBody>
              <p className="surfacestate-empty">{t("co.deals.empty")}</p>
            </PanelBody>
          )}
          {extra}
        </>
      ) : (
        <PanelBody>
          <SurfaceState state={state} emptyLabel={t("co.deals.empty")}>
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}

function DealRow({ deal }: Readonly<{ deal: Deal360 }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <PanelRow className="co-row">
      <button
        type="button"
        className="co-rowlink"
        onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
      >
        {deal.name}
      </button>
      <span className="co-row-meta">
        <span>{deal.stage_name ?? t("co.deals.noStage")}</span>
        {deal.amount?.amount_minor != null && (
          <span className="t-mono">
            {formatMoneyOrAbsent(
              deal.amount.amount_minor,
              deal.amount.currency,
              locale,
            )}
          </span>
        )}
        {deal.stalled && <Badge tone="warn">{t("deal.stalled")}</Badge>}
      </span>
    </PanelRow>
  );
}

/**
 * CommercialPanel is the overview's own reading of the pipeline: the two
 * lifetime figures the deals section actually carries, then the open deals
 * themselves. It is deliberately not DealsCard reused wholesale — the Deals
 * tab keeps that card in full, and this is the shorter reading a rep gets
 * without leaving Overview.
 *
 * No open-pipeline total is drawn: nothing in Organization360 sums the open
 * deals' amounts, and inventing one here would be exactly the fabricated
 * figure the deals section's own honesty rule forbids.
 */
export function CommercialPanel({
  view,
  titleAction,
  extra,
  onAllDeals,
  loading = false,
  figuresOnly = false,
}: Readonly<{
  view?: Organization360;
  // The "new deal" verb, gated by the caller on the record being writable.
  titleAction?: ReactNode;
  // What else belongs to this account's commercial standing but is not read
  // off its deals — the overview hands in what it is already under contract
  // for, rather than a second card repeating "the commercial picture" under
  // its own heading.
  //
  // Rendered OUTSIDE the deals branch below, unlike DealsCard's slot of the
  // same name: the two readings answer to different grants, and a reader who
  // may see contracts and not deals would otherwise lose theirs to somebody
  // else's permission.
  extra?: ReactNode;
  onAllDeals?: () => void;
  // The composite read's own pending flag — see sectionState's own doc.
  loading?: boolean;
  // Draw the FIGURES and the contract block without this card's own header
  // band or its list of deals, for a caller that already lists them. The
  // Company 360 card does: its work section names every open deal with the
  // reason it needs a person, and repeating them underneath would show each
  // deal twice on one screen.
  //
  // The figures are what does not appear there — what the account has won
  // over its life and how much it has lost — so this is the half of the
  // reading the work list cannot carry, not a second copy of it.
  figuresOnly?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const deals = view?.deals;
  const state = sectionState(
    view,
    "deals",
    Boolean(deals),
    deals?.data.length ?? 0,
    loading,
  );
  const present = state === "ready" || state === "empty";
  // The section is a page of `deals.data` with `has_more` beside it — past
  // the cap this reads as the whole open pipeline unless it says otherwise.
  const truncated = deals?.page.has_more === true;
  const figures = state === "ready" && deals && (
    <PanelBody className="co-figures">
      <CommercialFigure
        label={t("co.deals.wonLifetime")}
        value={formatMoneyOrAbsent(
          deals.won_lifetime?.amount_minor,
          deals.won_lifetime?.currency,
          locale,
        )}
      />
      <CommercialFigure
        label={t("co.commercial.lostFigure")}
        value={formatNumber(deals.lost_count, locale)}
      />
    </PanelBody>
  );
  if (figuresOnly) {
    // The contract block first, for the same reason it leads the whole card:
    // what the account is already signed for frames the deals still moving.
    return (
      <>
        {extra}
        {figures}
        {/* `figures` covers `ready` alone. Every other state still owes the
            reader a sentence — an empty section is a FACT about the account
            and a withheld one is a fact about the reader, and a section that
            fell silently blank would be read as neither. */}
        {state !== "ready" && (
          <PanelBody>
            <SurfaceState state={state} emptyLabel={t("co.deals.empty")}>
              {null}
            </SurfaceState>
          </PanelBody>
        )}
      </>
    );
  }
  return (
    <Panel
      title={t("co.commercial.title")}
      titleAction={present ? titleAction : undefined}
      footer={
        present && (onAllDeals || truncated) ? (
          <>
            {truncated && (
              <p className="co-row-meta">{t("co.commercial.truncated")}</p>
            )}
            {onAllDeals && (
              <Button small variant="ghost" onClick={onAllDeals}>
                {t("co.commercial.allDeals")}
              </Button>
            )}
          </>
        ) : undefined
      }
    >
      {/* Before the pipeline, and before the panel's own deals footer: what
          the account is already signed for frames the deals that are still
          moving, and the Deals tab reads in that order too. */}
      {extra}
      {state === "ready" && deals ? (
        <>
          {figures}
          {deals.data.map((deal) => (
            <PanelRow key={deal.deal_id} className="co-commercial-row">
              <button
                type="button"
                className="co-rowlink co-commercial-name"
                onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
              >
                <span className="co-commercial-title">{deal.name}</span>
                {deal.expected_close_date && (
                  <span className="co-commercial-sub">
                    {t("commercial.closes", {
                      when: formatDate(
                        deal.expected_close_date,
                        locale,
                        recordZone,
                      ),
                    })}
                  </span>
                )}
              </button>
              <span className="co-row-meta">
                {deal.stage_name && <Badge>{deal.stage_name}</Badge>}
                {deal.amount?.amount_minor != null && (
                  <span className="t-mono">
                    {formatMoneyOrAbsent(
                      deal.amount.amount_minor,
                      deal.amount.currency,
                      locale,
                    )}
                  </span>
                )}
              </span>
            </PanelRow>
          ))}
        </>
      ) : (
        <PanelBody>
          <SurfaceState state={state} emptyLabel={t("co.deals.empty")}>
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}

// One eyebrow-labelled figure. Shared shape with the finance panel, so the
// two read as the same kind of reading rather than two different cards that
// happen to sit near each other.
function CommercialFigure({
  label,
  value,
}: Readonly<{ label: string; value: string }>) {
  return (
    <div className="co-figure">
      <Eyebrow>{label}</Eyebrow>
      {/* A figure the page does not have still occupies its slot, as the
          absence its formatter returned: the reader sees WHICH reading is
          missing rather than a shorter row that reads as complete. */}
      <span className="co-figure-value">{value}</span>
    </div>
  );
}

// How many entries the overview's chronology carries. The full history is the
// History tab; this is "what happened lately" without leaving Overview.
const RECENT_ACTIVITY_LIMIT = 5;

// One run of the timeline under the day it happened. Consecutive rather than
// grouped-by-key, because `entries` arrives newest-first from the server and a
// day is never revisited later in the same page.
type ActivityDay = { key: string; entries: TimelineEntry[] };

function groupByDay(
  entries: readonly TimelineEntry[],
  locale: Locale,
  recordZone: string,
) {
  const days: ActivityDay[] = [];
  for (const entry of entries) {
    const key = formatDate(entry.atIso, locale, recordZone);
    const last = days.at(-1);
    if (last?.key === key) {
      last.entries.push(entry);
    } else {
      days.push({ key, entries: [entry] });
    }
  }
  return days;
}

/**
 * RecentActivityPanel is the overview's chronology: the same activities
 * section the rail used to carry, grouped under the day they happened rather
 * than as a flat list — a day is one thing that happened, several messages
 * are how it happened.
 *
 * Reads the SAME activities section the account's Suggestions and health
 * cards read, so the story here cannot disagree with what they cite.
 */
export function RecentActivityPanel({
  view,
  onOpenHistory,
  loading = false,
  bare = false,
}: Readonly<{
  view?: Organization360;
  // Where the header's link leads. Absent for a caller with no History tab
  // of its own (the stories file).
  onOpenHistory?: () => void;
  // The composite read's own pending flag — see sectionState's own doc.
  loading?: boolean;
  // Render the BODY without this card's own header band, for a caller that
  // holds the Panel itself and labels the section. One implementation, two
  // mounts: the Deals tab still draws the whole card, and the Company 360
  // card draws this section inside its own chrome — a second copy of the
  // timeline is how two surfaces come to disagree about what happened.
  bare?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const recordZone = useRecordZone();
  // Every logged activity, not only the ones with a subject: a call or a note
  // often has none, and filtering them out here would under-report the
  // chronology and — because the count feeds sectionState — draw "nothing
  // logged with them yet" on an account that has been called five times.
  const logged = view?.activities?.data ?? [];
  const state = sectionState(
    view,
    "activities",
    Boolean(view?.activities),
    logged.length,
    loading,
  );
  const days = groupByDay(
    activityTimeline(logged.slice(0, RECENT_ACTIVITY_LIMIT), viewerId),
    locale,
    recordZone,
  );
  const body =
    state === "ready" ? (
      days.map((day) => (
        <div key={day.key} className="co-timeline-day">
          {/* One level under whatever names this timeline: the card's own
              title when it stands alone, the section subhead when it is a
              section of the Company 360 card. */}
          <Eyebrow as={bare ? "h4" : "h3"} className="co-timeline-day-heading">
            {day.key}
          </Eyebrow>
          <ul className="timeline">
            {day.entries.map((entry) => (
              <TimelineRow key={entry.id} entry={entry} zone={recordZone} />
            ))}
          </ul>
        </div>
      ))
    ) : (
      <PanelBody>
        <SurfaceState state={state} emptyLabel={t("co.recent.empty")}>
          {null}
        </SurfaceState>
      </PanelBody>
    );
  if (bare) {
    return <>{body}</>;
  }
  return (
    <Panel
      title={t("co.recent.title")}
      titleAction={
        onOpenHistory && (
          <Button small variant="ghost" onClick={onOpenHistory}>
            {t("co.recent.viewHistory")}
          </Button>
        )
      }
    >
      {body}
    </Panel>
  );
}

/**
 * NextSteps is the middle column's first block: the open tasks on this
 * account, overdue first, each showing what it is linked to.
 *
 * The tick is `update`'s own verb (`TaskCompleteCheck`), not `renderAction`'s
 * — a row names its primary move as the row, not as one more item in a menu.
 * `renderAction` is left for whatever ELSE a caller wants beside the tick
 * (snooze), and stays hidden until the row is hovered or focused, since a
 * list of open tasks reads by title and due date first and only reveals its
 * verbs on approach.
 */
export function NextSteps({
  view,
  renderAction,
  onOpenTask,
  update,
}: Readonly<{
  view: Organization360;
  renderAction?: (step: NextStep) => ReactNode;
  // Given, the subject opens the task where it is listed. Absent, it stays
  // plain text rather than a button that goes nowhere.
  onOpenTask?: (step: NextStep) => void;
  // Wires the tick to the real completion write. Absent (the stories file,
  // a read-only account) draws the row with no checkbox at all — a box that
  // cannot be ticked is worse than no box.
  update?: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const steps = view.next_steps?.data ?? [];
  const state = sectionState(
    view,
    "next_steps",
    Boolean(view.next_steps),
    steps.length,
  );
  // A withheld block is dropped entirely — the middle column is the story,
  // and a refusal in the middle of it says nothing a rep can act on. Every
  // other state is shown, because "no open task" and "we could not tell"
  // lead to different next moves.
  if (state === "withheld") {
    return null;
  }
  return (
    <Panel title={t("co.next.title")}>
      {state === "unavailable" && (
        <PanelBody>
          <p className="surfacestate-withheld">{t("co.section.unavailable")}</p>
        </PanelBody>
      )}
      {state === "empty" && (
        <PanelBody>
          <p className="surfacestate-empty">{t("co.next.empty")}</p>
        </PanelBody>
      )}
      {state === "ready" &&
        steps.map((step) => (
          <PanelRow key={step.activity_id} className="co-task-row">
            {update && (
              <TaskCompleteCheck
                activityId={step.activity_id}
                update={update}
              />
            )}
            <span className="co-task-body">
              {onOpenTask ? (
                <button
                  type="button"
                  className="co-rowlink"
                  onClick={() => onOpenTask(step)}
                >
                  {step.subject}
                </button>
              ) : (
                <span>{step.subject}</span>
              )}
              <span className="co-row-meta">
                {step.overdue && (
                  <Badge tone="danger">{t("co.next.overdue")}</Badge>
                )}
                {!step.overdue && step.due_at && (
                  <span>
                    {t("co.next.due", {
                      // The one viewer-clock reading on this record page, and
                      // it is not a preference: `dueInstant` mints a due date
                      // as the end of the picked day in the BROWSER's zone, so
                      // the stored instant already carries the picker's clock.
                      // Read in the organization's zone it names a different
                      // calendar day than the one the picker chose, for every
                      // reader outside that zone — there is no organization
                      // reading of it to prefer. The timeline below still reads
                      // in the record zone, because an activity's occurrence IS a
                      // fact about the record.
                      when: formatDate(step.due_at, locale, viewerZone()),
                    })}
                  </span>
                )}
                {!step.due_at && <span>{t("co.next.undated")}</span>}
                {step.linked_deal_id && (
                  <EntityRef kind="deal" id={step.linked_deal_id} />
                )}
                {step.linked_person_id && (
                  <EntityRef kind="person" id={step.linked_person_id} />
                )}
                {step.assignee_id && (
                  <EntityRef kind="user" id={step.assignee_id} />
                )}
              </span>
            </span>
            {renderAction && (
              <span className="co-task-verbs">{renderAction(step)}</span>
            )}
          </PanelRow>
        ))}
    </Panel>
  );
}

type Question = components["schemas"]["OrganizationQuestion"];
type Suggestion = components["schemas"]["Organization360Suggestion"];
type Answer = components["schemas"]["OrganizationAnswer"];
// The prepared questions, in the order the card offers them: what is open now,
// then what to walk in with, then what has moved.
//
// Keyed by question rather than listed, so the type is EXHAUSTIVE: a question
// declared upstream and not given a position here fails to compile, instead of
// shipping a server that answers it and a card that never asks.
const QUESTIONS: readonly Question[] = Object.keys({
  whats_open: 0,
  meeting_prep: 0,
  whats_changed: 0,
} satisfies Record<Question, 0>) as Question[];

/**
 * AskCard is "Ask Margince": three prepared questions, answered from this
 * account's own records.
 *
 * The questions are BUTTONS, not a text box. Each one names the records its
 * answer is written from, which is what lets every sentence carry a citation
 * the reader can open — and a text box that quietly answered from a subset
 * would look exactly like one that had searched everything.
 */
export function AskSection({
  orgId,
  enabled,
  onOpenRecord,
  projects,
}: Readonly<{
  orgId: string;
  enabled: boolean;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // The account's projects, as the page read them. Offered as a picker
  // when any is live, so a question can be asked about one engagement
  // rather than the whole account.
  projects?: readonly PickableProject[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [projectId, setProjectId] = useState("");
  const recordZone = useRecordZone();
  const live = liveProjects(projects);
  useSoleProjectDefault(live, projectId, setProjectId);
  useClearVanishedChoice(live, projectId, setProjectId);
  const ask = useMutation({
    // The project travels as the mutation variable beside the question, so
    // a stale closure cannot ask about a project the picker no longer shows.
    mutationFn: async ({
      question,
      project,
    }: {
      question: Question;
      project: string;
    }) => {
      const { data, error } = await api.POST("/organizations/{id}/ask", {
        params: { path: { id: orgId } },
        body: { question, ...(project ? { project_id: project } : {}) },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  if (!enabled) {
    return null;
  }
  const answer: Answer | undefined = ask.data;
  // A payload without sentences is an answer this build cannot read, not an
  // account with nothing to say — the same distinction every card here keeps.
  const readable = Array.isArray(answer?.sentences) ? answer : undefined;
  return (
    <section className="co-part" aria-label={t("co.ask.title")}>
      <Eyebrow as="h3">{t("co.ask.title")}</Eyebrow>
      <ProjectPicker
        projects={live}
        projectId={projectId}
        onChange={(next) => {
          setProjectId(next);
          // The answer on screen was written about the previous project, and
          // its scope line would otherwise stand over the next project's key.
          ask.reset();
        }}
        scope={readable?.scope}
      />
      <p className="co-ask-questions">
        {QUESTIONS.map((question) => (
          <Button
            key={question}
            small
            onClick={() => ask.mutate({ question, project: projectId })}
            disabled={ask.isPending}
          >
            {t(`co.ask.q.${question}`)}
          </Button>
        ))}
      </p>
      {ask.isPending && <Skeleton width="100%" height={40} />}
      {ask.isError && (
        <p className="surfacestate-withheld">
          {t("co.ask.failed")}
          {/* The server's own detail says WHICH failure — budget exhausted reads
              differently from a malformed request, and a rep can act on one. */}
          {` ${problemMessageOf(ask.error, t)}`}
        </p>
      )}
      {/* The previous answer is hidden while the next question is in flight.
          Leaving it under the spinner puts a finished answer next to a loading
          one, and the reader has no way to tell which question they are
          looking at the answer to. */}
      {readable && !ask.isPending && (
        <>
          {/* The question is repeated above its answer: three buttons and one
              answer block leaves the reader guessing which they pressed once
              they have scrolled, and the wrong pairing is worse than none. */}
          <p className="co-ask-asked">{t(`co.ask.q.${readable.question}`)}</p>
          {readable.sentences.length === 0 ? (
            // An empty answer is a real outcome, not a failure: the question's
            // records are not ones this reader can see, so there is nothing to
            // say. Saying that is honest; a sentence written around the gap
            // would not be.
            <p className="surfacestate-empty">{t("co.ask.nothing")}</p>
          ) : (
            <SentenceList
              sentences={readable.sentences}
              onOpenRecord={onOpenRecord}
            />
          )}
          <p className="co-row-meta">
            <WrittenBy by={readable.generated_by} />
            <span>
              {t("co.brief.generatedAt", {
                when: formatDate(readable.generated_at, locale, recordZone),
              })}
            </span>
          </p>
        </>
      )}
    </section>
  );
}

/**
 * SuggestionsCard is what this account looks like it needs next.
 *
 * Each row leads with the REASON the rule fired, because a rep must be able to
 * disagree with the reason rather than with a verdict they cannot inspect. A
 * dismissal is theirs alone and is keyed on the evidence, so the same advice
 * stays gone while the situation holds and comes back when it changes.
 */
type Health = NonNullable<Organization360["health"]>;
// One rated dimension of the account's health: the rating, and the sentence it
// was read from. Named here because three readings carry it as their basis.
type HealthDimension = NonNullable<Health["relationship"]>;

/**
 * HealthCard is how the relationship stands, in the parts a reader can act on
 * (AC-company-3).
 *
 * It replaced a single 0–100 score. That number was the MAX over the account's
 * contacts of a decayed message count, so one talkative contact spoke for the
 * whole account and a long, low-volume relationship read as near-dead. Each
 * line here names a fact instead: "no inbound for 90 days" says what to do,
 * where "2/100" said only a mood.
 *
 * A part the server could not compute is ABSENT, never zero. Zero is a claim
 * about the account; absence is a fact about the reading.
 */
// The rating vocabulary, worst first. The ORDER is the worst-of rule: a
// verdict is the lowest-ranked rating among the dimensions that have one
// (PO-AC-N-11).
export type StateStrip = NonNullable<Organization360["state_strip"]>;

// Whose move it is, in words. Exported (and no longer rendered by this file
// as a strip tile) because the daily brief's context band reads the same
// `engagement` field now — companytoday.tsx composes the label from here
// rather than re-deriving it.
export const ENGAGEMENT_LABELS: Record<
  NonNullable<StateStrip["engagement"]>["state"],
  MessageKey
> = {
  never_contacted: "co.strip.engagement.never_contacted",
  active: "co.strip.engagement.active",
  waiting_on_them: "co.strip.engagement.waiting_on_them",
  waiting_on_us: "co.strip.engagement.waiting_on_us",
  dormant: "co.strip.engagement.dormant",
};

// The two states that name a problem rather than a condition. Colouring only
// these keeps the brief from reading as a dashboard where every tile is lit.
export const ENGAGEMENT_TONE: Partial<
  Record<NonNullable<StateStrip["engagement"]>["state"], "warn">
> = {
  waiting_on_them: "warn",
  dormant: "warn",
};

// A reading the caller's grants withheld. Shared with the person record's
// readings row and rail rather than spelled per surface: all three state the
// same fact about the same reader, and a second spelling is exactly the drift
// that had these rows drawn by two different components in the first place.
const WITHHELD_READING: MessageKey = "record.notShown";

// A reading nobody has judged. It is NOT the withheld word — "you may not see
// this" and "there is no verdict yet" are opposite facts about who is missing
// what, and a slot that confuses them sends the reader to ask for an access
// grant that would show them nothing. Its own key rather than the lifecycle
// label it happens to match today: a rename of one must not silently move the
// other.
const UNASSESSED_READING: MessageKey = "co.strip.notAssessed";

/**
 * StateStrip is the KPI row directly under the header: FIVE slots, always five,
 * and every lifecycle draws the account's own standing — stage, open pipeline,
 * relationship, health — because a customer needs those readings at least as
 * much as a prospect does. Being a customer is not a reason to stop reporting
 * whether the relationship is healthy or what deals are open with them. What
 * the lifecycle changes is the fifth reading: a prospect is asked when the deal
 * lands, a customer what the money says.
 *
 * The standing readings come first, the money readings after: what is true of
 * the account regardless of what it has paid is the frame the money sits in,
 * not the other way round. A row that led with money would ask a reader to
 * read the account's health off a number they have not been given the
 * context for yet.
 *
 * EVERY SLOT ALWAYS DRAWS, and says honestly that it has no reading when it has
 * none. A slot that vanishes leaves the reader unable to tell WHICH reading is
 * missing — the row simply looks shorter — and only an empty state is allowed to
 * claim there is none (SurfaceState's rule). So the three absences are three
 * different words and never one: withheld is a fact about the READER, unassessed
 * is a fact about how much has been judged, and "no date" is a fact about the
 * ACCOUNT. Inventing "never contacted" out of a withheld engagement would state
 * the business conclusion a rep acts on, from a permission boundary.
 */
//
// What it must never render is the harder half of the rule, and every omission
// below is one of its bullets: no €0 when the figure is unavailable, no
// cross-currency sum without its conversion source, and nothing called
// "revenue" that is only a count of open deals.
export function StateStrip({
  orgId,
  view,
  onOpenTab,
}: Readonly<{
  orgId: string;
  view?: Organization360;
  // The tab each reading is a reading OF. Optional, because a surface that
  // draws these outside the record page (the storybook, a mirror) has no tab
  // strip to send anybody to — and a door with nowhere behind it is not drawn
  // rather than drawn dead.
  onOpenTab?: (tab: CompanyTab) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const strip = view?.state_strip;
  if (!strip) {
    // Absent for two different reasons, and only `sections_omitted` tells
    // them apart. The caller already withholds this component entirely while
    // the composite read is still in flight or has failed (`view` itself is
    // undefined then), so reaching this branch WITH a view means the read
    // succeeded and this one section did not — either it was withheld, or a
    // future account state has nothing here yet. Only the first is worth
    // saying: silently dropping the whole KPI row on a page that otherwise
    // rendered would read as "no readings for this account", the empty state
    // a permission boundary must never impersonate.
    if (view && omitted(view, "state_strip")) {
      return (
        <section className="co-strip-withheld" aria-label={t("co.strip.title")}>
          <p className="surfacestate-withheld">{t("co.section.restricted")}</p>
        </section>
      );
    }
    return null;
  }
  // A CURRENT customer only. A former one has invoices in its past, but "do
  // they pay us, and on time?" is not the question their page is opened with,
  // and leading with a money reading on an account that has stopped buying
  // reads as though the relationship were still running.
  const customer = strip.account.lifecycle === "customer";
  // The contract pairs an absent optional section with its name in
  // `sections_omitted` (Organization360), so the reason `health` did not arrive
  // is readable rather than guessable — and guessing is how a grant boundary
  // gets reported as an account nobody has assessed.
  const healthWithheld = view != null && omitted(view, "health");
  return (
    // Four free-standing cards, not the StatStrip plate. A strip is read ACROSS
    // as one comparison, which is what a row of unlike readings — a stage, a
    // date, a sum of money — actually wanted. These four are not unlike: they
    // are one verdict and the three dimensions it is computed from, and each is
    // a door into the tab that holds the detail. Cards read one at a time,
    // which is how a reader uses them.
    //
    // The region's name and the page rhythm are the SCREEN's to set; the tiles
    // inside are the shared primitive's, unreached-into.
    <section className="co-readings" aria-label={t("co.strip.title")}>
      <div className="co-readings-grid" data-testid="company-strip">
        {/* The verdict first, then what it is made of. Stage and expected close
            left the row with the plate: the lifecycle is read on the record's
            own name badge and the close date on the deal that carries it, and
            a reading in two places is one the reader has to reconcile. */}
        <AccountHealthStat
          health={view?.health}
          orgId={orgId}
          locale={locale}
          withheld={healthWithheld}
          t={t}
        />
        <PipelineCard
          commercial={strip.commercial}
          dimension={view?.health?.commercial}
          locale={locale}
          recordZone={recordZone}
          onOpen={onOpenTab && (() => onOpenTab("deals"))}
          t={t}
        />
        {/* Money is a reading every account gets now, not only a customer: on
            one we have never billed it says so, which is a fact about the
            account, where an absent fourth card is a hole the reader has to
            interpret. */}
        <MoneyStat
          orgId={orgId}
          locale={locale}
          customer={customer}
          dimension={view?.health?.payment}
          onOpen={onOpenTab && (() => onOpenTab("finance"))}
          t={t}
        />
        {/* Whose move it is and the worst open signal both live in the daily
            brief's context band (companytoday.tsx), off the same `engagement`
            and `signal` fields — so these cards carry the account's STANDING
            and the brief carries what is DATED. */}
        <HealthStat
          health={view?.health}
          locale={locale}
          withheld={healthWithheld}
          onOpen={onOpenTab && (() => onOpenTab("people"))}
          t={t}
        />
      </div>
    </section>
  );
}

/**
 * A median days-after-due as a sentence (FIN-FORM-3).
 *
 * Negative days mean they pay BEFORE the due date. "-4 days after due" is a
 * puzzle; "typically 4 days early" is the reading. Shared by the KPI slot and
 * the finance card so the two cannot come to describe earliness differently —
 * spelled twice, only one of the copies would be changed.
 */
export function medianDaysLabel(
  median: number,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  return median < 0
    ? t("finance.medianEarly", { days: formatNumber(Math.abs(median), locale) })
    : t("finance.medianAfterDue", { days: formatNumber(median, locale) });
}

// The caveat on a figure that IS shown but is not current. Undefined when the
// figure is current and needs none.
function staleDetailKey(
  state?: components["schemas"]["FinanceSummaryState"],
): MessageKey | undefined {
  switch (state) {
    case "stale":
      return "co.strip.fin.staleFigure";
    case "error":
      return "co.strip.fin.errorFigure";
    case "syncing":
      // The first pass has not finished, so what is shown may be partial.
      return "co.strip.fin.syncing";
    default:
      return undefined;
  }
}

// Why there is no figure, in the reader's terms. Each state has its own fix,
// and naming the wrong one costs the reader a trip to a settings page they did
// not need.
function financeDetailKey({
  pending,
  withheld,
  failed,
  state,
}: Readonly<{
  pending: boolean;
  withheld: boolean;
  failed: boolean;
  state?: components["schemas"]["FinanceSummaryState"];
}>): MessageKey {
  if (pending) {
    return "co.strip.fin.loading";
  }
  // Both before the state switch: with no answer there is no state to read,
  // and guessing one from its absence is how a denial became setup advice.
  if (withheld) {
    return "co.strip.fin.withheld";
  }
  if (failed) {
    return "co.strip.fin.error";
  }
  switch (state) {
    case "unmapped":
      return "co.strip.fin.unmapped";
    case "syncing":
      return "co.strip.fin.syncing";
    case "stale":
      return "co.strip.fin.staleFigure";
    case "error":
      return "co.strip.fin.error";
    case "connected":
      // A live, mapped source that produced no figure. Nothing is broken and
      // there is nothing to set up — we have simply never billed them, or no
      // invoice could be converted. Setup advice here sends the reader to fix
      // a connection that is already working.
      return "co.strip.fin.nothingBilled";
    default:
      // no_connection, and the read that never answered. Both mean there is
      // no source to read, which is the one case the setup advice fits.
      return "co.strip.fin.noConnection";
  }
}

/**
 * The customer row's ONE money slot: what this account has been invoiced over
 * the trailing year.
 *
 * One slot, not three. The strip is a GLANCE, and the Finance tab
 * (companyfinance.tsx) is where the detail lives — which is already why open
 * balance and the payment-habit median are not here. Spending three of five
 * slots on windows of the same figure buried the account's own standing behind
 * the money, and the standing is the frame the money is read in.
 *
 * The label follows the STATE, because the two states answer different
 * questions. With a figure it names the window the figure covers; with none it
 * says "Finance", because the reason there is none is usually a fact about the
 * connection rather than about that window — labelling a "connect your
 * accounting" slot "net invoiced, 12 months" claims we looked at those twelve
 * months and found nothing there.
 */
function MoneyStat({
  orgId,
  locale,
  customer,
  dimension,
  onOpen,
  t,
}: Readonly<{
  orgId: string;
  locale: Locale;
  // A CURRENT customer. Everyone else has never been invoiced, and the card
  // says exactly that rather than reporting a finance connection that has
  // nothing to do with them.
  customer: boolean;
  // The payment health reading, shown as this card's basis so the verdict a
  // reader meets on the health card can be checked against the money it was
  // read from, on the card that holds the money.
  dimension?: HealthDimension;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // The door out of this reading, handed to every shape it takes: a
  // withheld reading and a priced one are the same reading, and only one
  // of them offering the tab would make the way out look like a property
  // of the figure.
  const door = { openLabel: t("co.strip.open.finance"), onOpen };
  // The SAME query the finance card and the payment health dimension run, so
  // every money reading on one page agrees and all but the first cost no
  // request.
  const { data, isPending, isError, error } = useFinanceSummary(orgId);
  const basis = dimension ? (
    <FactList
      facts={[
        {
          key: "payment",
          term: t(HEALTH_DIMENSION_LABEL.payment),
          value: t(HEALTH_RATING_LABEL[dimension.rating]),
          note: dimension.reason,
        },
      ]}
    />
  ) : undefined;
  // Never invoiced is a fact about the ACCOUNT, and it outranks every state the
  // finance connection could be in: a prospect on an installation with no
  // accounting connected must not be told to go and connect one, and a
  // prospect on an installation that HAS one must not read as though we had
  // billed them and got nothing.
  if (!customer) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.finance")}
        value={t("co.strip.financeUnknown")}
        detail={t("co.strip.fin.notACustomer")}
      />
    );
  }
  // A refusal is not a failure and neither is a setup gap. A reader whose role
  // cannot see finance told to "connect your accounting" is sent to a settings
  // page to fix a permission — the one thing they cannot fix from there.
  const withheld = isError && problemCodeOf(error) === "permission_denied";
  const amount = data?.net_invoiced;
  const caveat = staleDetailKey(data?.state);
  // No figure is not €0, and the six reasons there is none are not one reason.
  // "Connect your accounting" is wrong advice for a connection that exists and
  // is syncing, stale, errored or unmatched — it sends the reader to set up
  // something they already have.
  if (!amount || amount.amount_minor == null || !amount.currency) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.finance")}
        value={t("co.strip.financeUnknown")}
        detail={t(
          financeDetailKey({
            pending: isPending,
            withheld,
            failed: isError && !withheld,
            state: data?.state,
          }),
        )}
        basisLabel={basis ? t("co.strip.basis.reading") : undefined}
        basis={basis}
      />
    );
  }
  return (
    <StatCard
      {...door}
      label={t("co.strip.netInvoiced")}
      value={formatMoneyCompact(amount.amount_minor, amount.currency, locale)}
      // The provider name goes in the detail line rather than beside the label:
      // a strip slot is narrower than a free-standing stat card, and a badge
      // beside the label wraps onto its own row underneath it, standing the one
      // slot that names its source taller than every sibling in the row.
      //
      // A figure that is not current is shown WITH its caveat rather than
      // withheld: the last known number is usually the right one, and hiding it
      // tells the reader less than showing it qualified would. The caveat takes
      // the line ahead of the provider name, because which accounting system a
      // figure came from matters less than whether the figure is current — and
      // the Finance tab names the connection anyway.
      //
      // Stale and error say DIFFERENT things, which is why `staleDetailKey` does
      // not fold them: `stale` is a sync that SUCCEEDED, just long enough ago
      // that the date matters; `error` is the last good answer after an attempt
      // that failed. Calling either one the other is a wrong claim about whether
      // anything is broken.
      // Lifetime rides in the detail line rather than taking a slot of its own:
      // beside the trailing year it is the one comparison nothing else on this
      // page carries — what the account has ever been worth against what it has
      // been worth lately — and two money slots on a five-slot row would make
      // this row a finance report rather than a glance.
      //
      // Overdue and the open balance stay OUT, and not for want of room. The
      // Finance tab renders both as headline figures one tab away, so a copy
      // here is a second answer to the same question, read at a glance and
      // drifting from the first the moment either changes.
      detail={
        caveat
          ? t(caveat)
          : moneyPhrase(
              "co.strip.lifetimeOf",
              data?.net_invoiced_lifetime,
              locale,
              t,
            ) ||
            data?.provider ||
            undefined
      }
      basisLabel={basis ? t("co.strip.basis.reading") : undefined}
      basis={basis}
    />
  );
}

// One money figure as a phrase for the detail line, or nothing. Both halves are
// required and neither absence has a substitute: a figure with no currency
// cannot be rendered at all, and a zero the server did not send would say this
// account owes us nothing when the truth is that nobody has told us.
function moneyPhrase(
  key: "co.strip.lifetimeOf",
  amount:
    | { amount_minor?: number | null; currency?: string | null }
    | undefined,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string | undefined {
  return amount?.amount_minor != null && amount.currency
    ? t(key, {
        amount: formatMoneyCompact(
          amount.amount_minor,
          amount.currency,
          locale,
        ),
      })
    : undefined;
}

type StripCommercial = NonNullable<
  NonNullable<Organization360["state_strip"]>["commercial"]
>;

// Open pipeline, labelled as exactly what it is: the sum of open deals, never
// "potential" and never "revenue" (§4.2). Unpriced when nothing on the account
// carries a convertible figure — a €0 there would claim a priced pipeline
// worth nothing, where the truth is the page cannot price it.
function PipelineCard({
  commercial,
  dimension,
  locale,
  recordZone,
  onOpen,
  t,
}: Readonly<{
  commercial?: StripCommercial | null;
  // The commercial health reading, shown as this card's basis so the verdict
  // on the health card can be checked against the deals it was read from, on
  // the card that holds those deals.
  dimension?: HealthDimension;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // The door out of this reading, handed to every shape it takes: a
  // withheld reading and a priced one are the same reading, and only one
  // of them offering the tab would make the way out look like a property
  // of the figure.
  const door = { openLabel: t("co.strip.open.deals"), onOpen };
  const basis = dimension ? (
    <FactList
      facts={[
        {
          key: "commercial",
          term: t(HEALTH_DIMENSION_LABEL.commercial),
          value: t(HEALTH_RATING_LABEL[dimension.rating]),
          note: dimension.reason,
        },
      ]}
    />
  ) : undefined;
  const basisProps = {
    basisLabel: basis ? t("co.strip.basis.reading") : undefined,
    basis,
  };
  if (!commercial) {
    // A null `commercial` is the contract's way of saying the caller has no
    // deal grant, so this is the READER's boundary and not an account with
    // nothing running. "No open deals" here would be the business conclusion a
    // rep acts on, invented out of a permission.
    return (
      <StatCard
        {...door}
        label={t("co.strip.pipeline")}
        value={t(WITHHELD_READING)}
        {...basisProps}
      />
    );
  }
  // No open deals is not an unpriced pipeline. Saying "no convertible amount"
  // about an account that has nothing open reports a data problem where the
  // truth is simply that nothing is running.
  if (commercial.open_count === 0) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.pipeline")}
        value={t("co.strip.noOpenDeals")}
        {...basisProps}
      />
    );
  }
  const { open_pipeline_minor_base: value, base_currency: currency } =
    commercial;
  const stalled =
    commercial.stalled_count > 0
      ? t("co.strip.stalled", {
          count: formatNumber(commercial.stalled_count, locale),
        })
      : undefined;
  if (value == null || !currency) {
    // Open deals with no priceable figure still say how many there are: the
    // count is a fact, the money is not. The unpriced note is never dropped
    // for the stalled one — a reader who is told only "1 stalled" has no way
    // to know the pipeline was never priced at all.
    return (
      <StatCard
        {...door}
        label={t("co.strip.pipeline")}
        value={t("co.strip.openDeals", {
          count: formatNumber(commercial.open_count, locale),
        })}
        detail={join(t("co.strip.unpriced"), stalled)}
        tone={stalled ? "warn" : undefined}
        {...basisProps}
      />
    );
  }
  // Everything qualifying this figure travels WITH it. §4.2 forbids a
  // cross-currency sum without an explicit conversion source and as-of date,
  // and forbids a total that silently covers only part of the pipeline — so a
  // partial total names its share, and a converted one names the oldest rate
  // date standing behind it.
  const partial = commercial.priced_count < commercial.open_count;
  const converted =
    commercial.converted_count > 0 && commercial.fx_as_of
      ? t("co.strip.convertedAsOf", {
          count: formatNumber(commercial.converted_count, locale),
          date: formatDate(commercial.fx_as_of, locale, recordZone),
        })
      : undefined;
  return (
    <StatCard
      {...door}
      label={t("co.strip.pipeline")}
      value={formatMoney(value, currency, locale)}
      tone={stalled ? "warn" : undefined}
      detail={join(
        partial
          ? t("co.strip.pricedPartly", {
              priced: formatNumber(commercial.priced_count, locale),
              total: formatNumber(commercial.open_count, locale),
            })
          : t("co.strip.openDeals", {
              count: formatNumber(commercial.open_count, locale),
            }),
        converted,
        stalled,
      )}
      {...basisProps}
    />
  );
}

// One detail line from the parts that apply. A card has room for one, and
// dropping a part because another is present is how a qualification goes
// missing exactly when it matters.
function join(...parts: (string | undefined)[]): string {
  return parts.filter(Boolean).join(" · ");
}

// Health as a STATUS with its reason, never a 0-100 verdict (§4.2). The card
// below the fold decomposes it; this says which way it points and why.
//
// It reports the BALANCE of the exchange rather than its recency, because the
// daily brief already answers "whose move is it" — two readings saying "in
// conversation" in different words is one reading's worth of information taking
// two slots of five. A relationship where they write and we do not answer, and
// one where we write into silence, are both "in conversation" by recency and are
// opposite problems.
function HealthStat({
  health,
  locale,
  withheld,
  onOpen,
  t,
}: Readonly<{
  health?: Health;
  locale: Locale;
  withheld: boolean;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // The door out of this reading, handed to every shape it takes: a
  // withheld reading and a priced one are the same reading, and only one
  // of them offering the tab would make the way out look like a property
  // of the figure.
  const door = { openLabel: t("co.strip.open.people"), onOpen };
  const dimension = health?.relationship;
  const basisProps = {
    basisLabel: dimension ? t("co.strip.basis.reading") : undefined,
    basis: dimension ? (
      <FactList
        facts={[
          {
            key: "relationship",
            term: t(HEALTH_DIMENSION_LABEL.relationship),
            value: t(HEALTH_RATING_LABEL[dimension.rating]),
            note: dimension.reason,
          },
        ]}
      />
    ) : undefined,
  };
  if (!health) {
    // No health section at all. Withheld says so; anything else has simply not
    // been assessed. Neither is "they have never written" — that is a claim
    // about the account this read has no basis for.
    return (
      <StatCard
        {...door}
        label={t("co.strip.health")}
        value={t(withheld ? WITHHELD_READING : UNASSESSED_READING)}
      />
    );
  }
  const days = health.days_since_last_inbound;
  if (days == null) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.health")}
        value={t("co.strip.noInboundEver")}
        tone="warn"
        {...basisProps}
      />
    );
  }
  if (days > HEALTH_QUIET_DAYS) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.health")}
        value={t("co.strip.healthQuiet")}
        tone="warn"
        detail={t("co.health.sinceInbound", {
          days: formatNumber(days, locale),
        })}
        {...basisProps}
      />
    );
  }
  // A live relationship: say who is carrying it. Below a third of the
  // exchange coming from them is us talking to ourselves, whatever the dates
  // say; above two thirds they are asking more than we are answering.
  const share = health.reply_balance;
  if (share == null) {
    return (
      <StatCard
        {...door}
        label={t("co.strip.health")}
        value={t("co.strip.healthActive")}
        {...basisProps}
      />
    );
  }
  const percent = Math.round(share * 100);
  const oneSided = share < 0.34 || share > 0.66;
  return (
    <StatCard
      {...door}
      label={t("co.strip.health")}
      value={
        oneSided ? t("co.strip.healthOneSided") : t("co.strip.healthBalanced")
      }
      tone={oneSided ? "warn" : undefined}
      detail={t("co.strip.replyShare", {
        percent: formatNumber(percent, locale),
      })}
      {...basisProps}
    />
  );
}

// The threshold that separates a live conversation from a quiet one. It names
// a number the strip states rather than one the reader must infer from a date,
// and it is deliberately the same span the dormant engagement state uses.
const HEALTH_QUIET_DAYS = 30;

// AccountHealthStat leads the readings: the rated dimensions' worst verdict,
// how many of the possible three were rated at all, and — behind a disclosure —
// the three themselves with the sentence each was read from.
//
// The verdict and its constituents in ONE card rather than four: the three
// cards beside this one answer "what is the deals/money/relationship picture",
// which is a different question from "is this account healthy". Splitting the
// verdict from what computes it is what left a reader holding a summary they
// could not check.
//
// With nothing rated it says so and keeps its denominator. It must not borrow a
// verdict `worstOf` itself declined to give, and it must not carry a tone: a
// grey slot with no words reads as a reading that failed to load, where "not
// assessed, 0 of 3 rated" says which reading is missing and how far off a
// verdict the account is.
// The sharpest thing wrong with the account, named and explained: "Payment —
// three invoices are past due, the oldest by 18 days".
//
// ONE dimension, not all of them. A card is read at a glance, and three
// sentences stacked in a caption is a paragraph the reader skips — the receipt
// behind the card still lists every dimension with its own reason. A dimension
// rated at risk with nothing written about it is skipped rather than named with
// silence after it, which reads as a card that failed to finish its sentence.
function worstReason(
  dimensions: readonly {
    key: "relationship" | "commercial" | "payment";
    dimension?: { rating: string; reason?: string | null };
  }[],
  t: ReturnType<typeof useT>,
): ReactNode {
  const worst = dimensions.find(
    (entry) => entry.dimension?.rating === "at_risk" && entry.dimension.reason,
  );
  if (!worst?.dimension?.reason) {
    return null;
  }
  return (
    <span>
      {t("co.strip.healthSummary.because", {
        dimension: t(HEALTH_DIMENSION_LABEL[worst.key]),
        reason: worst.dimension.reason,
      })}
    </span>
  );
}

function AccountHealthStat({
  health,
  locale,
  orgId,
  withheld,
  t,
}: Readonly<{
  health?: Health;
  locale: Locale;
  orgId: string;
  withheld: boolean;
  t: ReturnType<typeof useT>;
}>) {
  const { overall, rated, payment } = useAccountStanding(orgId, health);
  const dimensions = [
    { key: "relationship", dimension: health?.relationship },
    { key: "commercial", dimension: health?.commercial },
    { key: "payment", dimension: payment },
  ] as const;
  // Only the dimensions that were actually rated. An unrated one is not a
  // failing one, and listing it as a blank row would invite exactly that
  // reading — `worstOf`'s denominator above already says how many are missing.
  //
  // `flatMap` rather than `filter` so the absent case is dropped where the type
  // can see it: a filter leaves the caller holding an optional it then has to
  // assert away, and an assertion is a claim the compiler stopped checking.
  const basisFacts = dimensions.flatMap(({ key, dimension }) =>
    dimension
      ? [
          {
            key,
            term: t(HEALTH_DIMENSION_LABEL[key]),
            value: t(HEALTH_RATING_LABEL[dimension.rating]),
            note: dimension.reason,
          },
        ]
      : [],
  );
  if (!overall) {
    return (
      <StatCard
        label={t("co.strip.healthSummary")}
        value={t(withheld ? WITHHELD_READING : UNASSESSED_READING)}
        detail={
          withheld
            ? undefined
            : t("co.strip.healthSummary.of", {
                rated: formatNumber(rated, locale),
              })
        }
      />
    );
  }
  // The denominator is always said, not only when something is failing:
  // "3 of 3 rated" and "1 of 3 rated" are different claims about how much is
  // known, and a figure that only speaks up when things are bad would let a
  // thin reading pass for a complete one.
  const failing = dimensions.filter(
    (entry) => entry.dimension?.rating === "at_risk",
  ).length;
  return (
    <StatCard
      label={t("co.strip.healthSummary")}
      value={t(HEALTH_RATING_LABEL[overall])}
      tone={overall === "at_risk" ? "warn" : undefined}
      dot={overall === "at_risk"}
      // One segment per dimension, filled for the ones that are not at risk.
      // The denominator is what was RATED, not the three that exist: a bar out
      // of three on an account with one rating would draw two empty segments
      // that read as two failures.
      meter={{ filled: rated - failing, total: rated }}
      // How much is failing, and WHY. The count alone made a reader open the
      // receipt to learn the one thing the card exists to tell them; the
      // server already writes a sentence per dimension, and the worst one is
      // the answer to "why at risk".
      detail={
        <>
          {/* The count line is dropped where it would only restate the verdict:
              on the one rated dimension that IS the verdict, "1 of 1 at risk"
              says what the word above it already said. Every other shape keeps
              it — "1 of 3 at risk" and "3 of 3 at risk" are different accounts,
              and "1 of 3 rated" is a thin reading saying so. */}
          {!(failing === 1 && rated === 1) && (
            <span>
              {failing > 0
                ? t("co.strip.healthSummary.failingOf", {
                    failing: formatNumber(failing, locale),
                    rated: formatNumber(rated, locale),
                  })
                : t("co.strip.healthSummary.of", {
                    rated: formatNumber(rated, locale),
                  })}
            </span>
          )}
          {worstReason(dimensions, t)}
        </>
      }
      basisLabel={
        basisFacts.length > 0 ? t("co.strip.basis.health") : undefined
      }
      basis={
        basisFacts.length > 0 ? <FactList facts={basisFacts} /> : undefined
      }
    />
  );
}

// What performing a suggestion means. The server names it; this maps the name
// to the words on the button.
export type SuggestionAction = NonNullable<Suggestion["action"]>;

const SUGGESTION_ACTION_LABELS: Record<SuggestionAction["kind"], MessageKey> = {
  draft_reply: "co.suggest.act.draftReply",
  open_deal: "co.suggest.act.openDeal",
  add_task: "co.suggest.act.addTask",
};

// SuggestionActionButton exists so the action is narrowed ONCE, at the call
// site, rather than re-narrowed inside a callback where TypeScript has already
// lost it.

/**
 * The names this page already holds, for citations the server could not name.
 *
 * The writer names a record when it had the name at hand and leaves it out
 * otherwise; nothing invents one. But an account's own 360 is HOLDING its
 * people and its deals, and printing "contact" beside a reason while the
 * roster three sections down says the person's name is the page failing to
 * read itself. Only records this view actually carries — anything else answers
 * undefined and falls back to the kind.
 */
export function recordNamesIn(view?: Organization360) {
  const names = new Map<string, string>();
  for (const person of view?.people?.data ?? []) {
    names.set(`person:${person.person_id}`, person.full_name);
  }
  for (const deal of view?.deals?.data ?? []) {
    names.set(`deal:${deal.deal_id}`, deal.name);
  }
  const org = view?.organization;
  if (org) {
    names.set(`organization:${org.id}`, org.display_name);
  }
  return (entityType: string, entityId: string) =>
    names.get(`${entityType}:${entityId}`);
}

function SuggestionActionButton({
  action,
  onPerform,
}: Readonly<{
  action: SuggestionAction;
  onPerform: (action: SuggestionAction) => void;
}>) {
  const t = useT();
  // Only the draft is the agent's own work. Opening a deal and adding a task
  // are things the reader does, and painting them indigo would spend the one
  // mark that means "a machine wrote this" on two clicks where nothing did.
  const byMargince = action.kind === "draft_reply";
  return (
    <Button
      small
      variant={byMargince ? "ai" : "primary"}
      onClick={() => onPerform(action)}
    >
      {byMargince && <Sparkles aria-hidden="true" />}
      {t(SUGGESTION_ACTION_LABELS[action.kind])}
    </Button>
  );
}

// nextCommitmentLine is the daily brief's own footer reading: what is owed
// and how soon. It is not a suggestion — nobody proposed it, the open tasks
// section simply has one — so it sits in the footer rather than as a row.
// Exported so the brief (companytoday.tsx) reads the same truncation-honesty
// logic rather than a second copy of it.
export function nextCommitmentLine(
  view: Organization360 | undefined,
  locale: Locale,
  t: ReturnType<typeof useT>,
): { headline: string; overdue: boolean } | undefined {
  const steps = view?.next_steps?.data ?? [];
  const step = steps[0];
  if (!step) {
    return undefined;
  }
  // The section is a page of 25 with `has_more` beside it, so past the cap
  // the count is a claim about the PAGE. "12 overdue" on an account with 40
  // is the kind of small wrong figure a rep plans against.
  const truncated = view?.next_steps?.page?.has_more === true;
  const overdueCount = steps.filter((each) => each.overdue).length;
  const count = overdueCount > 0 ? overdueCount : steps.length;
  const key = overdueCount > 0 ? "overdue" : "open";
  return {
    headline: truncated
      ? t(`co.suggest.commitment.${key}AtLeast`, {
          count: formatNumber(count, locale),
        })
      : t(`co.suggest.commitment.${key}Count`, {
          count: formatNumber(count, locale),
        }),
    overdue: overdueCount > 0,
  };
}

// useSuggestionsBody is the advice section's data and rows, split out of the
// Panel that used to own it: the daily brief now carries this chrome, so the
// dismiss mutation and the "move" rows live here where both that panel and
// the standalone `SuggestionsSection` (still used on its own in tests) can
// reach them without a second, drifting copy. Exported so companytoday.tsx
// composes the same rows rather than reimplementing them.
export function useSuggestionsBody({
  orgId,
  view,
  onOpenRecord,
  onPerform,
}: Readonly<{
  orgId: string;
  view?: Organization360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Performing the advice is the page's job, not this section's: the
  // composer, the deal and the task form all live above it.
  onPerform?: (action: SuggestionAction) => void;
}>): {
  // Whether the section has rows worth showing. A withheld, empty or
  // unavailable suggestion block carries none — advice is additive, and
  // "no advice" or "we cannot advise you" are not things a rep acts on.
  ready: boolean;
  rows: ReactNode;
  // How many rows `rows` draws. A caller that wants to count them beside its
  // own title cannot count a ReactNode, and a caller that recomputed the
  // number from the same view would be a second answer free to disagree with
  // the one on screen.
  count: number;
  // The truncation count and a failed dismissal, additive on top of whatever
  // else the caller's own footer carries.
  footer?: ReactNode;
} {
  const { locale } = useLocale();
  const t = useT();
  const recordZone = useRecordZone();
  const client = useQueryClient();
  const dismiss = useMutation({
    mutationFn: async (fingerprint: string) => {
      const { error } = await api.POST(
        "/organizations/{id}/suggestions/dismiss",
        { params: { path: { id: orgId } }, body: { fingerprint } },
      );
      if (error) {
        throwProblem(error);
      }
    },
    // The 360 is the only thing that knows which suggestions survive, so the
    // row goes when the re-read says it does. Hiding it locally on click would
    // hide it even when the dismissal never reached the server.
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["organization360", orgId] }),
  });

  const suggestions: Suggestion[] = view?.suggestions ?? [];
  const nameOf = recordNamesIn(view);
  const dropped = view?.suggestions_dropped;
  const state = sectionState(
    view,
    "suggestions",
    Boolean(view?.suggestions),
    suggestions.length,
  );
  if (state !== "ready") {
    return { ready: false, rows: null, count: 0 };
  }
  const footer =
    (dropped !== undefined && dropped > 0) || dismiss.isError ? (
      <>
        {/* A truncated list with no count reads as "that is everything".
            Absent means the section was never computed, which this card
            does not render at all. */}
        {dropped !== undefined && dropped > 0 && (
          <p className="co-row-meta">
            {t("co.suggest.more", { count: formatNumber(dropped, locale) })}
          </p>
        )}
        {/* The row staying put with no word reads as a click that missed,
            and the rep clicks again. */}
        {dismiss.isError && (
          <p className="surfacestate-withheld">
            {t("co.suggest.dismissFailed")}
            {` ${problemMessageOf(dismiss.error, t)}`}
          </p>
        )}
      </>
    ) : undefined;
  const rows = suggestions.map((suggestion) => (
    <PanelRow key={suggestion.fingerprint} className="co-move">
      <span className="co-move-body">
        {/* Who found this, and when. The mark says a machine did — the same
            indigo the card's own band carries, because it is the same claim
            about authorship rather than a second kind of importance. */}
        <span className="co-move-by">
          <Sparkles aria-hidden="true" className="co-move-spark" />
          {t("co.suggest.found")}
          {suggestion.due_at && (
            <span className="t-mono co-move-when">
              {formatDate(suggestion.due_at, locale, recordZone)}
            </span>
          )}
        </span>
        {/* The ASK, at the row's loudest weight: what the rule wants done.
            Falls back to the kind only when the rule named no title of
            its own. */}
        <span className="co-move-ask">
          {suggestion.title ?? t(`co.suggest.kind.${suggestion.kind}`)}
        </span>
        {/* The WHY, under the ask and quieter: the reason is the suggestion,
            and the rest of the row is chrome around it.

            It is also the handle on what the advice rests on. The records the
            rule fired on used to sit beside it as a row of chips, which put
            the working on screen for every reader whether or not they were
            questioning the advice; behind the reason they are one glance away
            for the reader who is, and out of the way of the one who is not. */}
        <Popover className="co-move-why" onHover label={suggestion.reason}>
          <span className="co-move-basis-head t-eyebrow">
            {t("co.suggest.basedOn")}
          </span>
          <Citations
            evidence={suggestion.evidence}
            nameOf={nameOf}
            onOpenRecord={onOpenRecord}
          />
        </Popover>
        <span className="co-move-do">
          <span className="co-move-actions">
            {/* What performing the advice means, named by the server. A
                rule that could not name one carries null and this
                renders nothing rather than a control that does nothing. */}
            {suggestion.action && onPerform && (
              <SuggestionActionButton
                action={suggestion.action}
                onPerform={onPerform}
              />
            )}
            {/* Putting this off is not the row's verb and must not look like
                one: bordered beside the action it would offer a reader two
                equal choices, when only one of them advances the account. */}
            <Button
              small
              className="co-move-defer"
              onClick={() => dismiss.mutate(suggestion.fingerprint)}
              // Only the row in flight is disabled: one dismissal must not
              // freeze the rep's other choices.
              disabled={
                dismiss.isPending &&
                dismiss.variables === suggestion.fingerprint
              }
            >
              {t("co.suggest.dismiss")}
            </Button>
          </span>
        </span>
      </span>
    </PanelRow>
  ));
  return { ready: true, rows, count: suggestions.length, footer };
}

/**
 * SuggestionsSection is the advice rows on their own, in their own Panel —
 * used standalone where nothing else on the page carries this chrome (the
 * stories file, and the suites that exercise the rows without the rest of
 * the daily brief). The live record page mounts the merged brief instead
 * (`TodayOnThisAccount`, companytoday.tsx), which composes the same rows
 * body via `useSuggestionsBody` alongside its own context band.
 */
export function SuggestionsSection({
  orgId,
  view,
  onOpenRecord,
  onPerform,
  onOpenTasks,
}: Readonly<{
  orgId: string;
  view?: Organization360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onPerform?: (action: SuggestionAction) => void;
  // Where the footer's commitment reading leads. Absent for a caller with no
  // Tasks tab of its own (the stories file).
  onOpenTasks?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const body = useSuggestionsBody({ orgId, view, onOpenRecord, onPerform });
  if (!body.ready) {
    return null;
  }
  const commitment = nextCommitmentLine(view, locale, t);
  const footer =
    commitment || onOpenTasks || body.footer ? (
      <>
        {commitment && (
          <Badge tone={commitment.overdue ? "warn" : undefined}>
            {commitment.headline}
          </Badge>
        )}
        {onOpenTasks && (
          <Button small variant="ghost" onClick={onOpenTasks}>
            {t("co.suggest.viewTasks")}
          </Button>
        )}
        {body.footer}
      </>
    ) : undefined;
  return (
    <Panel
      title={t("co.suggest.title")}
      footer={footer}
      tone="accent"
      className="co-lead"
    >
      {body.rows}
    </Panel>
  );
}
