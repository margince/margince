import {
  Fragment,
  type ReactNode,
  Suspense,
  useDeferredValue,
  useMemo,
} from "react";
import {
  AnalyticsScreen,
  BookingScreen,
  BriefScreen,
  BuyerRoomScreen,
  ClientSurfaceScreen,
  CompaniesScreen,
  CompanyScreen,
  ConfirmDetailsScreen,
  ContactPageV2,
  ContactsScreen,
  CustomRoute,
  DealsRoute,
  ExtensionRoute,
  FiltersScreen,
  LeadScreen,
  LeadsScreen,
  LegalReviewsRoute,
  OAuthConsent,
  OfferScreen,
  PartnersScreen,
  PreferenceCenterScreen,
  ProjectScreen,
  ProjectsScreen,
  ResetRoute,
  ScheduledSendsScreen,
  ScreenNotice,
  ScreenPending,
  SearchRoute,
  SettingsScreen,
  ShareRoute,
  safeDecode,
  TagResultScreen,
  UnsubscribeScreen,
  WorklistRedirect,
} from "./App.screens";
import { CUSTOM_SCREEN } from "./app/custom";
import { opensReservedPage } from "./app/nav";
import { UnsavedGuard } from "./components/unsaved";
import { OnboardingScreen } from "./features/onboarding/onboarding";
import { RESET_ROUTE } from "./features/sign-in/auth";
import { isContactTab } from "./lib/contacttab";
import { EXTENSION_SCREEN } from "./lib/extensions";
import {
  navigate,
  parseHash,
  routeHash,
  routeIdentity,
  type Screen,
} from "./lib/router";

// The path segments below the screen. Only two of the four are dispatched on.
// id3 belongs to the screens that read it themselves (the consent return's
// provider), and nothing routes on it.
type ScreenArgs = Readonly<{ id?: string; id2?: string }>;

