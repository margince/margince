import { Fragment, useEffect, useState } from "react";
import {
  previewedOidcProviders,
  previewedPasswordReset,
  previewedUnavailableProviders,
  uiPreviewOidcEnabled,
} from "../../app/ui-preview";
import wordmarkDark from "../../assets/wordmark-dark.png";
import wordmarkWhite from "../../assets/wordmark-white.png";
import {
  AuthCardTitle,
  AuthExperience,
  type AuthPhase,
} from "../../components/auth-core";
import { Button } from "../../design-system/atoms";
import { providerBrandName } from "../../design-system/provider-mark";
import { LOCALES, localeNameKey, useLocale, useT } from "../../i18n";
import { useAuthCapabilities } from "../../lib/capabilities";
import { useOutageRecovery } from "../../lib/connectivity";
import { OIDC_FAILURE_HASH, takeReturnRoute } from "../../lib/returnroute";
import { forgetHashCredential, takeHashCredential } from "../../lib/router";
import { sessionAnswers } from "../../lib/session";
import { FederatedOnlyCard } from "./auth-providers";
import { MfaForm } from "./auth-signin";
import "./auth.css";

// The default unauthenticated screen is LOGIN, not setup or signup
// (ADR-0061): one installation serves one company, provisioned at
// API boot from the deployment file — the browser never creates a tenant and
// never selects one. The Margince Core introduces the AI beside a real <form>
// (Enter submits), and only the authentication methods the capabilities probe
// reports as operational render: the forgot-password flow appears exactly when
// the server can complete it.
import { ForgotForm, LoginForm, Notice } from "./auth-login";
import { ResetForm } from "./auth-reset";

// AuthNotice is the boundary's transient context for the login screen: a
// deliberate sign-out, an expired session, or a federated sign-in that did
// not complete — informational, never danger styling (§9.5: the user has
// nothing to correct). The first two arrive as a prop from App's own state;
// "oidc-failed" is read from the address instead (see oidcFailureNotice
// below) because it is the callback redirect's own doing, not something
// App decided.
export type AuthNotice =
  | "signed-out"
  | "session-expired"
  | "oidc-failed"
  | null;

// The OIDC callback's neutral-failure redirect lands on OIDC_FAILURE_HASH
// (cmd/api/googlesignin.go's FailureURL). It is not a Screen this app routes to
// (no "login" entry in lib/router.tsx's SCREENS). The unauthenticated gate in
// App.tsx renders AuthScreen ahead of any route match, whatever the hash names.
// It is consumed the way clearResetHash below scrubs its own marker. It is read
// once on mount, then rewritten out of the address so a reload or a Back press
// cannot replay it.

// isOidcFailureMarker is a PURE read — no mutation — so it is safe under
// React's StrictMode (development only), which double-invokes a useState lazy
// initializer to surface impure ones. An earlier version decided the answer AND
// cleared the address in one impure step: the first invocation cleared it and
// answered true, the second then saw the already-cleared hash and answered
// false — and false is the call whose result actually became the committed
// state, silently dropping the notice on every dev-mode render. Splitting the
// read from the clear (below) removes the impurity instead of working around
// it, so no double-invoke count changes the answer.
function isOidcFailureMarker(): boolean {
  return globalThis.location?.hash === OIDC_FAILURE_HASH;
}

// clearOidcFailureMarker is the side effect, kept OUT of the initializer and
// run from an effect instead — effects are also double-invoked in
// StrictMode (mount, cleanup, mount again), but this one is idempotent: the
// second call finds the marker already gone and does nothing. Guarding on
// isOidcFailureMarker() first (rather than clearing unconditionally) is what
// makes a later, unrelated remount of AuthScreen safe too — by then the
// address has moved on and there is nothing here to touch.
function clearOidcFailureMarker(): void {
  if (!isOidcFailureMarker()) {
    return;
  }
  const { pathname, search } = globalThis.location;
  // The link the reader followed comes back, so a second attempt keeps it.
  const route = takeReturnRoute() ?? "";
  globalThis.history?.replaceState?.(null, "", `${pathname}${search}${route}`);
}

