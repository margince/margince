// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LogOut } from "lucide-react";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { useLogout } from "../../lib/session";

/** The way out of an unfinished setup. The gate keeps every other screen
 *  away until the journey is done, so this stands in the band. */
export function OnboardingSignOut() {
  const t = useT();
  const logout = useLogout();
  return (
    <>
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
