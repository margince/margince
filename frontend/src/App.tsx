import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";
// Route → screen. The table below is total over `Screen` (lib/router.tsx), so
// every address the product answers has a view. An address it does not answer
// is a not-found page rather than a surface that reads as unbuilt.
import { ResetRoute, ROUTE_CHUNKS } from "./App.screens";
import { ScreenView } from "./App.views";
import { AskFromAddress } from "./app/askfromaddress";
import { isPublicBookingId } from "./app/bookingroute";
import { AuthSplash } from "./app/bootsplash";
import { ConfinedAccountScreen, useProbeAnswer } from "./app/confinement";
import { DateFormatsProvider } from "./app/dateformats";
import { prefetchHome } from "./app/homeprefetch";
import { passesOnboardingGate } from "./app/onboarding-gate";
import {
  CommandPalette,
  useBuiltinCommands,
  usePaletteHotkey,
} from "./app/palette";
import { Shell, useRoute } from "./app/shell";
import { SourceHost } from "./app/sourcehost";
import { CallLogHost } from "./components/calllog.host";
import { useJourneyProgress } from "./features/onboarding/onboarding-journey";
import {
  ReleaseSkewScreen,
  useSkewedApiRelease,
} from "./features/settings/releaseskew";
import { WriteToHost } from "./features/settings/writetohost";
import {
  type AuthNotice,
  AuthScreen,
  AvailabilityScreen,
  RESET_ROUTE,
} from "./features/sign-in/auth";
import {
  fetchSetupStatus,
  SetupClaimScreen,
} from "./features/sign-in/setupclaim";
import { useLocale } from "./i18n";
import { useInstallationDescribed } from "./lib/installationcompany";
import { markJourney } from "./lib/journeymarks";
import { RecordZoneProvider, useConfiguredRecordZone } from "./lib/recordzone";
import { SPA_RELEASE } from "./lib/release";
import { navigateReplacing, type Screen } from "./lib/router";
import { AuthProbeError, consumeAuthExitNotice, useMe } from "./lib/session";
import { useTaskReminderNotifications } from "./lib/taskreminders";
import { currentParams } from "./lib/urlstate";

// The anonymous public surfaces render without a session — their slug in the
// path is the whole address (security: [] in the contract).
const PUBLIC_SCREENS: ReadonlySet<Screen> = new Set([
  "book",
  "confirm",
  "preferences",
  "unsubscribe",
  "room",
]);

export function App() {
  // Ahead of every gate below, since reading the address also takes a one-time
  // credential out of it (lib/router.tsx). A credential must leave the bar
  // whatever this function renders instead.
  const route = useRoute();
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
  // The emailed reset link is a bearer credential, not a session check: it
  // must reach the reset form whether or not this browser already carries a
  // live cookie, so it is routed here, ahead of AuthedApp's session gate,
  // rather than left for AuthedApp to reach only when unauthenticated (where
  // an existing session instead rendered the authenticated shell's fallback
  // for an unrecognised route).
  if (route.screen === RESET_ROUTE) {
    return (
      <RaillessFrame>
        <ResetRoute />
      </RaillessFrame>
    );
  }
  return <AuthedApp route={route} />;
}

