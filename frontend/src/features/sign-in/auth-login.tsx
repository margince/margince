import { useMutation } from "@tanstack/react-query";
import { Lock, Mail } from "lucide-react";
import { type FormEvent, useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { AuthCardTitle, type AuthPhase } from "../../components/auth-core";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { usePasswordReveal } from "../../design-system/passwordreveal";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import {
  type OidcProviders,
  ProviderButtons,
  startFederatedSignIn,
} from "./auth-providers";
import {
  LoginError,
  landSignedIn,
  loginErrorKey,
  loginFailureOf,
} from "./auth-signin";

export function LoginForm({
  onAuthed,
  onPhase,
  resetAvailable,
  providers,
  providersSynthesized,
  unavailableProviders,
  onForgot,
  onChallenge,
}: Readonly<{
  onAuthed: () => void | Promise<void>;
  onPhase: (phase: AuthPhase) => void;
  resetAvailable: boolean;
  /** Served by /auth/capabilities. Empty means no federated block. */
  providers: OidcProviders;
  /** True only when `providers` above was invented by the preview layer
   * rather than served by the capability. See `startFederatedSignIn`. */
  providersSynthesized: boolean;
  /** Preview-only; empty in the product. See `ProviderButtons`. */
  unavailableProviders: ReadonlySet<string>;
  onForgot: () => void;
  onChallenge: (challenge: string) => void;
}>) {
  const t = useT();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [capsLock, setCapsLock] = useState(false);
  const reveal = usePasswordReveal({
    show: t("auth.showPassword"),
    hide: t("auth.hidePassword"),
  });
  const emailRef = useRef<HTMLInputElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);

  // Focus lands on email at render. It is programmatic rather than the
  // autoFocus attribute, so the a11y lint's blanket rule stays intact.
  // The login page keeps the one justified exception.
  useEffect(() => {
    emailRef.current?.focus();
  }, []);

  const login = useMutation({
    mutationFn: async () => {
      const result = await api
        .POST("/auth/login", { body: { email: email.trim(), password } })
        .catch(() => null);
      if (!result) {
        throw new LoginError("unreachable");
      }
      if (result.error) {
        throw new LoginError(loginFailureOf(result.response.status));
      }
      // 202 sets no session: the password was right and a second factor is owed.
      if (result.response.status === 202) {
        if (!result.data || !("mfa_challenge" in result.data)) {
          throw new LoginError("unreachable");
        }
        return { challenge: result.data.mfa_challenge };
      }
      // The login response only says the credential exchange succeeded. The
      // session is real when the app's authenticated /me probe accepts the
      // resulting cookie; keep the Core in its signing-in state until then.
      await onAuthed();
      return null;
    },
    onSuccess: (pending) =>
      pending ? onChallenge(pending.challenge) : landSignedIn(onPhase),
    onError: (error) => {
      onPhase("error");
      if (error instanceof LoginError && error.failure === "credentials") {
        // A rejected credential clears the password; the email stays for
        // the retry.
        setPassword("");
      }
      // The error summary is announced and receives focus; tab order then
      // leads back into the fields.
      requestAnimationFrame(() => errorRef.current?.focus());
    },
  });

  const ready = email.trim() !== "" && password !== "";
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (ready && !login.isPending) {
      onPhase("signing-in");
      login.mutate();
    }
  };

  return (
    <form className="auth-card" onSubmit={submit}>
      {/* The page says its name once. The greeting beside this form is the
          page's h1, so the form's own title and sub-line would repeat it in a
          larger voice than the sentence they follow. Both stay in the
          accessibility tree, where a form still wants its name, and leave the
          composition. The other views keep theirs visible: nothing above a
          password reset form says what it is. */}
      <AuthCardTitle className="sr-only">{t("auth.loginTitle")}</AuthCardTitle>
      <p className="sr-only">{t("auth.loginSub")}</p>
      <ProviderButtons
        providers={providers}
        disabled={login.isPending}
        unavailable={unavailableProviders}
        onSelect={(providerKey) =>
          startFederatedSignIn(providerKey, providersSynthesized)
        }
      />
      {/* Visible labels, which diverge from the reference artifact. It names
          its fields with a placeholder and an aria-label, and a placeholder is
          not a label: it vanishes once the field has content (WCAG 3.3.2).
          ADR-0076 binds the WCAG rules unamended, so where the picture and
          WCAG disagree, WCAG wins. */}
      <div className="auth-fields">
        <Field label={t("auth.email")} icon={<Mail aria-hidden />}>
          {(control) => (
            <TextInput
              {...control}
              ref={emailRef}
              type="email"
              required
              /* A stable `name`, because `id` here cannot be one: Field mints
                 its id with useId(), which derives from the component's
                 position in the tree, and this tree changes with the notice and
                 the provider block. Chrome autofills from the autocomplete
                 token alone, but Firefox and several password managers fall
                 back to name/id to match a SAVED credential to a rendered field
                 — with neither stable, they have nothing to match on. */
              name="email"
              autoComplete="username"
              inputMode="email"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              placeholder={t("auth.emailPlaceholder")}
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          )}
        </Field>
        <Field
          label={t("auth.password")}
          icon={<Lock aria-hidden />}
          labelEnd={
            resetAvailable ? (
              <button type="button" className="auth-link" onClick={onForgot}>
                {t("auth.forgotLink")}
              </button>
            ) : undefined
          }
          hint={capsLock ? t("auth.capsLock") : undefined}
          // Announced when it appears, not only when focus arrives. Caps lock
          // is pressed while typing. A warning heard only after leaving and
          // returning to the field arrives after the password it was about.
          hintLive
          trailing={reveal.trailing}
        >
          {(control) => (
            <TextInput
              {...control}
              type={reveal.type}
              required
              name="password"
              autoComplete="current-password"
              autoCapitalize="none"
              spellCheck={false}
              placeholder={t("auth.passwordPlaceholder")}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              onKeyUp={(event) =>
                setCapsLock(event.getModifierState?.("CapsLock") ?? false)
              }
            />
          )}
        </Field>
      </div>
      {login.isError && (
        <div className="auth-error" role="alert" tabIndex={-1} ref={errorRef}>
          <p className="ae-t">{t(loginErrorKey(login.error))}</p>
        </div>
      )}
      <div className="auth-actions">
        {/* Refuses the press only while a request is in flight. An empty
            field is answered by native validation on the inputs, not by a pale
            control with nothing to say.

            `pending`, not `disabled`: the label stays "Sign in" the whole way
            through, because renaming a control mid-press makes a screen reader
            re-read the control itself. The busy sentence is `busyLabel`, which
            lands in `aria-describedby` instead. The sentence is still spoken,
            and the button keeps the name it had when the reader pressed it. */}
        <Button
          type="submit"
          variant="primary"
          pending={login.isPending}
          busyLabel={t("auth.signingIn")}
        >
          {t("auth.signIn")}
        </Button>
      </div>
    </form>
  );
}

