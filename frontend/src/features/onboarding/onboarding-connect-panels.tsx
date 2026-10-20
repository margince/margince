import { useMutation } from "@tanstack/react-query";
import { ArrowRight, CircleCheck, Mail, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../../api/client";
import { Badge, Button, Disclosure } from "../../design-system/atoms";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemMessageOf, unwrap } from "../../lib/problem";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../../lib/storage";
import { CaptureNotice } from "../capture/capture-notice";
import { useConnectors } from "../capture/connectors";
import { ConnectPostureStep } from "../settings/connect-posture";
import { OnboardingBackread } from "./onboarding-backread";
import { ConnectWarn } from "./onboarding-connect-panels.imap";

// The provider connect panels: real inbox capture, one panel per provider.
// The conversational connect act renders them in the artifact panel behind
// the per-purpose consent turn; connecting stays value-before-permission
// and the panels never claim a connection the server did not confirm.
//
// A confirmed OAuth connection hands straight to the backread step: access
// granted is not history read, and the two are asked separately — the grant
// costs nothing, reading the history spends budget.

export { ImapConnectPanel } from "./onboarding-connect-panels.imap";

// The OAuth outcomes that no retry can clear: the provider refused the grant,
// its API was never enabled for this deployment, or it refused this
// deployment's client credentials. Keyed off the server's outcome segment, and
// pointing at the same copy Settings renders so the two surfaces cannot drift
// apart.
const PERMANENT_FAILURE_BODY: Record<string, MessageKey | undefined> = {
  misconfigured: "connectors.oauthMisconfigured",
  bad_client: "connectors.oauthBadClient",
  rejected: "connectors.oauthRejected",
};

export type OAuthProvider = "gmail" | "graph";

const OAUTH_PROVIDERS: readonly OAuthProvider[] = ["gmail", "graph"];

// The consent return carries its provider as a route segment. A route segment
// is just text, so it is narrowed by membership in the known set — never
// asserted into the union. null means "no provider this build knows", which is
// NOT the same fact as the segment being absent: the caller keeps the two apart.
function asOAuthProvider(value: string | undefined): OAuthProvider | null {
  return OAUTH_PROVIDERS.find((p) => p === value) ?? null;
}

// A real "allow" click leaves the page entirely for the provider's own
// consent screen — this tab's sessionStorage is the only thing that survives
// that round trip, so it is what tells a genuine return apart from a stale
// or bookmarked `/connect/ok/...` URL replayed with no live attempt behind
// it. `sessionStorage` (not `localStorage`) is deliberate: the mark belongs
// to the ONE tab that started the trip, the same scope the redirect itself
// stays within.
function markOAuthAttempt(provider: OAuthProvider): void {
  writeStored(STORAGE_KEYS.oauthAttempt, provider);
}

/** The provider a real attempt from THIS tab is returning for, or null if
 * none is recorded — read-only, so the caller decides when the mark is
 * actually spent (`clearOAuthAttempt`). */
export function peekOAuthAttempt(): OAuthProvider | null {
  return asOAuthProvider(readStored(STORAGE_KEYS.oauthAttempt) ?? undefined);
}

/** Spends the mark so a reload of the same return URL reads as the stale
 * link it now is, not a second live attempt. */
export function clearOAuthAttempt(): void {
  removeStored(STORAGE_KEYS.oauthAttempt);
}

const OAUTH_COPY: Record<
  OAuthProvider,
  {
    btn: MessageKey;
    hint: MessageKey;
    unverified: MessageKey;
    failed: MessageKey;
  }
> = {
  gmail: {
    btn: "ob.s4.googleBtn",
    hint: "ob.s4.googleHint",
    unverified: "ob.s4.googleUnverified",
    failed: "ob.s4.googleFailed",
  },
  graph: {
    btn: "ob.s4.microsoftBtn",
    hint: "ob.s4.microsoftHint",
    unverified: "ob.s4.microsoftUnverified",
    failed: "ob.s4.microsoftFailed",
  },
};