// UnavailableOrClaimable splits the 503 the boundary already reached into its
// two product states. "Not ready" is true of both, but only one of them has
// something the contact in front of the browser can do: an installation that
// holds no company and is WAITING to be claimed (ADR-0105) gets the claim
// screen; anything else keeps the availability message.
//
// The probe runs only on this branch, and only for the installation kind: a
// connectivity failure says nothing about whether a claim is possible, and
// asking would replace a true message with a guess. While it is in flight the
// availability screen stands — it is the honest answer until the probe says
// otherwise, and it is what the user would have seen anyway.
function UnavailableOrClaimable({
  kind,
  onRetry,
}: Readonly<{ kind: "connection" | "installation"; onRetry: () => void }>) {
  const claimable = useQuery({
    queryKey: ["setup-status"],
    queryFn: fetchSetupStatus,
    enabled: kind === "installation",
    retry: false,
  });
  // Retry re-probes BOTH. fetchSetupStatus resolves rather than throws, so a
  // one-off failure caches a `false` under this key — without refetching it,
  // "Try again" would keep showing the availability screen for an installation
  // that has been claimable all along, and only a page reload would recover.
  const retryBoth = () => {
    onRetry();
    if (kind === "installation") {
      void claimable.refetch();
    }
  };
  if (kind === "installation" && claimable.data?.claimable) {
    // A successful claim provisions the installation, so the same /v1/me probe
    // that sent us here now answers — the boundary re-resolves rather than this
    // screen deciding where to go next.
    return <SetupClaimScreen onClaimed={onRetry} status={claimable.data} />;
  }
  return <AvailabilityScreen kind={kind} onRetry={retryBoth} />;
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
  // A 401 after a previously live session is an expiry (or a deliberate
  // sign-out, which useLogout marks); a 401 on first load is just "not
  // signed in" and carries no notice.
  const hadSession = useRef(false);
  const [notice, setNotice] = useState<AuthNotice>(null);
  // The signed-in person's own language, handed up to the provider that sits
  // above this component. It cannot ask for `/me` itself, and this is the first
  // place the answer exists — so a choice made on another browser reaches the
  // catalog here or nowhere.
  const { adoptLocale } = useLocale();
  useEffect(() => {
    const chosen = me.data?.user?.locale;
    if (chosen) {
      adoptLocale(chosen);
    }
  }, [me.data, adoptLocale]);
  useEffect(() => {
    if (me.data) {
      hadSession.current = true;
      setNotice(null);
      markJourney("session");
      return;
    }
    if (
      me.error instanceof AuthProbeError &&
      me.error.kind === "unauthorized"
    ) {
      const exit = consumeAuthExitNotice();
      if (exit) {
        setNotice(exit);
      } else if (hadSession.current) {
        setNotice("session-expired");
      }
      hadSession.current = false;
    }
  }, [me.data, me.error]);

  // Probed only once the session is known good: an unauthenticated /company
  // would 401 and say nothing about onboarding.
  const authed = !me.isPending && !me.isError;
  const { described, pending: companyPending } =
    useInstallationDescribed(authed);
  const progress = useJourneyProgress(
    authed,
    authed &&
      described === true &&
      me.data?.authorization?.seat_type === "full",
  );
  // The company's clock, for every record date under this boundary. Read
  // here rather than per screen so all of them agree, and gated on the session
  // for the same reason the company probe is: an unauthenticated read would
  // 401 and say nothing about the installation.
  const recordZone = useConfiguredRecordZone(authed);

  // The route is a dependency so the gate holds on every navigation. Otherwise
  // the palette or a typed hash walks past onboarding.
  // passesOnboardingGate's screens are exempt, or this effect would fight its
  // own destination (onboarding) or destroy a request that cannot survive the
  // rewrite (oauth-consent).
  const { screen: gateScreen, id: gateId, id2: gateId2 } = route;
  useEffect(() => {
    if (
      authed &&
      described !== undefined &&
      (!described || progress.unfinished) &&
      !passesOnboardingGate({ screen: gateScreen, id: gateId, id2: gateId2 })
    ) {
      // Replacing, not pushing: this is a redirect, and the address it sends
      // the reader away from answers by sending them here again. Pushed, Back
      // lands on that address, the effect fires, and the reader is returned to
      // onboarding by the one key that exists for getting out of things.
      navigateReplacing({ screen: "onboarding", id: "company" });
    }
  }, [authed, described, progress.unfinished, gateScreen, gateId, gateId2]);

  const { refusal, confinement } = useProbeAnswer(me);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const gatesPending = companyPending || recordZone.pending || progress.pending;
  const shellReady = authed && !gatesPending;
  // Home's reads start here rather than when its lazy screen commits, and only
  // once nothing will send the reader to onboarding instead.
  const opensHome =
    shellReady &&
    described === true &&
    !progress.unfinished &&
    route.screen === "home";
  const queryClient = useQueryClient();
  useEffect(() => {
    if (shellReady) {
      markJourney("gates");
    }
  }, [shellReady]);
  useEffect(() => {
    if (!opensHome) {
      return;
    }
    return prefetchHome(queryClient, currentParams());
  }, [opensHome, queryClient]);
  const commands = useBuiltinCommands();
  usePaletteHotkey(paletteOpen, setPaletteOpen);
  useTaskReminderNotifications(authed);

  if (me.isPending && refusal === null) {
    return (
      <RaillessFrame>
        <AuthSplash />
      </RaillessFrame>
    );
  }
  if (confinement) {
    return (
      <RaillessFrame>
        <ConfinedAccountScreen {...confinement} />
      </RaillessFrame>
    );
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
      <RecordZoneProvider zone={recordZone.zone}>
        {/* An address pressed on any screen writes from here: the composer is
          the product's, not the reader's mail client's. */}
        <WriteToHost>
          <CallLogHost>
            <SourceHost>
              <AuthedShell onOpenSearch={() => setPaletteOpen(true)}>
                <ScreenView
                  screen={route.screen}
                  id={route.id}
                  id2={route.id2}
                />
              </AuthedShell>
            </SourceHost>
          </CallLogHost>
        </WriteToHost>
        <CommandPalette
          open={paletteOpen}
          onClose={() => setPaletteOpen(false)}
          commands={commands}
        />
        {/* Over whatever the reader was doing, not on a screen of its own. */}
        <AskFromAddress />
      </RecordZoneProvider>
    </DateFormatsProvider>
  );
}

