// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LogOut } from "lucide-react";
import { useExitAsking } from "../components/unsaved";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { useLogout } from "../lib/session";
import type { RowSeat } from "./accountseats";

/**
 * The way out, with the guard against a second POST while the first is in
 * flight.
 */
export function SignOutRow({ seat }: Readonly<{ seat: RowSeat }>) {
  const t = useT();
  const logout = useLogout();
  const askFirst = useExitAsking();
  return (
    <>
      <button
        type="button"
        className="acctrow"
        role="menuitem"
        {...seat}
        disabled={logout.isPending}
        onClick={() => askFirst(() => logout.mutate())}
      >
        <LogOut size={15} aria-hidden />
        {t("shell.signOutAria")}
      </button>
      {/* A refused sign-out is the one failure here a reader must not have to
          infer. The row re-enables when the request settles either way, so
          without this a session that is still open looks exactly like one that
          has ended — and the next thing the reader does, they do believing they
          have signed out. `role="none"`: a menu's children are its items, and an
          alert is not one. */}
      {logout.isError && (
        <div className="acctrowalert" role="none">
          <Callout tone="danger" kind="outcome" title={t("shell.signOutErr")}>
            {problemMessageOf(logout.error, t)}
          </Callout>
        </div>
      )}
    </>
  );
}
