// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { AuthCardTitle, AuthExperience } from "../components/auth-core";
import { usePageTitle, Wordmark } from "../components/auth-identity";
import { TrialContactDialog } from "../components/trialcontact";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import {
  LicenseCheckResult,
  useLicenseCheck,
} from "../features/settings/license";
import { useT } from "../i18n";
import { useCan, useCanWrite } from "../lib/capability";
import {
  useLicenseEntitlement,
  useTrialStanding,
} from "../lib/license-posture";
import { useLogout } from "../lib/session";
import "../components/auth-core.css";
import "./trialended.css";

/**
 * What every seat sees once the installation's trial has ended. The server
 * refuses every change by then, and this screen says why. It holds the one
 * way out, the message to the Margince team. An admin is also told where the
 * license goes, and one who may change the installation can check it again.
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
    <AuthExperience phase="unavailable" notice>
      <Wordmark alt={t("auth.title")} />
      <section className="auth-card">
        <AuthCardTitle as="h1">{t("trial.locked.title")}</AuthCardTitle>
        <p>{t("trial.locked.body")}</p>
        {!admin && <p>{t("trial.locked.member")}</p>}
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
        {admin && <LicenseKeyHelp />}
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

/** Where an admin's license key goes, set apart from the way out above it. */
function LicenseKeyHelp() {
  const t = useT();
  const heading = useId();
  const canCheck = useCanWrite("installation_settings", "update");
  const check = useLicenseCheck();
  return (
    <section className="trialended-key" aria-labelledby={heading}>
      <Heading size="small" as="h2" id={heading}>
        {t("trial.locked.adminTitle")}
      </Heading>
      <p>{t("trial.locked.admin")}</p>
      {canCheck && (
        <Button
          variant="link"
          onClick={() => check.mutate()}
          pending={check.isPending}
        >
          {t("license.check.button")}
        </Button>
      )}
      <LicenseCheckResult check={check} />
    </section>
  );
}
