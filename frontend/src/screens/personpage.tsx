import { useQuery } from "@tanstack/react-query";
import {
  CalendarDays,
  CheckSquare,
  FileText,
  Link as LinkIcon,
  Mail,
  MapPin,
  Phone,
  Search,
} from "lucide-react";
import type { ReactNode } from "react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { PageAside, PageAsideToggle } from "../app/pageaside";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { useUrlParams } from "../app/urlstate";
import { Button, OverflowMenu } from "../design-system/atoms";
import { RecordView } from "../design-system/composed";
import { ContactLink } from "../design-system/contactlink";
import { EmailDetail } from "../design-system/emaildetail";
import { IconAction } from "../design-system/iconaction";
import { OffsiteLink } from "../design-system/offsitelink";
import { liveProjects } from "../design-system/projectpicker";
import { RecordTabs } from "../design-system/recordtabs";
import { formatDateTime } from "../format/format";
import { linkedinUrl } from "../format/weburl";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem, useMe, useSorMode } from "./common";
import { ComposeModal } from "./compose";
import { ConsentSection } from "./consent";
import { rosterOwnerName, useRoster, useRosterPartial } from "./entityref";
import { LogActivityAction } from "./logactivity";
import { PersonMeetingBrief } from "./meetingbrief";
import {
  hasCommercial,
  hasMatters,
  PersonBriefCard,
  PersonCommercialCard,
  PersonCommitmentsCard,
  PersonMattersCard,
} from "./personcards";
import { EnrichedFields } from "./personcorrections";
import { PersonComposer, PersonResearchDrawer } from "./persondrawers";
import { PersonFilesTab } from "./personfiles";
import { PersonMemory } from "./personmemory";
import { PersonNetworkTab } from "./personnetwork";
import { BRIEF_PARAM, COMPOSE_PARAM } from "./personpage.address";
import { PersonRail } from "./personrail";
import { PersonReadings } from "./personreadings";
import { PersonResearchTab } from "./personresearch";
import { PERSON_TABS, type PersonTab, personTabRoute } from "./persontab";
import {
  PersonDealsTab,
  PersonMeetingsTab,
  PersonTimelineTab,
} from "./persontabs";
import { PersonToday } from "./persontoday";
import type { Transport } from "./persontransports";
import { primaryTransportAction, useTransports } from "./persontransports";
import { RecordReading, RecordReadingPair } from "./record360";
import { EmailVerb, RecordEmailAside } from "./recordemail";
import "./person360.css";
import { buyingRoleLabel } from "./companypeople/summary";

// The person record page V2 (ADR-0096, concept person-record-page-v2).
//
// It opens on a REASON, not a record: the moment the server selected leads,
// the facts that change how you read it sit above the fold, and the database
// view of the person is a tab away rather than the first thing on screen.

type Person360 = components["schemas"]["Person360"];
type PersonMomentAction = components["schemas"]["PersonMomentAction"];

// The tab words every record page shares. A private set for this page is how
// the same tab came to read "Timeline" here and "History" on the account.
const TAB_LABEL_KEYS: Readonly<Record<PersonTab, MessageKey>> = {
  overview: "tab.overview",
  timeline: "tab.timeline",
  network: "tab.network",
  deals: "tab.deals",
  meetings: "tab.meetings",
  research: "tab.research",
  documents: "tab.documents",
};

// The intent phrases a moment action can ask the composer to open with. The
// server names the reason in its own vocabulary ("agenda", "follow_up"); the
// composer hands what it holds to a language model and shows it to a rep, so
// both need a sentence rather than an enum value.
const COMPOSER_INTENT_KEYS: Readonly<Record<string, MessageKey>> = {
  agenda: "person.composer.intentAgenda",
  reply: "person.composer.intentReply",
  deliver_commitment: "person.composer.intentCommitment",
  follow_up: "person.composer.intentFollowUp",
};