// Pre-consent: the server mints the consent URL (and the signed state + CSRF
// cookie that guard the callback); the browser just goes. One panel serves
// every OAuth provider — only the copy and the POST path vary.
//
// This panel never signals completion itself: a real "allow" click leaves
// the page for the provider's own consent screen, and the connection is
// confirmed only once the redirect returns, by `OAuthReturnPanel`. `onDismiss`
// closes whatever is showing this panel WITHOUT deciding anything — the
// reader changed their mind about this one provider, not about connecting at
// all. Skipping the whole required step is a separate, more deliberate
// action the surface offers beside the provider choice, not a button buried
// inside one provider's own ask.
export function OAuthConnectPanel({
  provider,
  onDismiss,
  onPendingChange,
}: Readonly<{
  provider: OAuthProvider;
  onDismiss: () => void;
  /**
   * Told whenever the connect POST's in-flight state changes. The caller
   * wraps the dialog's own close handler with it: X, Escape, and backdrop
   * dismissal all resolve to that ONE handler (see `ConnectDialog`), so this
   * is the one place that lets them honor the same no-dismissal-during-submit
   * invariant the disabled "Not now" button below already enforces.
   */
  onPendingChange?: (pending: boolean) => void;
}>) {
  const t = useT();
  const copy = OAUTH_COPY[provider];
  const connect = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.POST("/connectors/{provider}/connect", {
          params: { path: { provider } },
          body: {},
        }),
      );
    },
    onSuccess: (data) => {
      if (data.authorize_url) {
        markOAuthAttempt(provider);
        globalThis.location.assign(data.authorize_url);
      }
    },
  });
  useEffect(() => {
    onPendingChange?.(connect.isPending);
  }, [connect.isPending, onPendingChange]);
  return (
    <>
      <div className="form-stack">
        {connect.isError && (
          <ConnectWarn
            title={t(copy.failed)}
            body={problemMessageOf(connect.error, t)}
          />
        )}
        {/* What the grant covers is background: true, and not what the reader is
          deciding at this moment, which is whether to press Connect. */}
        <Disclosure summary={t("ob.s4.accessToggle")}>
          <p className="spoken-hint t-caption">
            <ShieldCheck aria-hidden /> {t(copy.hint)}
          </p>
        </Disclosure>
        {/* The warning stays on the surface. It describes the NEXT screen — the
          provider calls a self-hosted app unverified — so a reader who meets
          that screen without having been told reasonably concludes something
          is wrong with the thing they just pressed. A caution about what a
          button does belongs beside the button, never behind a fold. */}
        <p className="ob-google-unverified">{t(copy.unverified)}</p>
        {/* Last thing read before the grant screen, because after it the mailbox
          is connected and the telling is too late. */}
        <CaptureNotice />
      </div>
      <div className="actions">
        <Button
          // Dismissal is a decision NOT to connect, so it must not be
          // available while the credential POST it would abandon is still
          // in flight: a success landing after the reader already backed out
          // would leave a mailbox connected against a "no" the panel already
          // promised.
          disabled={connect.isPending}
          onClick={onDismiss}
        >
          {t("ob.s4.notNow")}
        </Button>
        <Button
          variant="primary"
          disabled={connect.isPending}
          onClick={() => connect.mutate()}
        >
          {connect.isPending ? (
            <>
              <span className="ob-spinner" /> {t("ob.s4.connecting")}
            </>
          ) : (
            <>
              <Mail aria-hidden /> {t(copy.btn)}
            </>
          )}
        </Button>
      </div>
    </>
  );
}

