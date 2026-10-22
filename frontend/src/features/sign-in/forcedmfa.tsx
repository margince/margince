// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { AuthCardTitle, AuthExperience } from "../../components/auth-core";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { problemCodeOf } from "../../lib/problem";
import { useLogout } from "../../lib/session";
import { usePageTitle, Wordmark } from "./auth";
import {
  MfaSetupActions,
  MfaSetupSteps,
  useTotpConfirm,
  useTotpStart,
} from "./mfasetup";
import "./auth.css";

/**
 * The account the installation requires a second factor from, before it holds
 * one. The server answers nothing but the enrolment routes, so this runs the
 * Account page's setup at the boundary.
 */
export function ForcedMfaScreen({
  onEnrolled,
}: Readonly<{ onEnrolled: () => void }>) {
  const t = useT();
  usePageTitle(t("forcedMfa.pageTitle"));
  const formId = useId();
  const start = useTotpStart();
  const flow = useTotpConfirm();
  const logout = useLogout();
  const alreadyOn = [start.error, flow.confirm.error].some(
    (error) => problemCodeOf(error) === "mfa_already_enrolled",
  );
  const begun = start.data !== undefined || start.isPending || start.isError;

  return (
    <AuthExperience phase="unavailable">
      <Wordmark alt={t("auth.title")} />
      <section className="auth-card">
        <AuthCardTitle>{t("forcedMfa.title")}</AuthCardTitle>
        <p>{t("forcedMfa.body")}</p>
        {alreadyOn && (
          <Callout
            kind="outcome"
            tone="info"
            title={t("forcedMfa.alreadyOn")}
            actions={
              <Button variant="primary" onClick={onEnrolled}>
                {t("forcedMfa.continue")}
              </Button>
            }
          />
        )}
        {!alreadyOn && begun && (
          <>
            <MfaSetupSteps
              flow={flow}
              formId={formId}
              enrolment={start.data}
              starting={start.isPending}
              startError={start.error}
              onRetry={() => start.mutate()}
            />
            <MfaSetupActions
              className="auth-actions"
              flow={flow}
              formId={formId}
              enrolment={start.data}
              onDone={onEnrolled}
            />
          </>
        )}
        <div className="auth-actions">
          {!begun && (
            <Button variant="primary" onClick={() => start.mutate()}>
              {t("mfa.setUp")}
            </Button>
          )}
          {!flow.codes && (
            <Button
              variant="ghost"
              disabled={flow.confirm.isPending}
              pending={logout.isPending}
              onClick={() => logout.mutate()}
            >
              {t("auth.signOut")}
            </Button>
          )}
        </div>
        <ErrorLine error={logout.error} />
      </section>
    </AuthExperience>
  );
}