// What the composer opens with, from an action's prefill.
//
// An intent this client does not know opens an EMPTY composer rather than
// passing the raw token through: "deliver_commitment" in the intent field is
// worse than nothing, because a rep reads it as something the product meant to
// say, and the model reads it as an instruction nobody wrote.
function composerIntentOf(
  prefill: Readonly<Record<string, string>> | undefined,
  t: ReturnType<typeof useT>,
): string {
  const key = COMPOSER_INTENT_KEYS[prefill?.intent ?? ""];
  if (!key) {
    return "";
  }
  // The promise itself rides in `subject`. Without it, a rung firing on one of
  // several open commitments asks the composer to deliver "what we promised"
  // and leaves the drafter to guess which.
  const subject = prefill?.subject?.trim();
  return subject ? `${t(key)}: ${subject}` : t(key);
}

/**
 * ProfileLink is the header's LinkedIn fact: a link when the recorded address
 * really is LinkedIn, the word alone when it is not.
 *
 * The label is the fixed word rather than the address, which makes it a CLAIM
 * about the destination — and `social` is an open map a crawl or a connector
 * can write. An arbitrary host under that word is a phishing link wearing the
 * product's own chrome, so the host is checked before the anchor is drawn. The
 * fact is kept either way: that this contact has a profile recorded is true
 * whatever the value turns out to be.
 */
function ProfileLink({ href }: Readonly<{ href: string }>) {
  const t = useT();
  const label = t("person.page.linkedin");
  if (!linkedinUrl(href)) {
    return label;
  }
  return (
    <OffsiteLink href={href} className="pe-meta-link">
      {label}
    </OffsiteLink>
  );
}

/**
 * PersonTabPanel is what the tab bar chose, and nothing else: the page above
 * it decides which record it is on, and this decides which face of that record
 * the reader asked for.
 *
 * Four of them read the 360 the page already holds. Documents owns its read
 * because the 360 carries no attachments, and the Timeline's CHANGES half
 * owns one for the same reason — its ACTIVITIES half is the 360's own section.
 */
function PersonTabPanel({
  onOpenEmail,
  tab,
  personId,
  view,
  onBriefMeeting,
}: Readonly<{
  tab: PersonTab;
  personId: string;
  view: Person360;
  onBriefMeeting: (activityId: string) => void;
  /** Opens one message in the record's drawer, which the page owns. */
  onOpenEmail?: (activityId: string) => void;
}>) {
  switch (tab) {
    case "timeline":
      return (
        <PersonTimelineTab
          onOpenEmail={onOpenEmail}
          personId={personId}
          view={view}
          onBriefMeeting={onBriefMeeting}
        />
      );
    case "network":
      return <PersonNetworkTab personId={personId} view={view} />;
    case "deals":
      return <PersonDealsTab view={view} />;
    case "meetings":
      return <PersonMeetingsTab view={view} onBriefMeeting={onBriefMeeting} />;
    case "research":
      return <PersonResearchTab view={view} />;
    case "documents":
      return <PersonFilesTab personId={personId} />;
    // Overview is drawn by the page itself, above this component: its stack
    // reads the page's other queries (the brief) and its moment drives the
    // page's own action loop.
    default:
      return null;
  }
}

// What a moment card's action does, off the surface the server named. A
// standalone function rather than a closure inside PersonPageV2: the switch's
// branches are this function's cognitive weight, not the page's.
//
// A destination this client cannot route opens NOTHING rather than guessing:
// the typed descriptor exists so a button whose path does not exist is never
// rendered, and silently doing something else would be worse than the 404 it
// was meant to prevent.
function runPersonMomentAction(
  action: PersonMomentAction,
  t: ReturnType<typeof useT>,
  handlers: Readonly<{
    openComposer: (intent: string) => void;
    setDrawer: (drawer: Drawer) => void;
    openBrief: (activityId: string | null) => void;
    // The prep moment is raised off the next meeting, so that is the one it
    // briefs by default. An action naming its own activity is honoured over
    // it, because a moment about a different meeting must not open a brief
    // about this one.
    nextMeetingId: string | null;
  }>,
): void {
  const destination = action.destination;
  if (!destination) {
    // An action with no destination is its own destination — the composer is
    // the sensible home for the drafting kinds.
    if (action.kind === "draft_reply") {
      handlers.openComposer("");
    }
    return;
  }
  switch (destination.surface) {
    case "composer":
      handlers.openComposer(composerIntentOf(destination.prefill, t));
      return;
    case "research":
      handlers.setDrawer("research");
      return;
    case "meeting_brief":
      handlers.openBrief(destination.entity_id ?? handlers.nextMeetingId);
      return;
    case "record":
      if (destination.entity_id) {
        navigate({ screen: "deals", id: destination.entity_id });
      }
      return;
    case "activity_log":
      handlers.setDrawer("activity_log");
      return;
    case "task":
      handlers.setDrawer("activity_task");
      return;
    default:
      // Every surface PersonMomentDestination documents today is handled
      // above, so the compiler holds this exhaustive: a surface the contract
      // adds and this switch does not is a type error here, not a live
      // button that silently does nothing until someone notices.
      destination.surface satisfies never;
      return;
  }
}