type View =
  | { kind: "login" }
  | { kind: "mfa"; challenge: string }
  | { kind: "forgot" }
  | { kind: "forgot-sent"; email: string }
  | { kind: "reset"; token: string }
  | { kind: "reset-done" };

/**
 * The hash route the emailed reset link lands on. Minted by identity/reset.go.
 * Exported so App.tsx can route this entry through the unauthenticated auth
 * flow even when a session cookie is already live — an existing session must
 * not hide the reset form behind the authenticated shell.
 *
 * The token rides the FRAGMENT (`#/reset-password?token=…`), not the document
 * query, and that is the first defence rather than a formatting choice: a
 * fragment is never sent to a server, so it cannot land in an access log or a
 * Referer header, and it is not part of a Cache Storage key, so nothing that
 * keys on a URL persists it to disk. Parsing it as this app's own hash route
 * also means any static host serves index.html for `/` with no SPA fallback.
 * What is left is the history entry, and lib/router.tsx takes the token out of
 * that before anything renders.
 */
export const RESET_ROUTE = "reset-password";

// clearResetHash drops a lingering `#/reset-password` from the address bar
// once the reset entry is done with. The token itself left with the router
// (lib/router.tsx), but the bare route survives until this runs.
// Left in place, it would make LoginForm's "restore the originally requested
// route" check see a non-empty hash and skip the post-login redirect to
// the Brief, stranding a completed reset on a screen this app never routes to.
// Guarded on the route so it is safe to call from every "back to login" exit,
// including the ones that never touched the reset flow.
function clearResetHash(): void {
  if (typeof globalThis.location === "undefined") {
    return;
  }
  const hash = globalThis.location.hash.replace(/^#\/?/, "").split("?")[0];
  if (hash === RESET_ROUTE) {
    globalThis.history?.replaceState?.(
      null,
      "",
      `${globalThis.location.pathname}${globalThis.location.search}`,
    );
  }
}

// The two sign-in steps drive the Core; every other view leaves it at rest.
function corePhase(view: View, phase: AuthPhase): AuthPhase {
  return view.kind === "login" || view.kind === "mfa" ? phase : "quiet";
}

export function AuthScreen({
  onAuthed,
  notice = null,
}: Readonly<{ onAuthed: () => void | Promise<void>; notice?: AuthNotice }>) {
  const t = useT();
  // The emailed deep link lands wherever the unauthenticated gate renders this
  // screen, so no route table entry is needed. The token is already out of the
  // address bar. lib/router.tsx takes it as it reads the hash, ahead of every
  // gate that can render instead of this screen, and hands it over in memory.
  const [view, setView] = useState<View>(() => {
    const token = takeHashCredential(RESET_ROUTE);
    return token ? { kind: "reset", token } : { kind: "login" };
  });
  // Read once, same instant as the reset token above: both are one-shot markers
  // this screen's own mount is responsible for taking out of the address before
  // anything else reads it. The read itself is pure — see isOidcFailureMarker's
  // comment for why — the clear happens in the effect below.
  const [oidcFailed] = useState(isOidcFailureMarker);
  useEffect(() => {
    if (oidcFailed) {
      clearOidcFailureMarker();
    }
  }, [oidcFailed]);
  const effectiveNotice: AuthNotice =
    notice ?? (oidcFailed ? "oidc-failed" : null);
  // A link pasted into a tab already on this screen changes the hash and
  // nothing else — no remount, so the initializer above never runs again. The
  // one view that sends a reader off to ask for a fresh link leaves them on
  // #/reset-password, which is exactly the tab they paste it into.
  useEffect(() => {
    const onHashChange = () => {
      const token = takeHashCredential(RESET_ROUTE);
      if (token) {
        setView({ kind: "reset", token });
      }
    };
    globalThis.addEventListener("hashchange", onHashChange);
    return () => globalThis.removeEventListener("hashchange", onHashChange);
  }, []);
  // The token is in this screen's state now, so memory empties the way the
  // address did. Memory outlives a mount and the address did not: left there,
  // it would put the reset form back over a reader who had gone back to login.
  // Re-asserted on every view change rather than only on a new token, so the
  // invariant is "the screen holds it, memory does not" rather than "it was
  // emptied once".
  useEffect(() => {
    if (view.kind === "reset") {
      forgetHashCredential(RESET_ROUTE, view.token);
    }
  }, [view]);
  const [authPhase, setAuthPhase] = useState<AuthPhase>("idle");
  usePageTitle(t("auth.pageTitle"));

  // The anonymous capability probe drives what the screen offers — a dead
  // "Forgot password?" link is a misleading affordance, so it renders only
  // when the reset flow can complete end to end. Shared with the release gate
  // above this screen, which reads the api's release off the same answer.
  const capabilities = useAuthCapabilities();
  // The capability's real value, passed through the ONE ui-preview override site
  // for the reset link. Off by default, in which case this is the identity
  // function on the server's own answer.
  const resetAvailable = previewedPasswordReset(
    capabilities.data?.password_reset === true,
  );
  const setLoginView = () => {
    setAuthPhase("idle");
    setView({ kind: "login" });
    clearResetHash();
  };

  // Whether this installation offers email + password at all. An installation
  // that puts an identity provider in front of Margince turns the method off,
  // and then the form below must not render: every route it posts to refuses.
  //
  // Absent an answer it is offered — a probe still in flight, or one that
  // failed, shows the method nearly every installation has rather than a blank
  // card. The server refuses the password routes either way, so the cost of
  // being wrong here is a form that cannot be submitted, never a session.
  const passwordOffered = capabilities.data?.password !== false;

  const servedOidcProviders = capabilities.data?.oidc_providers ?? [];
  // True only when previewedOidcProviders (below) actually invented the list
  // this render draws — never merely because the preview build flag is set. An
  // installation that genuinely serves OIDC providers keeps working buttons
  // even under a preview build: the switch exists to stand in for a server with
  // none configured, not to blanket-disable a real one.
  const oidcProvidersSynthesized =
    servedOidcProviders.length === 0 && uiPreviewOidcEnabled();
  // The server's answer, passed through the ONE ui-preview override site. Off
  // by default, in which case this is the identity function and the
  // capability's real value reaches the card verbatim — the label callback
  // included, which is never consulted on a served provider.
  //
  // Built once for both cards: which of them renders is a question about the
  // password method, and the providers are the same providers either way.
  const federatedProviders = previewedOidcProviders(
    servedOidcProviders,
    (providerKey) =>
      t("auth.continueWith", {
        // The key itself if we have no brand word for it: a preview that
        // invents a company's name is worse than one that shows the operator's
        // own identifier.
        brand: providerBrandName(providerKey) ?? providerKey,
      }),
  );
  // Empty in the product, always: the capability carries no availability field,
  // so only the preview layer can mark a provider (app/ui-preview.ts). Gated on
  // the same served list as `federatedProviders` above, so the marker never
  // falls on a provider this installation genuinely serves.
  const unavailableProviders =
    previewedUnavailableProviders(servedOidcProviders);

  return (
    <AuthExperience phase={corePhase(view, authPhase)}>
      <Wordmark alt={t("auth.title")} />
      {view.kind === "login" && (
        <>
          {effectiveNotice && (
            <p className="auth-notice t-sub" role="status">
              {t(
                effectiveNotice === "signed-out"
                  ? "auth.noticeSignedOut"
                  : effectiveNotice === "session-expired"
                    ? "auth.noticeSessionExpired"
                    : "auth.noticeOidcFailed",
              )}
            </p>
          )}
          {!passwordOffered ? (
            <FederatedOnlyCard
              providers={federatedProviders}
              providersSynthesized={oidcProvidersSynthesized}
              unavailableProviders={unavailableProviders}
            />
          ) : (
            <LoginForm
              onAuthed={onAuthed}
              onPhase={setAuthPhase}
              resetAvailable={resetAvailable}
              providers={federatedProviders}
              providersSynthesized={oidcProvidersSynthesized}
              unavailableProviders={unavailableProviders}
              onForgot={() => setView({ kind: "forgot" })}
              onChallenge={(challenge) => {
                setAuthPhase("idle");
                setView({ kind: "mfa", challenge });
              }}
            />
          )}
        </>
      )}
      {view.kind === "mfa" && (
        <MfaForm
          challenge={view.challenge}
          onAuthed={onAuthed}
          onPhase={setAuthPhase}
          onBack={setLoginView}
        />
      )}
      {view.kind === "forgot" && (
        <ForgotForm
          onSent={(email) => setView({ kind: "forgot-sent", email })}
          onBack={setLoginView}
        />
      )}
      {view.kind === "forgot-sent" && (
        <Notice
          title={t("auth.forgotSentTitle")}
          body={t("auth.forgotSentBody")}
          action={t("auth.backToLogin")}
          onAction={setLoginView}
        />
      )}
      {view.kind === "reset" && (
        <ResetForm
          token={view.token}
          onDone={() => setView({ kind: "reset-done" })}
          onRestart={() => setView({ kind: "forgot" })}
          selfServiceAvailable={resetAvailable}
        />
      )}
      {view.kind === "reset-done" && (
        <Notice
          title={t("auth.resetDoneTitle")}
          body={t("auth.resetDoneBody")}
          action={t("auth.backToLogin")}
          onAction={setLoginView}
        />
      )}
      <LocaleFooter />
    </AuthExperience>
  );
}

// The boundary's non-authentication half (§4): the api cannot be reached or the
// installation is not ready (503), and neither may read as a wrong password.
export function AvailabilityScreen({
  kind,
  onRetry,
}: Readonly<{ kind: "connection" | "installation"; onRetry: () => void }>) {
  const t = useT();
  useOutageRecovery(kind === "connection", sessionAnswers, onRetry);
  usePageTitle(t("auth.pageTitle"));
  return (
    <AuthExperience phase="unavailable">
      <Wordmark alt={t("auth.title")} />
      <section className="auth-card" role="alert">
        <AuthCardTitle>
          {t(
            kind === "connection"
              ? "auth.connectionTitle"
              : "auth.unavailableTitle",
          )}
        </AuthCardTitle>
        <p>
          {t(
            kind === "connection"
              ? "auth.connectionBody"
              : "auth.unavailableBody",
          )}
        </p>
        <div className="auth-actions">
          <Button variant="primary" onClick={onRetry}>
            {t("auth.retry")}
          </Button>
        </div>
      </section>
    </AuthExperience>
  );
}

// Wordmark renders the current Margince logo. Two source images (dark ink for
// the light theme, white for dark) swap via CSS on the data-theme toggle — no
// JS theme read needed. Shared with onboarding, which sizes it for its top bar
// through className: the product has ONE wordmark, and a screen that draws its
// own drifts the moment the real one changes.
export function Wordmark({
  alt,
  className = "auth-wordmark",
}: Readonly<{ alt: string; className?: string }>) {
  // The container carries the ONE accessible name: the theme swap hides
  // one <img> with display:none, so a name on either image alone would
  // vanish in the other theme.
  return (
    <span className={className} role="img" aria-label={alt}>
      <img className="auth-wordmark-light" src={wordmarkDark} alt="" />
      <img className="auth-wordmark-dark" src={wordmarkWhite} alt="" />
    </span>
  );
}

// usePageTitle stamps the document title for the unauthenticated surface
// (§7.1) and restores the product name on unmount.
export function usePageTitle(title: string) {
  useEffect(() => {
    const previous = document.title;
    document.title = title;
    return () => {
      document.title = previous;
    };
  }, [title]);
}

// LocaleFooter is the one footer utility that actually works today (§3.3
// honesty: no Privacy/Help links exist yet, so none render). Language
// names are proper nouns, deliberately not translated — which is exactly why
// each carries its own `lang` below: three languages sit in a document
// declared to be in one, and a screen reader would otherwise read "Tiếng
// Việt" with the phonemes of whichever locale is currently on. Same WCAG
// 3.1.1 rule LocaleProvider keeps for the document; our locale codes are BCP
// 47 language subtags, so the code IS the value `lang` wants.
function LocaleFooter() {
  const t = useT();
  const { locale, setLocale } = useLocale();
  return (
    <div className="auth-footer">
      {LOCALES.map((option, index) => (
        <Fragment key={option}>
          {index > 0 && <span aria-hidden>·</span>}
          <button
            type="button"
            className="auth-link"
            aria-pressed={option === locale}
            onClick={() => setLocale(option)}
          >
            <span lang={option}>{t(localeNameKey(option))}</span>
          </button>
        </Fragment>
      ))}
    </div>
  );
}
