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
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { PageAside, PageAsideToggle } from "../app/pageaside";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button, OverflowMenu } from "../design-system/atoms";
import { RecordView } from "../design-system/composed";
import { IconAction } from "../design-system/iconaction";
import { OffsiteLink } from "../design-system/offsitelink";
import { liveProjects } from "../design-system/projectpicker";
import { RecordTabs } from "../design-system/recordtabs";
import { linkedinUrl } from "../format/weburl";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { ConsentSection } from "./consent";
import { LogActivityAction } from "./logactivity";
import {
  hasCommercial,
  hasCommitments,
  hasMatters,
  PersonBriefCard,
  PersonCommercialCard,
  PersonCommitmentsCard,
  PersonMattersCard,
} from "./personcards";
import { EnrichedFields } from "./personcorrections";
import {
  PersonComposer,
  PersonMeetingBrief,
  PersonResearchDrawer,
} from "./persondrawers";
import { PersonFilesTab } from "./personfiles";
import { PersonMemory } from "./personmemory";
import { PersonNetworkTab } from "./personnetwork";
import { PersonRail } from "./personrail";
import { PersonResearchTab } from "./personresearch";
import { PersonStrip } from "./personstrip";
import { PERSON_TABS, type PersonTab, personTabRoute } from "./persontab";
import {
  PersonDealsTab,
  PersonMeetingsTab,
  PersonTimelineTab,
} from "./persontabs";
import { PersonToday } from "./persontoday";
import type { Transport } from "./persontransports";
import { primaryTransportAction, useTransports } from "./persontransports";
import "./person360.css";

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
  tab,
  personId,
  view,
  onBriefMeeting,
}: Readonly<{
  tab: PersonTab;
  personId: string;
  view: Person360;
  onBriefMeeting: (activityId: string) => void;
}>) {
  switch (tab) {
    case "timeline":
      return (
        <PersonTimelineTab
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

export function PersonPageV2({
  id,
  tab,
}: Readonly<{ id: string; tab: PersonTab }>) {
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
  // What the action that opened the composer wanted written. A rung knows WHY
  // it fired and says so in its prefill; before this the client dropped that
  // on the floor and opened the same empty composer as the generic button, so
  // "Draft a follow-up" and "Write an email" did exactly the same thing.
  const [composerIntent, setComposerIntent] = useState("");

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
  // The strip's consent slot asks a narrower, louder question than the hero
  // button above: not "may I write at all" but "what did the guard decide
  // about email", and a refusal is the one verdict that must read as loud as
  // a grant. Blocked only when every email entry says so; undefined when
  // there is no email entry to judge, which is not the same as a refusal.
  const emailEntries = (guard.data?.entries ?? []).filter(
    (entry) => entry.channel === "email",
  );
  const consentVerdict = emailAllowed
    ? "allowed"
    : emailEntries.length > 0
      ? "blocked"
      : undefined;

  // The action loop. Every surface the contract can name routes here.
  //
  // A destination this client cannot route opens NOTHING rather than guessing:
  // the typed descriptor exists so a button whose path does not exist is never
  // rendered, and silently doing something else would be worse than the 404 it
  // was meant to prevent.
  const runAction = (action: PersonMomentAction) => {
    const destination = action.destination;
    if (!destination) {
      // An action with no destination is its own destination — the composer is
      // the sensible home for the drafting kinds.
      if (action.kind === "draft_reply") {
        openComposer("");
      }
      return;
    }
    switch (destination.surface) {
      case "composer":
        openComposer(composerIntentOf(destination.prefill, t));
        return;
      case "research":
        setDrawer("research");
        return;
      case "meeting_brief":
        // The prep moment is raised off the next meeting, so that is the one
        // it briefs. An action naming its own activity is honoured over the
        // page's, because a moment about a different meeting must not open a
        // brief about this one.
        setDrawer(
          meetingDrawer(
            destination.entity_id ?? view.data.next_meeting?.activity_id,
          ),
        );
        return;
      case "record":
        if (destination.entity_id) {
          navigate({ screen: "deals", id: destination.entity_id });
        }
        return;
      case "activity_log":
        setDrawer("activity_log");
        return;
      default:
        // `task` has no surface on this page yet. Doing nothing is the honest
        // outcome; inventing a navigation would take the reader somewhere they
        // did not ask to go.
        return;
    }
  };

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
              onWrite={() => openComposer("")}
              onResearch={() => setDrawer("research")}
              onLogActivity={() => setDrawer("activity_log")}
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
            <PersonStrip view={view.data} consentVerdict={consentVerdict} />
            {view.data.moment && (
              <PersonToday
                moment={view.data.moment}
                firstName={firstName}
                onAction={runAction}
              />
            )}
            <PersonBriefCard
              brief={brief.data}
              loading={brief.isLoading}
              view={view.data}
            />
            {hasCommercial(view.data) && (
              <PersonCommercialCard view={view.data} />
            )}
            {hasCommitments(view.data) && (
              <PersonCommitmentsCard view={view.data} firstName={firstName} />
            )}
            {hasMatters(view.data) && (
              <PersonMattersCard view={view.data} firstName={firstName} />
            )}
            <PersonMemory view={view.data} />
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
          onBriefMeeting={(activityId) => setDrawer(meetingDrawer(activityId))}
        />
        <PersonComposer
          personId={id}
          view={view.data}
          guard={guard.data}
          open={drawer === "composer"}
          intent={composerIntent}
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
          activityId={briefingMeeting(drawer)}
          open={briefingMeeting(drawer) !== null}
          onClose={() => setDrawer(null)}
          projects={liveProjects(view.data.projects)}
        />
        {/* Mounted only while open, so the form starts fresh each time and the
            day it offers is today's, not the day the drawer was first built. */}
        {drawer === "activity_log" && (
          <LogActivityAction
            entityType="person"
            entityId={id}
            openOnMount
            onClose={() => setDrawer(null)}
          />
        )}
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
  | "research"
  | "activity_log"
  | { kind: "meeting"; activityId: string }
  | null;

// The brief the prep moment opens is the one for the next meeting; a timeline
// row names its own. Both go through here so the drawer has one shape.
function meetingDrawer(activityId: string | null | undefined): Drawer {
  return activityId ? { kind: "meeting", activityId } : null;
}

// Which meeting the open drawer is briefing, or null when it is not the brief.
function briefingMeeting(drawer: Drawer): string | null {
  return typeof drawer === "object" && drawer?.kind === "meeting"
    ? drawer.activityId
    : null;
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
  const person = view.person;
  const email = person.emails?.[0]?.email;
  const phone = person.phones?.[0]?.phone;
  const role = view.commercial?.role;
  return (
    <div className="pe-identity-meta">
      <div className="pe-meta-line">
        {email && (
          <span className="pe-meta-fact">
            <Mail size={13} aria-hidden="true" />
            {email}
          </span>
        )}
        {phone && (
          <span className="pe-meta-fact">
            <Phone size={13} aria-hidden="true" />
            {phone}
          </span>
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
            {t("person.page.buyingRole")}: {role.replace(/_/g, " ")}
          </span>
        )}
        <span className="pe-meta-fact pe-meta-quiet">
          {t("person.page.owner")}:{" "}
          {view.person.owner_id
            ? t("person.page.ownerAssigned")
            : t("person.page.ownerUnassigned")}
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

// The primary actions, in the concept's order (§5.2). Writing leads and is the
// only green one: a page with two primary actions has none.
function PersonActions({
  view,
  consentAllows,
  consentKnown,
  personId,
  onWrite,
  onResearch,
  onLogActivity,
}: Readonly<{
  view: Person360;
  consentAllows: boolean;
  consentKnown: boolean;
  personId: string;
  onWrite: () => void;
  onResearch: () => void;
  onLogActivity: () => void;
}>): ReactNode {
  const t = useT();
  // useCanWrite, not useCan: the form issues a POST, and a read seat is
  // refused before RBAC is consulted. The button stays on the page and says
  // why it will not press, so a reader can tell "not mine to do" from "this
  // build has no such button".
  const canLog = useCanWrite("activity", "create");
  // The transports the composer would offer, read here so the button NAMES
  // what pressing it does. The same reachability the drawer resolves: a label
  // computed from anything else is a promise the composer then breaks.
  const transports = useTransports(view);
  const write = primaryTransportAction(transports, t);
  const WriteIcon = write.icon;
  const refusal = writeRefusal({ transports, consentAllows, consentKnown }, t);
  return (
    <>
      {/* The lead verb, and the only green one: a page with two primary
          actions has none. It says which transport it will open when there is
          exactly one, stays neutral when the composer will ask, and explains
          itself rather than merely dimming when it may not be pressed. */}
      <Button
        variant="primary"
        disabled={!consentKnown}
        reason={refusal}
        onClick={onWrite}
      >
        <WriteIcon size={15} aria-hidden="true" /> {write.label}
      </Button>
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
      {/* Keeps its words. A tick box is the glyph for COMPLETING a task, so
          squaring this one would name the opposite of what it does. */}
      <Button onClick={() => navigate({ screen: "worklist" })}>
        <CheckSquare size={15} aria-hidden="true" />{" "}
        {t("person.action.addTask")}
      </Button>
      {/* A CRM a rep cannot write a meeting into is a CRM that only reads.
          This is the standing way in; the moment card offers the same form
          when its rung decides logging is the thing to do next. */}
      <Button
        disabled={!canLog}
        reason={canLog ? undefined : t("person.action.logRefused")}
        onClick={onLogActivity}
      >
        <FileText size={15} aria-hidden="true" /> {t("log.title")}
      </Button>
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