export function PersonPageV2({
  id,
  tab,
}: Readonly<{ id: string; tab: PersonTab }>) {
  // Which message the drawer is showing. One drawer over the record, owned by
  // the page: the timeline and the rail both open into it, so a citation in
  // the aside and a row in the body lead to the same place.
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const { locale } = useLocale();
  const t = useT();
  const recordZone = useRecordZone();
  const view = useQuery({
    queryKey: ["person360", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}/360", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const brief = useQuery({
    queryKey: ["personBrief", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}/brief", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const guard = useQuery({
    queryKey: ["personConsentGuard", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}/consent/guard", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  // Which drawer is open, if any. One at a time: two surfaces over the same
  // record would each claim to be the thing the reader is doing.
  const [drawer, setDrawer] = useState<Drawer>(null);
  const [briefedMeeting, openBrief] = useBriefedMeeting();
  // What the action that opened the composer wanted written. A rung knows WHY
  // it fired and says so in its prefill; before this the client dropped that
  // on the floor and opened the same empty composer as the generic button, so
  // "Draft a follow-up" and "Write an email" did exactly the same thing.
  const [composerIntent, setComposerIntent] = useState("");
  // BOTH doors to the composer, answered in one place: the address opens it on
  // arrival, pressing "Write an email" opens it after. The page is at its
  // complexity ceiling — the lint says so — so the question is settled in the
  // hook rather than in two conditionals in the markup.
  const composer = useComposer(t, drawer === "composer", composerIntent, () =>
    setDrawer(null),
  );

  // Every path to the composer goes through here, and the intent it opens with
  // is decided ONCE, on the way in. Two rules the two callers each get wrong on
  // their own: the generic "Write an email" must clear the reason the last rung
  // left behind, or it inherits a subject nobody asked for; and a composer
  // already on screen keeps what it holds, because a reader mid-sentence is not
  // asking to have their words replaced.
  const openComposer = (intent: string) => {
    if (drawer !== "composer") {
      setComposerIntent(intent);
    }
    setDrawer("composer");
  };

  // Read before the loading returns: a hook below an early return renders a
  // different hook count per state, which React rejects.
  const overlay = useSorMode() === "overlay";

  if (view.isLoading) {
    return <div className="wrap">{t("person.page.loading")}</div>;
  }
  if (view.isError || !view.data) {
    return <div className="wrap">{t("person.page.notOpened")}</div>;
  }

  const person = view.data.person;
  const firstName = person.full_name.split(" ")[0];
  // Consent is decided per PURPOSE, so the guard carries one email entry per
  // purpose. The hero button asks a wider question than any single entry —
  // "is there anything we may write to them about" — and reading only the
  // first entry answered it with whichever purpose sorted first, disabling the
  // button on a contact the product would happily let you mail transactionally.
  // Which purpose applies is then the composer's own question.
  const emailAllowed = (guard.data?.entries ?? []).some(
    (entry) => entry.channel === "email" && entry.verdict === "allowed",
  );

  // The action loop. Every surface the contract can name routes to
  // `runPersonMomentAction`, a standalone function rather than a closure here
  // so its switch's own branches are not this component's cognitive weight.
  const runAction = (action: PersonMomentAction) =>
    runPersonMomentAction(action, t, {
      openComposer,
      setDrawer,
      openBrief,
      nextMeetingId: view.data.next_meeting?.activity_id ?? null,
    });

  return (
    <div className="wrap">
      <RecordView
        name={person.full_name}
        avatarSrc={null}
        subtitle={<PersonSubtitle view={view.data} />}
        pulse={<PersonIdentityLine view={view.data} />}
        actions={
          <>
            <PersonActions
              view={view.data}
              consentAllows={emailAllowed}
              consentKnown={guard.data !== undefined}
              personId={id}
              overlay={overlay}
              onWrite={() => openComposer("")}
              onWriteMail={() => setDrawer("mail")}
              onResearch={() => setDrawer("research")}
              onLogActivity={() => setDrawer("activity_log")}
              onAddTask={() => setDrawer("activity_task")}
            />
            <PageAsideToggle />
          </>
        }
        actionsInline
        zone={recordZone}
        tabs={
          <RecordTabs
            options={PERSON_TABS}
            value={tab}
            onChange={(next) => navigate(personTabRoute(id, next))}
            labels={{
              overview: t(TAB_LABEL_KEYS.overview),
              timeline: t(TAB_LABEL_KEYS.timeline),
              network: t(TAB_LABEL_KEYS.network),
              deals: t(TAB_LABEL_KEYS.deals),
              meetings: t(TAB_LABEL_KEYS.meetings),
              research: t(TAB_LABEL_KEYS.research),
              documents: t(TAB_LABEL_KEYS.documents),
            }}
            // At least one connected provider has never been asked about this
            // contact, so there is a lookup waiting behind the tab. ANY of them
            // is enough: the reader still has somebody to ask, even if another
            // provider already answered. A CANCELLED run reads as never_run
            // too, and the dot coming back is right — nothing was bought.
            marks={{
              research: (view.data.provider_profiles ?? []).some(
                (profile) => profile.state === "never_run",
              ),
            }}
          />
        }
      >
        {/* The contact's context goes to the PAGE's own column, beside the
            work rather than inside the record's grid — so it runs the full
            height past the header and the tab strip, and does not move when a
            tab changes. What is true of the PERSON does not belong to
            whichever part of them is open. Same column, same fold and same
            memory of it as every other record page. */}
        <PageAside>
          <PersonRail
            view={view.data}
            guard={guard.data}
            firstName={firstName}
            onExplain={() => navigate({ screen: "contacts", id })}
            onOpenEmail={setOpenEmail}
          />
          <PersonEmailPanel
            personId={id}
            overlay={overlay}
            archived={Boolean(person.archived_at)}
          />
        </PageAside>
        {tab === "overview" && (
          <div className="record-stack">
            {/* The readings lead the overview, under the strip that chose it —
                the same place the account page puts its own. They belong to
                THIS body rather than to the record: the Deals tab is a list of
                deals and the Documents tab a filing cabinet, and a row of
                relationship readings over either is a header for a page it is
                not describing. */}
            <PersonReadings
              view={view.data}
              onOpenTab={(next) => navigate(personTabRoute(id, next))}
            />
            {/* ONE READING, IN PARTS — the shape every record page reads in:
                the call with the thread it was read from, the day's work, and
                under them the two sections a reader consults rather than
                reads. What was said lately and what is owed are the pair,
                because the moment above is argued from exactly those two. */}
            <RecordReading>
              <PersonToday
                moment={view.data.moment}
                name={person.full_name}
                view={view.data}
                onAction={runAction}
                onOpenTasks={() => navigate({ screen: "worklist" })}
              />
              <RecordReadingPair>
                <PersonMemory view={view.data} />
                <PersonCommitmentsCard view={view.data} firstName={firstName} />
              </RecordReadingPair>
            </RecordReading>
            {/* The contact in prose, under the reading of it: the moment
                answers what to DO, this answers who they ARE to us, in
                sentences with their sources under them. */}
            <PersonBriefCard
              brief={brief.data}
              loading={brief.isLoading}
              view={view.data}
            />
            {(hasCommercial(view.data) || hasMatters(view.data)) && (
              <RecordReadingPair>
                {hasCommercial(view.data) && (
                  <PersonCommercialCard view={view.data} />
                )}
                {hasMatters(view.data) && (
                  <PersonMattersCard view={view.data} firstName={firstName} />
                )}
              </RecordReadingPair>
            )}
            {/* What this person has agreed to, and the one way to ask them
                directly. It renders on a thin record too: what you may send is
                a live fact whether or not anyone has written to them yet. */}
            <ConsentSection personId={id} />
            {/* The fields Margince read off a signature or a card, and the
                one place a reader can confirm or correct them. */}
            <EnrichedFields personId={id} view={view.data} />
          </div>
        )}

        <PersonTabPanel
          tab={tab}
          personId={id}
          view={view.data}
          onBriefMeeting={openBrief}
          onOpenEmail={setOpenEmail}
        />
        <PersonComposer
          personId={id}
          view={view.data}
          guard={guard.data}
          open={composer.open}
          intent={composer.intent}
          onClose={composer.close}
        />
        {/* One drawer over the record. The timeline's rows and the rail's
            citations both open into it, so a reader who finds a message in the
            aside and one who finds it in the body land in the same place. */}
        {openEmail && (
          <EmailDetail
            activityId={openEmail}
            onClose={() => setOpenEmail(null)}
            formatWhen={(iso) => formatDateTime(iso, locale, recordZone)}
          />
        )}
        <PersonMailDrawer
          personId={id}
          open={drawer === "mail"}
          onClose={() => setDrawer(null)}
        />
        <PersonResearchDrawer
          personId={id}
          personName={person.full_name}
          providerProfiles={view.data.provider_profiles}
          open={drawer === "research"}
          onClose={() => setDrawer(null)}
        />
        <PersonMeetingBrief
          activityId={briefedMeeting}
          open={briefedMeeting !== null}
          onClose={() => openBrief(null)}
          projects={liveProjects(view.data.projects)}
        />
        <PersonActivityDrawer
          personId={id}
          drawer={drawer}
          onClose={() => setDrawer(null)}
        />
      </RecordView>
    </div>
  );
}

// Which drawer is open. Null is the ordinary state — the page is the thing the
// reader is looking at, and a drawer is a detour from it.
//
// The meeting brief names the meeting it is briefing. It used to be a bare
// "meeting" that always read `next_meeting`, which made a working feature
// reachable only for the soonest meeting and only while the prep moment was
// live — every other meeting on the record had a brief the backend would
// happily assemble and no way to ask for it.
type Drawer =
  | "composer"
  | "mail"
  | "research"
  | "activity_log"
  | "activity_task"
  | null;

/**
 * Which meeting the address says to brief, and how to change it.
 *
 * The meeting brief is the one drawer on this page with an ADDRESS, because it
 * is the one another screen sends a reader to. The other three are opened by
 * pressing something here, and a thing you can only open by pressing it needs
 * no name.
 *
 * DERIVED from the query rather than seeded from it, and the difference shows
 * on Back: a drawer held in useState stays open when the reader navigates back
 * out of it, because nothing tells it the address changed. Deriving makes Back
 * close it, which is what a reader pressing Back means.
 *
 * A hook rather than four lines in the page because the page is at its
 * complexity ceiling — the lint says so — and this is one idea a reader should
 * be able to take in on its own.
 */
/**
 * Whether the composer is open, what it is about, and how to close it.
 *
 * TWO DOORS, one answer. A reader presses "Write an email" on this page, or
 * arrives on `?compose=reply` from another screen — the worklist's
 * `draft_reply` move. Both end here, so neither caller has to know about the
 * other.
 *
 * The address half is DERIVED, not seeded, the same reason the brief drawer is:
 * a drawer held in state stays open when the reader navigates back out of it,
 * and deriving makes Back close it, which is what pressing Back means.
 *
 * An intent this client does not know opens NOTHING rather than an empty
 * composer: an address is something a person can type, and a stray
 * `?compose=x` silently starting an email is worse than a link that does not
 * work.
 */
function useComposer(
  t: ReturnType<typeof useT>,
  pressedOpen: boolean,
  pressedIntent: string,
  onPressedClose: () => void,
): Readonly<{ open: boolean; intent: string; close: () => void }> {
  const [params, setParams] = useUrlParams();
  const asked = params.get(COMPOSE_PARAM);
  const key = asked ? COMPOSER_INTENT_KEYS[asked] : undefined;
  const close = () => {
    onPressedClose();
    // The address goes with it, or Back would re-open what the reader just
    // closed and the link could never be dismissed.
    if (asked) {
      const out = new Map(params);
      out.delete(COMPOSE_PARAM);
      setParams(out);
    }
  };
  return {
    open: pressedOpen || key !== undefined,
    intent: key ? t(key) : pressedIntent,
    close,
  };
}

function useBriefedMeeting(): [
  string | null,
  (activityId: string | null) => void,
] {
  const [params, setParams] = useUrlParams();
  const setBriefed = (activityId: string | null) => {
    const out = new Map(params);
    if (activityId) {
      out.set(BRIEF_PARAM, activityId);
    } else {
      out.delete(BRIEF_PARAM);
    }
    setParams(out);
  };
  return [params.get(BRIEF_PARAM) ?? null, setBriefed];
}

// The header's second line: what this person does, and where. The company is a
// link because it is a record of its own, not a label.
function PersonSubtitle({ view }: Readonly<{ view: Person360 }>): ReactNode {
  const person = view.person;
  const employment = view.employments?.data?.[0];
  return (
    <div>
      {person.title}
      {employment?.organization_name && (
        <>
          {person.title ? " · " : ""}
          <button
            type="button"
            className="pe-meta-link"
            onClick={() =>
              navigate({
                screen: "companies",
                id: employment.organization_id,
              })
            }
          >
            {employment.organization_name}
          </button>
        </>
      )}
    </div>
  );
}

// The identity line under the name: how to reach them, and who holds the
// relationship. ONE wrapping line rather than two — a reader takes the whole
// line in at once, and splitting it made the header three deep for facts that
// are each a few words long. Standing is quieter than a contact method within
// that line: it qualifies the record rather than being a way to act on it.
function PersonIdentityLine({
  view,
}: Readonly<{ view: Person360 }>): ReactNode {
  const t = useT();
  // The owner off the roster's first page, as the deal's facts read theirs.
  const roster = useRoster("user", Boolean(view.person.owner_id));
  const rosterPartial = useRosterPartial("user", Boolean(view.person.owner_id));
  const person = view.person;
  const email = person.emails?.[0]?.email;
  const phone = person.phones?.[0]?.phone;
  const role = view.commercial?.role;
  return (
    <div className="pe-identity-meta">
      <div className="pe-meta-line">
        {/* The address and the number are LINKS: a reader who sees an address
            expects to click it, and a header that showed one and did nothing
            taught them the record was a printout. The link hands the value to
            their own client; the Write verb above stays the way to write on
            the product's behalf, behind its consent gate. */}
        {email && (
          <ContactLink
            kind="email"
            value={email}
            className="pe-meta-link"
            textClassName="pe-meta-fact"
          >
            <Mail size={13} aria-hidden="true" />
            {email}
          </ContactLink>
        )}
        {phone && (
          <ContactLink
            kind="phone"
            value={phone}
            className="pe-meta-link"
            textClassName="pe-meta-fact"
          >
            <Phone size={13} aria-hidden="true" />
            {phone}
          </ContactLink>
        )}
        {person.address?.city && (
          <span className="pe-meta-fact">
            <MapPin size={13} aria-hidden="true" />
            {person.address.city}
          </span>
        )}
        {/* `social` is an open map on the wire, so its values are unknown to
            the type system. The fact renders only when there is a string to
            stand behind it — a link with nothing at the end is worse than no
            link at all.

            It IS a link, because it wears a link's icon: a reader who sees the
            chain and the word clicks it, and for a while this row answered that
            click with nothing. OffsiteLink falls back to plain text when the
            value is not a web address, so a malformed one degrades to what this
            row used to be rather than to a dead anchor. */}
        {typeof person.social?.linkedin === "string" && (
          <span className="pe-meta-fact">
            <LinkIcon size={13} aria-hidden="true" />
            <ProfileLink href={person.social.linkedin} />
          </span>
        )}
        {/* The role is what the relationship edge records — never inferred from
            a job title, which is why a person with a title can still have no
            buying role and the line simply omits it. */}
        {role && (
          <span className="pe-meta-fact pe-meta-quiet">
            {t("person.page.buyingRole")}: {buyingRoleLabel(role, t)}
          </span>
        )}
        <span className="pe-meta-fact pe-meta-quiet">
          {t("person.page.owner")}:{" "}
          {rosterOwnerName(
            view.person.owner_id,
            roster,
            rosterPartial,
            t,
            t("person.page.ownerUnassigned"),
          )}
        </span>
      </div>
    </div>
  );
}

// Why the lead verb may not be pressed, in the reader's words, or undefined
// when it may.
//
// TWO facts refuse it and they are never merged into one sentence: consent says
// we may not write to this person, reachability says there is nowhere to write
// to. A rep who is told the wrong one goes looking in the wrong record.
//
// Reachability is asked first because it is the unconditional half — with no
// transport the composer has nothing to send on whatever consent says, and a
// consent sentence there would describe a decision that is not what stops them.
// A guard that has not answered yet refuses nothing: the button is disabled
// without a reason until the verdict is in, because claiming a refusal the
// server has not made is worse than a control that is briefly quiet.
function writeRefusal(
  state: Readonly<{
    transports: readonly Transport[];
    consentAllows: boolean;
    consentKnown: boolean;
  }>,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (state.transports.length === 0) {
    return t("person.action.noTransport");
  }
  if (state.consentAllows || !state.consentKnown) {
    return undefined;
  }
  return t("person.action.consentRefused");
}

// The rail's email box, and when the page may not draw it: not in overlay — a
// mirrored workspace has no thread data, and the server refuses the
// waiting-reply read there outright — and not on an archived contact, whose
// page offers no writes.
function PersonEmailPanel({
  personId,
  overlay,
  archived,
}: Readonly<{ personId: string; overlay: boolean; archived: boolean }>) {
  if (overlay || archived) {
    return null;
  }
  return (
    <RecordEmailAside
      entityType="person"
      entityId={personId}
      personId={personId}
      detectWaitingReply
    />
  );
}

// The header's generic mail verb opens the shared compose drawer — the thread
// offers and the conversation beside the form, the same shape every record's
// mail box opens. Split out so the page renders it unconditionally, the same
// way PersonMeetingBrief carries its own `open`. Keyed by the record: navigating
// to another person while it is open remounts it rather than re-pointing it —
// without the key the text written for one contact would be filed against
// another.
function PersonMailDrawer({
  personId,
  open,
  onClose,
}: Readonly<{ personId: string; open: boolean; onClose: () => void }>) {
  if (!open) {
    return null;
  }
  return (
    <ComposeModal
      key={personId}
      entityType="person"
      entityId={personId}
      personId={personId}
      open
      onClose={onClose}
    />
  );
}

// The Log activity / Add task drawer: one form, one drawer state, started on
// a different kind depending on which verb opened it — the same shape
// companyheader.tsx's two LogActivityAction triggers already show side by
// side. Split out the way PersonMailDrawer is above, so this drawer's own
// branching stays out of PersonPageV2's cognitive weight.
function PersonActivityDrawer({
  personId,
  drawer,
  onClose,
}: Readonly<{ personId: string; drawer: Drawer; onClose: () => void }>) {
  if (drawer !== "activity_log" && drawer !== "activity_task") {
    return null;
  }
  const asTask = drawer === "activity_task";
  return (
    <LogActivityAction
      entityType="person"
      entityId={personId}
      initialKind={asTask ? "task" : undefined}
      triggerLabel={asTask ? "log.addTask" : undefined}
      openOnMount
      onClose={onClose}
    />
  );
}

// The header's verbs, in the order every record page carries them: writing
// first, then the record's other doors. None of them is filled — the move
// worth doing is the one the call names, and that one carries the colour.
function PersonActions({
  view,
  consentAllows,
  consentKnown,
  personId,
  overlay,
  onWrite,
  onWriteMail,
  onResearch,
  onLogActivity,
  onAddTask,
}: Readonly<{
  view: Person360;
  consentAllows: boolean;
  consentKnown: boolean;
  personId: string;
  // LogActivityAction itself renders nothing in overlay — a mirrored
  // workspace has no activity write of its own, the same fact
  // PersonEmailPanel already states for the record's email box — so a
  // trigger drawn here would set drawer state a mount elsewhere refuses.
  overlay: boolean;
  onWrite: () => void;
  onWriteMail: () => void;
  onResearch: () => void;
  onLogActivity: () => void;
  onAddTask: () => void;
}>): ReactNode {
  const t = useT();
  // useCanWrite, not useCan: both this verb and Add task below issue the same
  // POST, and a read seat is refused before RBAC is consulted. The buttons
  // stay on the page and say why they will not press, so a reader can tell
  // "not mine to do" from "this build has no such button".
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  // Named once and pointed at by both buttons — Button's own contract for a
  // surface where several controls are refused by ONE fact: printing the
  // sentence beside each button says it as many times as there are buttons.
  const logRefusedId = useId();
  // A guard that has not answered yet refuses nothing: claiming a refusal
  // `/me` has not decided is worse than a control that is briefly quiet —
  // the same rule writeRefusal states for the identical shape, above.
  const logGrantKnown = me.data?.authorization !== undefined;
  const logRefused = logGrantKnown && !canLog ? logRefusedId : undefined;
  const logPending = !logGrantKnown;
  // The transports the composer would offer, read here so the button NAMES
  // what pressing it does. The same reachability the drawer resolves: a label
  // computed from anything else is a promise the composer then breaks.
  const transports = useTransports(view);
  const write = primaryTransportAction(transports, t);
  const WriteIcon = write.icon;
  const refusal = writeRefusal({ transports, consentAllows, consentKnown }, t);
  // Where the verb goes. Mail as the only way in opens the shared compose
  // drawer, which knows the record's conversations; a channel in the mix keeps
  // PersonComposer, the one place that can ask which transport and answer a
  // provider-anchored conversation.
  const mailOnly = transports.length === 1 && transports[0].id === "email";
  return (
    <>
      {/* The shared Email verb, wearing the transport it will open when there
          is exactly one, neutral when the composer will ask, and explaining
          itself rather than merely dimming when it may not be pressed. */}
      <EmailVerb
        label={write.label}
        icon={<WriteIcon size={15} aria-hidden="true" />}
        disabled={!consentKnown}
        reason={refusal}
        onClick={mailOnly ? onWriteMail : onWrite}
      />
      {/* Square, because a phone and a calendar are verbs a reader already
          knows from the glyph — and five labelled buttons in a row is a header
          that reads as a toolbar, with the one action the page is FOR no more
          prominent than the rest of them. `IconAction` owes each one its name
          on hover as well as to a screen reader. */}
      <IconAction
        label={t("person.action.call")}
        icon={<Phone size={15} aria-hidden="true" />}
        onClick={() => navigate(personTabRoute(personId, "timeline"))}
      />
      <IconAction
        label={t("person.action.meetings")}
        icon={<CalendarDays size={15} aria-hidden="true" />}
        onClick={() =>
          navigate({ screen: "contacts", id: personId, id2: "meetings" })
        }
      />
      {/* Neither verb is drawn in overlay: LogActivityAction, the form both
          open, renders nothing there — a mirrored workspace has no activity
          write of its own — so a trigger here would set drawer state a mount
          elsewhere refuses to act on. */}
      {!overlay && (
        <>
          {logRefused && (
            <p className="t-caption" id={logRefusedId}>
              {t("record.logActivityRefused")}
            </p>
          )}
          {/* A CRM a rep cannot write a meeting into is a CRM that only
              reads. This is the standing way in; the moment card offers the
              same form when its rung decides logging is the thing to do
              next. */}
          <Button
            disabled={logPending}
            reasonId={logRefused}
            onClick={onLogActivity}
          >
            <FileText size={15} aria-hidden="true" /> {t("log.title")}
          </Button>
          {/* Keeps its words. A tick box is the glyph for COMPLETING a task,
              so squaring this one would name the opposite of what it does.
              Files the task against THIS record — the same form Log activity
              opens, started on its task kind, rather than a navigation to the
              Worklist, which has no way to add one. */}
          <Button
            disabled={logPending}
            reasonId={logRefused}
            onClick={onAddTask}
          >
            <CheckSquare size={15} aria-hidden="true" />{" "}
            {t("person.action.addTask")}
          </Button>
        </>
      )}
      {/* A real menu. This was a button labelled "More actions" that navigated
          to the timeline tab — the same place the Call button went — so the one
          control on the header promising there was more behind it delivered a
          tab instead, and the promise was the only thing it did. Research moves
          in here because a magnifier reads as "search" and this verb is not
          search, and the timeline gets the honest name the product already uses
          for it everywhere else. */}
      <OverflowMenu label={t("record.moreActions")}>
        <Button small onClick={onResearch}>
          <Search size={15} aria-hidden="true" /> {t("person.action.research")}
        </Button>
        <Button
          small
          onClick={() => navigate(personTabRoute(personId, "timeline"))}
        >
          {t("record.fullHistory")}
        </Button>
      </OverflowMenu>
    </>
  );
}
