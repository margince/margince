import { useMutation } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { type FormEvent, useState } from "react";
import { api } from "../../api/client";
import { AuthCardTitle } from "../../components/auth-core";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { usePasswordReveal } from "../../design-system/passwordreveal";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isTooShort, MIN_PASSWORD } from "./passwordrule";

/*
 * Why a reset failure has to be classified at all.
 *
 * In the backend, identity/reset.go supersedes the outstanding token when it
 * mints a new link. So "request a new link" is right for a spent token and
 * harmful for anything else. A user whose request hit a network blip would
 * destroy a good link.
 *
 * `token` is the only failure where a new link is the answer. A refused password
 * belongs to the field, and a budget refusal is a wait. A server or transport
 * fault is neither the link's fault nor the user's.
 */
type ResetFailure = "token" | "password" | "rate-limited" | "server";

class ResetError extends Error {
  readonly failure: ResetFailure;
  constructor(failure: ResetFailure) {
    super(failure);
    this.name = "ResetError";
    this.failure = failure;
  }
}

function resetFailureOf(status: number | undefined): ResetFailure {
  // 401 is the only token verdict, and it is one verdict by design. The server
  // does not distinguish unknown, used and expired, so a token cannot be probed.
  // That is why the copy names all three.
  if (status === 401) return "token";
  if (status === 422) return "password";
  if (status === 429) return "rate-limited";
  return "server";
}

function resetErrorKey(failure: ResetFailure): MessageKey {
  if (failure === "token") return "auth.resetFailed";
  if (failure === "password") return "auth.resetRejectedPassword";
  // Not auth.errRateLimited: that one says "sign-in attempts", and this user is
  // setting a password. Copy that names the wrong action reads as the wrong error.
  if (failure === "rate-limited") return "auth.resetRateLimited";
  return "auth.resetServerFailed";
}

export function ResetForm({
  token,
  onDone,
  onRestart,
  selfServiceAvailable,
}: Readonly<{
  token: string;
  onDone: () => void;
  onRestart: () => void;
  // Whether the forgot-password flow exists at all. A token-bearing link is
  // redeemable on an installation with no outbound email (the admin issues it
  // by hand), but self-service recovery there is not.
  selfServiceAvailable: boolean;
}>) {
  const t = useT();
  const [password, setPassword] = useState("");
  const reveal = usePasswordReveal({
    show: t("auth.showPassword"),
    hide: t("auth.hidePassword"),
  });

  const reset = useMutation({
    mutationFn: async () => {
      // `.catch(() => null)` for the same reason LoginForm does it: a rejected
      // fetch (offline, DNS, CORS) is not an HTTP error and arrives as a thrown
      // TypeError. Without this, a transport fault looks like a dead token, and
      // the remedy offered for a dead token destroys a live one.
      const result = await api
        .POST("/auth/reset-password", {
          body: { token, new_password: password },
        })
        .catch(() => null);
      if (!result) {
        throw new ResetError("server");
      }
      if (result.error) {
        throw new ResetError(resetFailureOf(result.response?.status));
      }
    },
    onSuccess: onDone,
  });
  const failure =
    reset.error instanceof ResetError ? reset.error.failure : "server";

  // Code points, through the shared rule, so this screen and the change-password
  // card agree on what a character is. `password.length` counts UTF-16 units, so
  // an emoji password could clear the floor here and be refused there.
  const tooShort = isTooShort(password);
  const ready = password !== "" && !tooShort;
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (ready && !reset.isPending) {
      reset.mutate();
    }
  };

  return (
    <form className="auth-card" onSubmit={submit}>
      <AuthCardTitle>{t("auth.resetTitle")}</AuthCardTitle>
      <p>{t("auth.resetSub")}</p>
      <div className="auth-fields">
        <Field
          label={t("auth.newPassword")}
          icon={<Lock aria-hidden />}
          error={tooShort ? t("password.tooShort") : undefined}
          // The rule, until the rule is being broken. Then the refusal restates
          // it in the danger tone, and a grey copy underneath is noise.
          hint={tooShort ? undefined : t("auth.passwordHint")}
          trailing={reveal.trailing}
        >
          {(control) => (
            <TextInput
              {...control}
              type={reveal.type}
              required
              minLength={MIN_PASSWORD}
              name="new-password"
              autoComplete="new-password"
              autoCapitalize="none"
              spellCheck={false}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
          )}
        </Field>
      </div>
      {reset.isError && (
        <div className="auth-error" role="alert">
          <p className="ae-t">{t(resetErrorKey(failure))}</p>
          {/* The new-link offer appears only for a token verdict. Everywhere else
              it is the one action that makes things worse: it invalidates the
              token the user is still holding, which for a network blip or a
              refused password is a working link thrown away. */}
          {failure === "token" &&
            (selfServiceAvailable ? (
              <button type="button" className="auth-link" onClick={onRestart}>
                {t("auth.requestNewLink")}
              </button>
            ) : (
              // With no outbound email there is no self-service flow. An admin
              // issued this link by hand, so the next step is to ask that admin.
              // "Request a new link" would route into a flow that answers 501.
              // The capability probe prevents that same affordance on login.
              <p>{t("auth.askAdminForNewLink")}</p>
            ))}
        </div>
      )}
      <div className="auth-actions">
        <Button
          type="submit"
          variant="primary"
          disabled={!ready || reset.isPending}
        >
          {t("auth.setNewPassword")}
        </Button>
      </div>
    </form>
  );
}