export function ForgotForm({
  onSent,
  onBack,
}: Readonly<{ onSent: (email: string) => void; onBack: () => void }>) {
  const t = useT();
  const [email, setEmail] = useState("");

  const request = useMutation({
    mutationFn: async () => {
      const body = { email: email.trim() };
      unwrap(await api.POST("/auth/forgot-password", { body }));
    },
    onSuccess: () => onSent(email.trim()),
  });

  const ready = email.trim() !== "";
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (ready && !request.isPending) {
      request.mutate();
    }
  };

  return (
    <form className="auth-card" onSubmit={submit}>
      <AuthCardTitle>{t("auth.forgotTitle")}</AuthCardTitle>
      <p>{t("auth.forgotSub")}</p>
      <div className="auth-fields">
        {/* Same icon as the sign-in card's email field. Without it the text
            starts 22px further left than on the screen the user just came from,
            and the two cards stop looking like one surface. */}
        <Field label={t("auth.email")} icon={<Mail aria-hidden />}>
          {(control) => (
            <TextInput
              {...control}
              type="email"
              required
              name="email"
              /* "email", not "username": this form never accepts a password, and
                 labelling it username invites a manager to treat it as a sign-in
                 field and offer to fill a credential that has nowhere to go. */
              autoComplete="email"
              inputMode="email"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              placeholder={t("auth.emailPlaceholder")}
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          )}
        </Field>
      </div>
      {request.isError && (
        <ErrorNote message={problemMessageOf(request.error, t)} />
      )}
      <div className="auth-actions">
        <Button
          type="submit"
          variant="primary"
          disabled={!ready || request.isPending}
        >
          {t("auth.sendResetLink")}
        </Button>
        <button type="button" className="auth-link" onClick={onBack}>
          {t("auth.backToLogin")}
        </button>
      </div>
    </form>
  );
}

export function Notice({
  title,
  body,
  action,
  onAction,
}: Readonly<{
  title: string;
  body: string;
  action: string;
  onAction: () => void;
}>) {
  return (
    <section className="auth-card">
      <AuthCardTitle>{title}</AuthCardTitle>
      <p>{body}</p>
      <div className="auth-actions">
        <Button variant="primary" onClick={onAction}>
          {action}
        </Button>
      </div>
    </section>
  );
}

function ErrorNote({ message }: Readonly<{ message: string | null }>) {
  const t = useT();
  return (
    <div className="auth-error" role="alert">
      <p className="ae-t">{t("auth.failed")}</p>
      {message && <p className="ae-m t-sub">{message}</p>}
    </div>
  );
}
