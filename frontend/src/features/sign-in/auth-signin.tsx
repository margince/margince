// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { type FormEvent, useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { navigate } from "../../lib/router";
import { AuthCardTitle, type AuthPhase } from "./auth-core";
import { enteredCode, MfaCodeField } from "./mfacode";

// A refused credential never says which half was wrong, and a status decides
// the message; detail strings are never parsed.
type LoginFailure = "credentials" | "rate-limited" | "unreachable";

export class LoginError extends Error {
  readonly failure: LoginFailure;
  constructor(failure: LoginFailure) {
    super(failure);
    this.name = "LoginError";
    this.failure = failure;
  }
}

export function loginFailureOf(status: number): LoginFailure {
  if (status === 401) return "credentials";
  if (status === 429) return "rate-limited";
  return "unreachable";
}

export function loginErrorKey(
  error: unknown,
  credentialsKey: MessageKey = "auth.errCredentials",
): MessageKey {
  const failure = error instanceof LoginError ? error.failure : "unreachable";
  if (failure === "credentials") return credentialsKey;
  if (failure === "rate-limited") return "auth.errRateLimited";
  return "auth.errUnreachable";
}

// A deep link the user followed stays; only a bare entry lands on the Brief.
export function landSignedIn(onPhase: (phase: AuthPhase) => void) {
  onPhase("success");
  const hash = globalThis.location?.hash ?? "";
  if (!hash || hash === "#" || hash === "#/") {
    navigate({ screen: "home" });
  }
}

// The one field takes either factor the server accepts: an authenticator code
// or a recovery code, which is case-sensitive base64url, so letters stay as typed.
export function MfaForm({
  challenge,
  onAuthed,
  onPhase,
  onBack,
}: Readonly<{
  challenge: string;
  onAuthed: () => void | Promise<void>;
  onPhase: (phase: AuthPhase) => void;
  onBack: () => void;
}>) {
  const t = useT();
  const [code, setCode] = useState("");
  const codeRef = useRef<HTMLInputElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    codeRef.current?.focus();
  }, []);

  const verify = useMutation({
    mutationFn: async (attempt: { challenge: string; code: string }) => {
      const result = await api
        .POST("/auth/mfa", {
          body: { mfa_challenge: attempt.challenge, code: attempt.code },
        })
        .catch(() => null);
      if (!result) {
        throw new LoginError("unreachable");
      }
      if (result.error) {
        throw new LoginError(loginFailureOf(result.response.status));
      }
      await onAuthed();
    },
    onSuccess: () => landSignedIn(onPhase),
    onError: (error) => {
      onPhase("error");
      if (error instanceof LoginError && error.failure === "credentials") {
        setCode("");
      }
      requestAnimationFrame(() => errorRef.current?.focus());
    },
  });

  const entered = enteredCode(code);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (entered !== "" && !verify.isPending) {
      onPhase("signing-in");
      verify.mutate({ challenge, code: entered });
    }
  };

  return (
    <form className="auth-card" onSubmit={submit}>
      <AuthCardTitle>{t("auth.mfaTitle")}</AuthCardTitle>
      <p>{t("auth.mfaSub")}</p>
      <div className="auth-fields">
        <MfaCodeField
          ref={codeRef}
          label={t("auth.mfaCode")}
          icon={<KeyRound aria-hidden />}
          value={code}
          onChange={setCode}
        />
      </div>
      {verify.isError && (
        <div className="auth-error" role="alert" tabIndex={-1} ref={errorRef}>
          <p className="ae-t">
            {t(loginErrorKey(verify.error, "auth.mfaErrCode"))}
          </p>
        </div>
      )}
      <div className="auth-actions">
        <Button
          type="submit"
          variant="primary"
          pending={verify.isPending}
          busyLabel={t("auth.signingIn")}
        >
          {t("auth.mfaVerify")}
        </Button>
        <button type="button" className="auth-link" onClick={onBack}>
          {t("auth.backToLogin")}
        </button>
      </div>
    </form>
  );
}
