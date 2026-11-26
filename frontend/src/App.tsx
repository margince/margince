import { useQueryClient } from "@tanstack/react-query";
import {
  type ReactNode,
  Suspense,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
// Route → screen. The table below is total over `Screen` (lib/router.tsx), so
// every address the product answers has a view. An address it does not answer
// is a not-found page rather than a surface that reads as unbuilt.
import { ResetRoute } from "./App.screens";
import { ScreenView } from "./App.views";
import { useWarmRouteChunks } from "./App.warmchunks";
import { ApprovalHost } from "./app/approvalhost";
import { AskFromAddress } from "./app/askfromaddress";
import { AuthScreen, ReleaseSkewScreen } from "./app/authscreens";
import { isPublicBookingId } from "./app/bookingroute";
import { AuthSplash } from "./app/bootsplash";
import { lockScreen, useProbeAnswer } from "./app/confinement";
import { DateFormatsProvider } from "./app/dateformats";
import { useFeatureLanding } from "./app/featurelanding";
import { useHomePrefetch } from "./app/homeprefetch";
import { ONBOARDING_ENTRY, passesOnboardingGate } from "./app/onboarding-gate";
import { useCommandPalette } from "./app/palettehotkey";
import { Shell, useRoute } from "./app/shell";
import { SourceHost } from "./app/sourcehost";
import { UnavailableOrClaimable } from "./app/unavailableorclaimable";
import { CallLogHost } from "./components/calllog.host";
import { UnsavedExits } from "./components/unsaved";
import { useJourneyProgress } from "./features/onboarding/onboarding-journey";
import { WriteToHost } from "./features/settings/writetohost";
import type { AuthNotice } from "./features/sign-in/auth";
import { RESET_ROUTE } from "./features/sign-in/auth.route";
import { useAccountLocale, useLocale } from "./i18n";
import { lazy } from "./i18n/lazy";
import { useSkewedApiRelease } from "./lib/capabilities";
import {
  CompanyZoneProvider,
  useConfiguredCompanyZone,
} from "./lib/companyzone";
import { screenEnabled, useHiddenCoreFeatures } from "./lib/corefeatures";
import { useInstallationDescribed } from "./lib/installationcompany";
import { notifyNavigate } from "./lib/instruments";
import { markJourney } from "./lib/journeymarks";
import { useWarmProgressName } from "./lib/progressreader";
import { SPA_RELEASE } from "./lib/release";
import { routePattern } from "./lib/routepattern";
import { type Screen, takeHashCredential, watchAddress } from "./lib/router";
import { AuthProbeError, consumeAuthExitNotice, useMe } from "./lib/session";
import { sessionSeenNow } from "./lib/session.read";
import {
  removeStored,
  STORAGE_KEYS,
  watchStored,
  writeStored,
} from "./lib/storage";
import { useTaskReminderNotifications } from "./lib/taskreminders";
import { useAddressWriters } from "./lib/urlstate";
import {
  renderableViewerZone,
  useBrowserZoneOnFirstSignIn,
  ViewerZoneProvider,
} from "./lib/viewerzone";

const CommandPalette = lazy(async () => ({
  default: (await import("./app/palette")).BuiltinCommandPalette,
}));

// The anonymous public surfaces render without a session — their slug in the
// path is the whole address (security: [] in the contract).
const PUBLIC_SCREENS: ReadonlySet<Screen> = new Set([
  "book",
  "confirm",
  "confirm-merge",
  "preferences",
  "unsubscribe",
  "room",
]);

export function App() {
  // Ahead of every gate below, since reading the address also takes a one-time
  // credential out of it (lib/router.tsx). A credential must leave the bar
  // whatever this function renders instead.
  const route = useRoute();
  const pattern = routePattern(route);
  useEffect(() => notifyNavigate(pattern), [pattern]);
  const queryClient = useQueryClient();
  const { locale } = useLocale();
  const previousLocale = useRef(locale);
  useEffect(() => {
    if (previousLocale.current === locale) return;
    previousLocale.current = locale;
    queryClient.removeQueries({
      queryKey: ["reader-rendering"],
      type: "inactive",
    });
    void queryClient.invalidateQueries({
      predicate: (entry) =>
        entry.queryKey[0] !== "me" && entry.queryKey[0] !== "reader-rendering",
    });
  }, [locale, queryClient]);

  // BEFORE every other gate, the public screens included. A bundle and an api
  // from different releases disagree about the contract between them, so nothing
  // below here — the session probe, the booking page, the preference centre —
  // can be trusted to work, and a reader is better served by one honest screen
  // than by whichever request happens to fail first. It is inert on any build
  // that carries no release version, which is every local one.
  const skewedApiRelease = useSkewedApiRelease(SPA_RELEASE);
  if (skewedApiRelease !== null) {
    return (
      <RaillessFrame>
        <ReleaseSkewScreen app={SPA_RELEASE} server={skewedApiRelease} />
      </RaillessFrame>
    );
  }
  if (
    PUBLIC_SCREENS.has(route.screen) &&
    (route.screen !== "book" || isPublicBookingId(route.id))
  ) {
    return (
      <Shell onOpenSearch={() => undefined}>
        <ScreenView screen={route.screen} id={route.id} id2={route.id2} />
      </Shell>
    );
  }
  if (route.screen === RESET_ROUTE) {
    return <ResetEntry route={route} />;
  }
  return <AuthedApp route={route} />;
}

// The emailed reset link is a bearer credential, not a session check. With its
// token it reaches the reset form ahead of AuthedApp's session gate; without one
// it goes through the gate. A link pasted while the gate waits is scrubbed back
// to this same address, so the router is watched, not the route. A token once
// seen stays decided, since the form forgets it as it takes it.
function ResetEntry({
  route,
}: Readonly<{ route: ReturnType<typeof useRoute> }>) {
  const tokenHeld = useSyncExternalStore(
    watchAddress,
    () => takeHashCredential(RESET_ROUTE) !== null,
  );
  const [carriesToken, setCarriesToken] = useState(tokenHeld);
  if (tokenHeld && !carriesToken) {
    setCarriesToken(true);
  }
  if (!carriesToken) {
    return <AuthedApp route={route} />;
  }
  return (
    <RaillessFrame>
      <ResetRoute />
    </RaillessFrame>
  );
}

// Watched, so a sign-out here or in another tab ends Home's early reads at once.
const watchSessionSeen = (changed: () => void) =>
  watchStored(STORAGE_KEYS.sessionSeen, changed);

// Marks whether this browser has a session, and answers whether the probe was
// refused. A refused refetch keeps the last data beside its error.
function noteSessionSeen(data: unknown, error: unknown): boolean {
  const refused =
    error instanceof AuthProbeError && error.kind === "unauthorized";
  if (refused) {
    removeStored(STORAGE_KEYS.sessionSeen);
  } else if (data && !error) {
    writeStored(STORAGE_KEYS.sessionSeen, "1");
  }
  return refused;
}

type HomeReadState = Readonly<{
  screen: Screen;
  homeAvailable: boolean;
  signedIn: boolean;
  sessionSeen: boolean;
  refused: boolean;
  confined: boolean;
  heldForOnboarding: boolean;
  rememberedFinished: boolean;
  gatesPending: boolean;
}>;

// On a browser that saw this reader finish onboarding, Home's reads start with
// the session probe. Waiting for the gates it opens costs two round trips. An
// answer that sends the reader elsewhere stops them, and the cleanup aborts
// them. Any other reader's reads wait for the gates to admit them.
function homeMayRead(state: HomeReadState): boolean {
  return (
    state.screen === "home" &&
    state.homeAvailable &&
    (state.signedIn || state.sessionSeen) &&
    !state.refused &&
    !state.confined &&
    !state.heldForOnboarding &&
    (state.rememberedFinished || (state.signedIn && !state.gatesPending))
  );
}

type OnboardingHold = Readonly<{ held: boolean; turnedAway: boolean }>;

// The route is a dependency so the gate holds on every navigation. Otherwise
// the palette or a typed hash walks past onboarding.
// passesOnboardingGate's screens are exempt, or this effect would fight its
// own destination (onboarding) or destroy a request that cannot survive the
// rewrite (oauth-consent).
function useOnboardingHold(
  authed: boolean,
  described: boolean | undefined,
  unfinished: boolean,
  route: ReturnType<typeof useRoute>,
): OnboardingHold {
  const held = authed && described !== undefined && (!described || unfinished);
  const { screen, id, id2 } = route;
  const { navigateReplacing } = useAddressWriters();
  useEffect(() => {
    if (held && !passesOnboardingGate({ screen, id, id2 })) {
      // Replacing, not pushing: this is a redirect. The address it sends the
      // reader away from answers by sending them here again. Pushed, Back
      // lands on that address and the effect fires. The one key meant for
      // getting out of things then returns the reader to onboarding.
      navigateReplacing(ONBOARDING_ENTRY);
    }
  }, [held, screen, id, id2, navigateReplacing]);
  return { held, turnedAway: held && !passesOnboardingGate(route) };
}

// AuthGate: everything behind the session probes GET /v1/me, and the
// boundary branches on the TYPED failure (§4 of the login spec):
// 401 → login, network/5xx → connection problem, 503 → installation
// unavailable. Authentication and availability are different product
// states — a server outage must never read as "wrong password". On login
// success the screen refetches and the app renders at the route the user
// originally asked for. No redirect races: the gate owns the decision.
function AuthedApp({
  route,
}: Readonly<{ route: ReturnType<typeof useRoute> }>) {
  const me = useMe();
  const disabled = useHiddenCoreFeatures();
  // A 401 after a previously live session is an expiry (or a deliberate
  // sign-out, which useLogout marks); a 401 on first load is just "not
  // signed in" and carries no notice.
  const hadSession = useRef(false);
  const [notice, setNotice] = useState<AuthNotice>(null);
  // The signed-in user's own language, handed up to the provider that sits
  // above this component. It cannot ask for `/me` itself, and this is the first
  // place the answer exists — so a choice made on another browser reaches the
  // catalog here or nowhere.
  useAccountLocale(me.data?.user?.locale);
  // A user with no stored zone gets the browser's as a first guess. They
  // change it under Settings → Account.
  useBrowserZoneOnFirstSignIn(me.data);
  useEffect(() => {
    // The refusal is tested first: a refused re-read still holds the old data.
    if (noteSessionSeen(me.data, me.error)) {
      const exit = consumeAuthExitNotice();
      if (exit) {
        setNotice(exit);
      } else if (hadSession.current) {
        setNotice("session-expired");
      }
      hadSession.current = false;
      return;
    }
    if (me.data) {
      hadSession.current = true;
      setNotice(null);
      markJourney("session");
    }
  }, [me.data, me.error]);

  // Probed only once the session is known good: an unauthenticated /company
  // would 401 and say nothing about onboarding.
  const authed = !me.isPending && !me.isError;
  const { described, pending: companyPending } = useInstallationDescribed(
    authed,
    { readProfile: false },
  );
  const progress = useJourneyProgress(authed, described, me.data?.user?.id);
  // The company's clock, for every record date under this boundary. Read
  // here rather than per screen so all of them agree, and gated on the session
  // for the same reason the company probe is: an unauthenticated read would
  // 401 and say nothing about the installation.
  const companyZone = useConfiguredCompanyZone(authed);
  // The reader's own clock, for every time a signed-in screen shows. It is
  // the zone they stored, or their browser's until they have one.
  const storedZone = me.data?.user.timezone;
  const browserZone = useMemo(
    () => renderableViewerZone(storedZone),
    [storedZone],
  );

  const hold = useOnboardingHold(authed, described, progress.unfinished, route);

  const { refusal, confinement } = useProbeAnswer(me);
  const sessionSeen = useSyncExternalStore(
    watchSessionSeen,
    sessionSeenNow,
    sessionSeenNow,
  );
  const palette = useCommandPalette(route, !hold.held);
  const gatesPending =
    companyPending || companyZone.pending || progress.pending;
  const shellReady = authed && !gatesPending;
  const opensHome = homeMayRead({
    homeAvailable: screenEnabled("home", disabled),
    screen: route.screen,
    signedIn: me.data !== undefined,
    sessionSeen,
    refused: me.isError,
    confined: confinement !== null,
    heldForOnboarding: hold.held,
    rememberedFinished: progress.remembered,
    gatesPending,
  });
  useEffect(() => {
    if (shellReady) {
      markJourney("gates");
    }
  }, [shellReady]);
  useHomePrefetch(opensHome, me.data);
  useTaskReminderNotifications(authed);
  useFeatureLanding(shellReady, confinement !== null, disabled);

  if (me.isPending && refusal === null) {
    return (
      <RaillessFrame>
        <AuthSplash />
      </RaillessFrame>
    );
  }
  const lock = lockScreen(confinement, refusal, me.data);
  if (lock) {
    return <RaillessFrame>{lock}</RaillessFrame>;
  }
  if (refusal !== null) {
    const kind =
      refusal instanceof AuthProbeError ? refusal.kind : "connection";
    if (kind === "connection" || kind === "installation") {
      const retry = () => me.refetch({ cancelRefetch: false });
      return (
        <RaillessFrame>
          <UnavailableOrClaimable kind={kind} onRetry={retry} />
        </RaillessFrame>
      );
    }
    return (
      <RaillessFrame>
        <AuthScreen
          onAuthed={async () => {
            const result = await me.refetch();
            if (result.error) {
              throw result.error;
            }
          }}
          notice={notice}
        />
      </RaillessFrame>
    );
  }

  // An installation that has not described itself has nothing for any other
  // screen to show. The gate lives here rather than on the login path because
  // a live session never passes through login — a reload would otherwise walk
  // straight past onboarding into a company that does not exist.
  // The record zone joins this gate rather than getting one of its own: both
  // are answers the authenticated shell needs before it draws, and a second
  // splash after the first would read as two loads of one page. Holding here
  // is what lets every screen below take the zone as a settled value — paint
  // first and the day headings on an open timeline renumber under the reader.
  //
  // What makes holding here safe is that the wait is BOUNDED: every request
  // through api/client.ts carries a deadline, so a read that opens and never
  // answers becomes a rejection rather than an eternal `isPending`. Without
  // one, this splash is a screen with no error, no retry and no explanation
  // that only a reload leaves — and it covers routes that need neither read,
  // onboarding and OAuth consent among them. A read that FAILS falls through
  // to the shell, where each screen renders its own error state and its own
  // retry; the splash is for waiting, not for having waited.
  // The progress and rollout reads join the splash for the same reason the
  // company read does: a shell painted before they answer is a landing page the
  // gate then pulls away from under the reader.
  if (gatesPending) {
    return (
      <RaillessFrame>
        <AuthSplash />
      </RaillessFrame>
    );
  }

  return (
    <DateFormatsProvider>
      <CompanyZoneProvider zone={companyZone.zone}>
        <ViewerZoneProvider zone={browserZone}>
          {/* An address pressed on any screen writes from here: the composer is
          the product's, not the reader's mail client's. */}
          <WriteToHost>
            <CallLogHost>
              <SourceHost>
                <ApprovalHost>
                  {/* Sign out in the shell's account menu asks the page's guard. */}
                  <UnsavedExits>
                    <AuthedShell
                      hold={hold}
                      onOpenSearch={() => palette.show("all")}
                    >
                      <ScreenView
                        screen={route.screen}
                        id={route.id}
                        id2={route.id2}
                      />
                    </AuthedShell>
                  </UnsavedExits>
                </ApprovalHost>
              </SourceHost>
            </CallLogHost>
          </WriteToHost>
          {palette.open !== null && (
            <Suspense fallback={null}>
              <CommandPalette
                open={palette.open}
                mode={palette.mode}
                onClose={palette.hide}
              />
            </Suspense>
          )}
          {/* Over whatever the reader was doing, not on a screen of its own. */}
          <AskFromAddress />
        </ViewerZoneProvider>
      </CompanyZoneProvider>
    </DateFormatsProvider>
  );
}

function AuthedShell({
  children,
  hold,
  onOpenSearch,
}: Readonly<{
  children: ReactNode;
  hold: OnboardingHold;
  onOpenSearch: () => void;
}>) {
  useWarmRouteChunks();
  useWarmProgressName(!hold.held);
  // The gate is replacing this address, and its shell would only start the
  // full app's reads.
  if (hold.turnedAway) {
    return (
      <RaillessFrame>
        <AuthSplash />
      </RaillessFrame>
    );
  }
  return (
    <Shell onOpenSearch={onOpenSearch} held={hold.held}>
      {children}
    </Shell>
  );
}

// The rail-less page frame (same shape Shell renders for onboarding/booking),
// so the pre-session screens get the app background and scroll container.
function RaillessFrame({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <div className="app railless">
      <main className="main">
        <div className="scroll">{children}</div>
      </main>
    </div>
  );
}