// Route warm-up runs only after login.
// Fetches the route chunks in the background, once, for a reader who is past
// the login screen. It runs at idle so it never competes with the screen the
// reader is actually looking at, and with a deadline so a busy tab cannot defer
// it indefinitely — a click that arrives before the warm-up finishes suspends
// exactly as it would have without one, so the worst case is the behaviour
// without this hook rather than a slower one.
function useWarmRouteChunks() {
  useEffect(() => {
    let cancelled = false;
    // ONE chunk in flight at a time. Asking for all of them at once empties the
    // browser's per-host connection pool, and the read the reader is actually
    // waiting on — the record they just opened — queues behind two dozen files
    // nobody asked for. Warming that makes the next navigation slower is worse
    // than not warming at all.
    const warm = async () => {
      for (const load of ROUTE_CHUNKS) {
        if (cancelled) {
          return;
        }
        try {
          await load();
        } catch (reason: unknown) {
          // Nobody asked for this screen yet, so a failed fetch is not the
          // reader's problem: the navigation that does ask will suspend and
          // surface it through the error boundary. It is still worth saying out
          // loud, because a chunk that cannot be fetched is a broken deploy.
          console.warn("route chunk warm-up failed", reason);
        }
      }
    };
    // Typed as always present, and absent in jsdom and in older Safari, so the
    // check is a runtime one whatever the DOM lib claims.
    if (typeof window.requestIdleCallback === "function") {
      const handle = window.requestIdleCallback(() => void warm(), {
        timeout: 500,
      });
      return () => {
        cancelled = true;
        window.cancelIdleCallback(handle);
      };
    }
    const handle = window.setTimeout(() => void warm(), 0);
    return () => {
      cancelled = true;
      window.clearTimeout(handle);
    };
  }, []);
}

function AuthedShell({
  children,
  onOpenSearch,
}: Readonly<{ children: ReactNode; onOpenSearch: () => void }>) {
  useWarmRouteChunks();
  return <Shell onOpenSearch={onOpenSearch}>{children}</Shell>;
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
