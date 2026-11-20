import {
  Fragment,
  type ReactNode,
  Suspense,
  useDeferredValue,
  useState,
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
  ResetAddressSignedIn,
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
import {
  screenEnabled,
  useDisabledCoreFeatures,
  useHiddenCoreFeatures,
} from "./lib/corefeatures";
import { paintsFromEssentialCopy } from "./lib/essentialscreens";
import { EXTENSION_SCREEN } from "./lib/extensions";
import { traversalInFlight, traverseTo } from "./lib/historydepth";
import { routeIdentity } from "./lib/routeidentity";
import {
  navigateReplacing,
  parseHash,
  type Route,
  routeHash,
  type Screen,
  samePage,
  useLiveHash,
} from "./lib/router";
import { ScreenAddress, useReportShownAddress } from "./lib/screenaddress";
import { parseParams } from "./lib/urlstate";

type ScreenArgs = Readonly<{ id?: string; id2?: string; id3?: string }>;

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
    [RESET_ROUTE]: () => <ResetAddressSignedIn />,
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

// Through history, so the entries around the held one are kept. Never a replace
// while a traversal is under way: that would rename the entry it lands on.
function returnTo(address: string, depth: number | undefined): void {
  if (traversalInFlight() || (depth !== undefined && traverseTo(depth))) {
    return;
  }
  navigateReplacing(parseHash(address), parseParams(address));
}

// The view for a held address, parsed rather than threaded alongside the
// string. That leaves one source for what is on screen: the address the guard
// says it is holding.
export const screenOfAddress = (address: string) => {
  const route = parseHash(address);
  return SCREEN_VIEWS[route.screen](route);
};

// The shell paints from the essential catalog, and the screen's column waits
// here for the complete one.
export function screenWithLocale(address: string) {
  const view = screenOfAddress(address);
  return paintsFromEssentialCopy(address) ? (
    view
  ) : (
    <CompleteLocale>{view}</CompleteLocale>
  );
}

// What the held address is about, which is what the screen below is keyed on.
// Parsed from the same string for the same reason as the view above it.
const identityOfAddress = (address: string) =>
  routeIdentity(parseHash(address));

const withoutId3 = (route: Route) => routeHash({ ...route, id3: undefined });

// App passes the route without its fourth segment, so the bar supplies that
// and the dials. A route the bar does not spell (an unserved path's not-found,
// a story with no address) is addressed by the route alone.
function askedAddress(route: Route, live: string): string {
  return withoutId3(parseHash(live)) === withoutId3(route)
    ? live
    : routeHash(route);
}

// Deferring a new page keeps the ordinary loading body, or the page before,
// on screen while its code resolves, without committing Suspense's delayed
// fallback. A dial is not deferred: the screen it turns reads it at once.
function useShownAddress(asked: string): string | null {
  const page = routeHash(parseHash(asked));
  const shownPage = useDeferredValue<string | null>(page, null);
  // What the shown page goes on reading once the bar has moved past it.
  const [left, setLeft] = useState(asked);
  if (shownPage === page && left !== asked) {
    setLeft(asked);
  }
  if (shownPage === null) {
    return null;
  }
  return shownPage === page ? asked : left;
}

function DrawnFor({
  address,
  children,
}: Readonly<{ address: string; children: ReactNode }>) {
  useReportShownAddress(address);
  return (
    <ScreenAddress.Provider value={address}>{children}</ScreenAddress.Provider>
  );
}

export function ScreenView({
  screen,
  id,
  id2,
}: Readonly<{ screen: Screen; id?: string; id2?: string }>) {
  const disabled = useDisabledCoreFeatures();
  const hidden = useHiddenCoreFeatures();
  const shown = useShownAddress(
    askedAddress({ screen, id, id2 }, useLiveHash()),
  );
  return (
    <Suspense fallback={<ScreenPending />}>
      {/* The draft guard holds content while browser history can still move. */}
      {shown === null ? (
        <ScreenPending />
      ) : (
        <UnsavedGuard address={shown} samePlace={samePage} onKeep={returnTo}>
          {(held) => (
            <DrawnFor address={held}>
              <Fragment key={identityOfAddress(held)}>
                {screenEnabled(parseHash(held).screen, hidden) ? (
                  screenWithLocale(held)
                ) : (
                  <ScreenNotice
                    messageKey={
                      screenEnabled(parseHash(held).screen, disabled)
                        ? "shell.unavailableForRole"
                        : "shell.unknownPage"
                    }
                  />
                )}
              </Fragment>
            </DrawnFor>
          )}
        </UnsavedGuard>
      )}
    </Suspense>
  );
}
