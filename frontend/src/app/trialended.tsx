// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { AuthCardTitle, AuthExperience } from "../components/auth-core";
import { usePageTitle, Wordmark } from "../components/auth-identity";
import { TrialContactDialog } from "../components/trialcontact";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { useCan } from "../lib/capability";
import {
  useLicenseEntitlement,
  useTrialStanding,
} from "../lib/license-posture";
import { useLogout } from "../lib/session";
import "../components/auth-core.css";

/**
 * What every seat sees once the installation's trial has ended. The server
 * refuses every change by then; this screen says why instead of letting each
 * screen fail on its own, and it holds the one way out: the email to the
 * Margince team. An admin is also told where the license goes.
 */
export function TrialEndedScreen() {
  const t = useT();
  usePageTitle(t("trial.locked.pageTitle"));
  const admin = useCan("license", "read");
  const standing = useTrialStanding();
  const entitlement = useLicenseEntitlement(admin);
  const logout = useLogout();
  const [writing, setWriting] = useState(false);
  return (
    <AuthExperience phase="unavailable">
      <Wordmark alt={t("auth.title")} />
      <section className="auth-card">
        <AuthCardTitle>{t("trial.locked.title")}</AuthCardTitle>
        <p>{t("trial.locked.body")}</p>
        <p>{t(admin ? "trial.locked.admin" : "trial.locked.member")}</p>
        <div className="auth-actions">
          <Button variant="primary" onClick={() => setWriting(true)}>
            {t("trial.contact.title")}
          </Button>
          <Button
            variant="ghost"
            pending={logout.isPending}
            onClick={() => logout.mutate()}
          >
            {t("auth.signOut")}
          </Button>
        </div>
        <ErrorLine error={logout.error} />
      </section>
      {writing && (
        <TrialContactDialog
          standing={standing}
          seatsUsed={entitlement.data?.seats_used}
          onClose={() => setWriting(false)}
        />
      )}
    </AuthExperience>
  );
}