// One entry per address, and `Record<Screen, …>` is what makes that set
// complete. A screen added to the union in lib/router.tsx with no view here
// fails the build. It does not render the page an unknown hash gets. A
// fallback arm cannot tell an unwired screen from an unknown address.
const SCREEN_VIEWS: Readonly<Record<Screen, (args: ScreenArgs) => ReactNode>> =
  {
    home: () => <BriefScreen />,
    // The tab rides the URL, so it survives a reload and can be linked to.
    // An unknown segment falls back to overview rather than rendering an
    // empty page: a mistyped link should land somewhere, not nowhere.
    contacts: ({ id, id2 }) =>
      !id || opensReservedPage({ screen: "contacts", id }) ? (
        <ContactsScreen importing={id !== undefined} />
      ) : (
        <ContactPageV2 id={id} tab={isContactTab(id2) ? id2 : "overview"} />
      ),
    companies: ({ id }) =>
      id ? <CompanyScreen id={id} /> : <CompaniesScreen />,
    partners: () => <PartnersScreen />,
    leads: ({ id }) => (id ? <LeadScreen id={id} /> : <LeadsScreen />),
    deals: ({ id, id2 }) => <DealsRoute id={id} id2={id2} />,
    projects: ({ id }) => (id ? <ProjectScreen id={id} /> : <ProjectsScreen />),
    // One segment, and it names whose day it is. A team board row needs that
    // door. The other three dials stay state: one of four in the address would
    // make it describe a fraction of what the reader is looking at.
    worklist: ({ id }) => <WorklistRedirect opensOn={id} />,
    analytics: () => <AnalyticsScreen />,
    // The screen resolves its own address. Which entry an address names is the
    // settings IA's question. The admin half lives a segment deeper, and a
    // legacy link to it is answered and rewritten there rather than here.
    settings: (args) => (
      <SettingsScreen route={{ screen: "settings", ...args }} />
    ),
    // The object or library section, and a saved view below it, ride the URL.
    filters: ({ id, id2 }) => <FiltersScreen id={id} view={id2} />,
    // No segments: the queue is one page, and a row carries all a message has.
    // The literal, not SCHEDULED_SCREEN: importing that would pull the module
    // into the startup graph. The key is still typed against `Screen`.
    scheduled: () => <ScheduledSendsScreen />,
    "legal-reviews": ({ id }) => <LegalReviewsRoute id={id} />,
    offers: ({ id }) =>
      id ? (
        <OfferScreen id={id} />
      ) : (
        <ScreenNotice messageKey="screen.pending" />
      ),
    search: ({ id, id2 }) => (
      <SearchRoute q={id ? safeDecode(id) : ""} openActivityId={id2} />
    ),
    tags: ({ id }) => <TagResultScreen tagID={id} />,
    lists: ({ id }) => <FiltersScreen list={id ?? ""} />,
    share: ({ id, id2 }) => <ShareRoute id={id} id2={id2} />,
    onboarding: () => <OnboardingScreen />,
    client: () => <ClientSurfaceScreen />,
    // #/book/<host_slug> is the anonymous public variant
    book: ({ id }) => <BookingScreen hostSlug={id} />,
    // #/preferences/<token>: anonymous; the token in the path is the
    // whole capability (security: [] in the contract).
    preferences: ({ id }) => <PreferenceCenterScreen token={id} />,
    // #/unsubscribe/<token>/<purpose>: the page the visible unsubscribe
    // link in a message opens. Anonymous, and it never withdraws on arrival:
    // a mail scanner following the link is not a contact pressing a button.
    unsubscribe: ({ id, id2 }) => (
      <UnsubscribeScreen token={id} purpose={id2} />
    ),
    // #/confirm/<token>: anonymous, the sibling of the preference centre. The
    // token is single-use and shows the subject their own record, which is why
    // it is a different credential from the reusable preference link.
    confirm: ({ id }) => <ConfirmDetailsScreen token={id} />,
    // #/room?c=<credential>: the Deal Room's buyer, anonymous. The credential
    // is already out of the address by the time this table is reached. The
    // router takes it as it reads the hash (lib/router.tsx). A gate above this
    // dispatch can render instead of the route, and then this screen would
    // never mount to scrub it.
    room: () => <BuyerRoomScreen />,
    // reached only via the server's redirect off GET /oauth/authorize
    // (#/oauth-consent?…&consent=<nonce>), never a rail destination.
    "oauth-consent": () => <OAuthConsent />,
    [RESET_ROUTE]: () => <ResetRoute />,
    [EXTENSION_SCREEN]: ({ id }) => <ExtensionRoute name={id} />,
    // The fork seam (app/custom.ts). Upstream's registry is empty, so in
    // vanilla this arm renders the not-found card for every address. That is
    // the right answer, so the arm does not need to be conditional.
    [CUSTOM_SCREEN]: ({ id }) => <CustomRoute name={id} />,
    // A hash naming no address this app answers. The shell's heading says the
    // same word (shell.unknownPage). The reader is told once by the page and
    // once by the column, and neither claims a page by that name exists.
    "not-found": () => <ScreenNotice messageKey="shell.unknownPage" />,
  };

// A displayed route as one string, and back. The guard below compares
// addresses for equality, so the address it holds has to be a primitive. An
// object is a new value on every render. The hash the router already speaks is
// that primitive. `routeHash`/`parseHash` are the app's own serializer and
// parser, so this adds no second spelling of an address. A segment cannot be
// mangled by the round trip, because it came from a hash in the first place.
//
// Putting the address back is a normal navigation, through the one function
// that performs them. The reader keeps their edit, and the URL returns to the
// page they are on.
const navigateToAddress = (address: string) => navigate(parseHash(address));

// The view for a held address, parsed rather than threaded alongside the
// string. That leaves one source for what is on screen: the address the guard
// says it is holding.
export const screenOfAddress = (address: string) => {
  const route = parseHash(address);
  return SCREEN_VIEWS[route.screen](route);
};

