// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LogOut, UserRound } from "lucide-react";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { routeHash } from "../../lib/router";
import { useLogout } from "../../lib/session";
import { settingsHref } from "../../lib/settingsrouting";

/** The account's two doors out of an unfinished setup. The gate keeps every
 *  other screen away until the journey is done, so both stand in the band. */
export function OnboardingAccount() {
  const t = useT();
  const logout = useLogout();
  return (
    <>
      <AppLink
        className="iconbtn"
        href={routeHash(settingsHref("account"))}
        aria-label={t("settings.tab.account")}
        title={t("settings.tab.account")}
      >
        <UserRound aria-hidden />
      </AppLink>
      <Button
        variant="bare"
        iconOnly
        aria-label={t("auth.signOut")}
        title={t("auth.signOut")}
        pending={logout.isPending}
        onClick={() => logout.mutate()}
      >
        <LogOut aria-hidden />
      </Button>
      <ErrorLine error={logout.error} inline />
    </>
  );
}
