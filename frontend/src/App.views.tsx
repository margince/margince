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
  OnboardingScreen,
  PartnersScreen,
  PreferenceCenterScreen,
  ProgressScreen,
  ProjectScreen,
  ProjectsScreen,
  ResetRoute,
  SalesGoalsScreen,
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
import { CREATE_ID, IMPORT_ID, opensReservedPage } from "./app/nav";
import { UnsavedGuard } from "./components/unsaved";
import { RESET_ROUTE } from "./features/sign-in/auth.route";
import { CompleteLocale } from "./i18n";
import { isContactTab } from "./lib/contacttab";
import { screenEnabled, useDisabledCoreFeatures } from "./lib/corefeatures";
import { EXTENSION_SCREEN } from "./lib/extensions";
import { routeIdentity } from "./lib/routeidentity";
import { navigate, parseHash, routeHash, type Screen } from "./lib/router";

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
        <ContactsScreen
          importing={id === IMPORT_ID}
          startCreating={id === CREATE_ID}
        />
      ) : (
        <ContactPageV2 id={id} tab={isContactTab(id2) ? id2 : "overview"} />
      ),
    companies: ({ id }) =>
      !id || opensReservedPage({ screen: "companies", id }) ? (
        <CompaniesScreen startCreating={id !== undefined} />
      ) : (
        <CompanyScreen id={id} />
      ),
    partners: () => <PartnersScreen />,
    leads: ({ id }) =>
      !id || opensReservedPage({ screen: "leads", id }) ? (
        <LeadsScreen startCreating={id !== undefined} />
      ) : (
        <LeadScreen id={id} />
      ),
    deals: ({ id, id2 }) => <DealsRoute id={id} id2={id2} />,
    projects: ({ id }) =>
      !id || opensReservedPage({ screen: "projects", id }) ? (
        <ProjectsScreen startCreating={id !== undefined} />
      ) : (
        <ProjectScreen id={id} />
      ),
    // One segment, and it names whose day it is. A team board row needs that
    // door. The other three dials stay state: one of four in the address would
    // make it describe a fraction of what the reader is looking at.
    worklist: ({ id }) => <WorklistRedirect opensOn={id} />,
    analytics: () => <AnalyticsScreen />,
    // The period and the person or team ride the query, as Analytics dials do.
    progress: () => <ProgressScreen />,
    goals: () => <SalesGoalsScreen />,
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
    book: ({ id, id2 }) => <BookingScreen hostSlug={id} page={id2} />,
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

function screenWithLocale(address: string) {
  const view = screenOfAddress(address);
  return parseHash(address).screen === "analytics" ? (
    view
  ) : (
    <CompleteLocale>{view}</CompleteLocale>
  );
}

// What the held address is about, which is what the screen below is keyed on.
// Parsed from the same string for the same reason as the view above it.
const identityOfAddress = (address: string) =>
  routeIdentity(parseHash(address));

export function ScreenView({
  screen,
  id,
  id2,
}: Readonly<{ screen: Screen; id?: string; id2?: string }>) {
  // Deferring the first route keeps the ordinary loading body on screen while
  // its code resolves, without committing Suspense's delayed fallback.
  const disabled = useDisabledCoreFeatures();
  const asked = useMemo(() => ({ screen, id, id2 }), [screen, id, id2]);
  const shown = useDeferredValue<typeof asked | null>(asked, null);
  const shownAddress = shown ? routeHash(shown) : null;
  return (
    <Suspense fallback={<ScreenPending />}>
      {/* The draft guard holds content while browser history can still move. */}
      {shownAddress === null ? (
        <ScreenPending />
      ) : (
        <UnsavedGuard address={shownAddress} onKeep={navigateToAddress}>
          {(held) => (
            <Fragment key={identityOfAddress(held)}>
              {screenEnabled(parseHash(held).screen, disabled) ? (
                screenWithLocale(held)
              ) : (
                <ScreenNotice messageKey="shell.unknownPage" />
              )}
            </Fragment>
          )}
        </UnsavedGuard>
      )}
    </Suspense>
  );
}