// What the held address is about, which is what the screen below is keyed on.
// Parsed from the same string for the same reason as the view above it.
const identityOfAddress = (address: string) =>
  routeIdentity(parseHash(address));

export function ScreenView({
  screen,
  id,
  id2,
}: Readonly<{ screen: Screen; id?: string; id2?: string }>) {
  // The route is deferred, which keeps splitting from costing the reader
  // anything on a navigation. React renders the asked-for screen in the
  // background. When it suspends, it keeps the screen already on the page
  // instead of committing the fallback. A committed fallback would stay on
  // screen for its own anti-flicker interval before the content shows.
  // Measured on a record open: 64ms unsplit, 813ms split with a committed
  // fallback, 44ms split and deferred. The same requests went on the wire
  // every time.
  //
  // Deferred as one value, never three: a screen that updated while an id lagged
  // would render a company page against a contact's id.
  const asked = useMemo(() => ({ screen, id, id2 }), [screen, id, id2]);
  const shown = useDeferredValue(asked);
  // One string for the whole displayed address, and it is what the unsaved-edit
  // guard below holds. Built from `shown` rather than from the live route: what
  // has to be held is what is on screen, and the deferred value is that.
  const shownAddress = routeHash(shown);
  // The boundary a lazy screen suspends against on the first paint, when there
  // is no previous screen to hold. A failure to fetch one is thrown from here
  // into AppErrorBoundary (app/errorboundary.tsx). That happens when the chunk
  // 404s because a deploy replaced it under a tab that was already open. The
  // boundary turns it into the retry card instead of a blank frame.
  return (
    <Suspense fallback={<ScreenPending />}>
      {/* Keyed by what the address is about (lib/router.tsx's routeIdentity).
          Moving to another thing is then a remount rather than a re-render.
          Two reasons, and the second is the load-bearing one. A screen carries
          state about the record it was opened for: an expanded section, a
          half-typed note, a scroll position. Reconciling one record's screen
          into another's keeps all of it. That is how a note begun on contact A
          ends up on the form for contact B. And an arrival animation
          (design-system/enter.css) plays when a block is inserted. Without a
          key the DOM nodes are reused, so walking from one record to the next
          would be the one navigation in the product with no motion at all.

          The identity, not the whole address: that is the difference between
          a tab and a destination. A tab segment names a view of the thing
          already on screen. Remounting on it threw away a page the reader had
          not left. The header, the readings, the rail and the tab strip they
          had just clicked all re-animated. Every query on the page re-observed
          and refetched.

          Two things still key on the whole address. The guard below does,
          because a half-typed note is worth asking about however small the
          move is. The shell's scroll reset does too: a panel swapped under a
          reader who had scrolled past the tab strip should still open at its
          top.

          A Fragment rather than a wrapper element: the key belongs to the
          subtree, and the shell's content column must keep the screen's own
          root as its child (shell.css sizes `.wrap` and a full-height `.lt`
          against it). */}
      {/* The guard sits here, above the screen, and not inside any one of
          them. Inside a screen it can only see moves within that screen. A
          settings draft was safe from one tab to the next. It was still
          discarded the moment the reader clicked Contacts, because the screen
          holding the guard unmounted before it could ask. Above the screen,
          every address change is one it can hold. The claim comes from
          wherever the draft is, through `useUnsavedGuard`, so no screen has to
          know this exists.

          It holds content, and leaves the URL free. The address is allowed to
          move, and the browser's own Back and Forward are left alone; what
          waits is the subtree. Refusing a history gesture means rewriting
          entries underneath the reader. Getting that wrong breaks navigation
          for everybody. */}
      <UnsavedGuard address={shownAddress} onKeep={navigateToAddress}>
        {(held) => (
          <Fragment key={identityOfAddress(held)}>
            {screenOfAddress(held)}
          </Fragment>
        )}
      </UnsavedGuard>
    </Suspense>
  );
}