// Post-consent: the roster row IS the proof a connection happened — never a
// static claim the server hasn't confirmed. The import offered next belongs to
// the mailbox that just connected, so the returning provider is matched
// exactly: the roster is provider-ordered, and taking whichever OAuth row
// comes first would offer to import Gmail after a Microsoft consent.
export function OAuthReturnPanel({
  outcome,
  provider,
  onDone,
}: Readonly<{
  outcome?: string;
  /** The provider the consent returned for, from the deep-link route. */
  provider?: string;
  /** Closes the result. The mailbox question is answered by the roster the
   * connect surface reads, and leaving the STEP is that surface's own way
   * onward — so a reader who has just connected mail is back among the cards,
   * LinkedIn included, rather than carried out of the step. */
  onDone: () => void;
}>) {
  const t = useT();
  // The shared read, held until the return says "ok". Every reader of this path
  // (features/capture/connectors.tsx) shares one cache entry. So this panel, the
  // connect scene's cards and the shell's import chip all read the same roster
  // of connected mailboxes.
  const connections = useConnectors({ enabled: outcome === "ok" });
  const returning = asOAuthProvider(provider);
  // Raised while the posture write is in flight. The backread is what reads a
  // year of mail, so starting one before the answer commits imports under
  // whichever of the two wins.
  const [postureSaving, setPostureSaving] = useState(false);
  // A segment this build cannot resolve to a provider names no mailbox, and
  // falling back would offer the import for one the human did not just connect.
  // That is precisely the failure the exact match exists to prevent, so it lands
  // on the confirm-failure state instead of guessing. An ABSENT segment is a
  // different fact — a landing URL minted before the provider rode the route —
  // and the roster's first live OAuth mailbox is the best answer there.
  const unresolvedProvider = provider !== undefined && returning === null;
  // Every outcome besides a resolved "ok" is `undefined` roster data away
  // from ever finding a live row, so `live` stays safely undefined for them
  // rather than throwing.
  const live = connections.data?.data.find((c) =>
    returning === null
      ? asOAuthProvider(c.provider) !== null && c.status === "connected"
      : c.provider === returning && c.status === "connected",
  );
  if (outcome === "denied") {
    return (
      <ConnectWarn
        title={t("ob.s4.connectDenied")}
        body={t("ob.s4.connectRetry")}
      />
    );
  }
  // Onboarding is the DEFAULT return surface, so it sees the same server
  // outcome enum Settings does and must handle all of it: an outcome only one
  // renderer knows about falls through to the other's generic advice. These two
  // failures are permanent, so neither may repeat connectRetry's "try again" —
  // they reuse the Settings wording rather than minting a second copy of it.
  // Object.hasOwn, not a bare index: a route segment like "constructor" would
  // otherwise resolve to an inherited member and render an empty banner.
  const permanentBody =
    outcome && Object.hasOwn(PERMANENT_FAILURE_BODY, outcome)
      ? PERMANENT_FAILURE_BODY[outcome]
      : undefined;
  if (permanentBody) {
    return (
      <ConnectWarn
        title={t("ob.s4.connectConfirmFailed")}
        body={t(permanentBody)}
      />
    );
  }
  if (outcome !== "ok" || unresolvedProvider) {
    return (
      <ConnectWarn
        title={t("ob.s4.connectConfirmFailed")}
        body={t("ob.s4.connectRetry")}
      />
    );
  }
  return (
    <div className="connect-result">
      <div className="cr-h">
        <CircleCheck aria-hidden /> {t("ob.s4.connectOkTitle")}
      </div>
      <p className="ob-sub">{t("ob.s4.connectOkBody")}</p>
      {connections.isPending && (
        <p className="t-caption">{t("ob.s4.connectVerifying")}</p>
      )}
      {live && (
        <>
          <Badge tone="success" icon={ShieldCheck}>
            {t("ob.s4.connectLive")}
          </Badge>
          {/* Asked BEFORE the backread, because the backread is what reads a
              year of mail: a posture chosen after it has already let every
              captured message in under the previous answer. */}

          {/* Only when the return NAMED its provider. With the segment absent
              `live` is the roster's first live OAuth mailbox — a good enough
              guess for the backread, which offers to read a mailbox and can be
              declined, and the wrong basis for this: writing a posture to a
              guessed row silently changes who may read a DIFFERENT inbox. The
              posture is then left to Settings, where the row is named. */}
          {returning !== null && (
            <ConnectPostureStep
              provider={live.provider}
              posture={live.mail_posture ?? undefined}
              onPendingChange={setPostureSaving}
            />
          )}
          {/* The mailbox is live, so the step is not finished yet: how far back
              to read it is the next question, and the backread owns the exit
              from here — its own leave controls finish onboarding, whether or
              not a read is running. */}
          <OnboardingBackread
            provider={live.provider}
            initial={live.backfill}
            disabled={postureSaving}
            onDone={onDone}
          />
        </>
      )}
      {!connections.isPending && !live && (
        <ConnectWarn
          title={t("ob.s4.connectConfirmFailed")}
          body={t("ob.s4.connectRetry")}
        />
      )}
      {/* A roster that ANSWERED and showed no connected mailbox is a fact: the
        consent came back and nothing is connected, which the cards behind this
        dialog already show. A roster that FAILED is not a fact in either
        direction, so this exit is withheld: the reader closes the dialog by
        its own chrome and the surface's roster failure says what to do. */}
      {!connections.isPending && !connections.isError && live === undefined && (
        <Button variant="primary" onClick={onDone}>
          {t("ob.conv.connect.dialogDone")} <ArrowRight aria-hidden />
        </Button>
      )}
    </div>
  );
}
