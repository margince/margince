// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LogOut } from "lucide-react";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { problemMessageOf, useLogout } from "../screens/common";
import { comparableRelease, SPA_RELEASE } from "./release";

// The foot of the account menu: the way out, and the release the reader is on.
// Apart from account.tsx so the menu's roving walk and its rows read on their own.

/**
 * What a row needs to be a menu item. It holds the row's roving tabstop, the
 * handle the menu moves focus with, and the callback that records focus.
 *
 * Optional at every call site, because the phone sheet's rows are ordinary
 * content walked with Tab. A `role="menuitem"` there would promise keys that
 * nothing implements.
 */
export type RowSeat = Readonly<{
  ref: (element: HTMLElement | null) => void;
  tabIndex: number;
  onFocus: () => void;
}>;

/**
 * The way out, with the guard against a second POST while the first is in
 * flight. One spelling for the menu and the sheet: two would be two ways to sign
 * out, and only one of them would keep the guard.
 */
export function SignOutRow({ seat }: Readonly<{ seat?: RowSeat }>) {
  const t = useT();
  const logout = useLogout();
  return (
    <>
      <button
        type="button"
        className="acctrow"
        role={seat ? "menuitem" : undefined}
        tabIndex={seat?.tabIndex}
        ref={seat?.ref}
        onFocus={seat?.onFocus}
        disabled={logout.isPending}
        onClick={() => logout.mutate()}
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

/**
 * The release this bundle was built from, so a reader reporting a problem can
 * say which build it is in. A statement, not a row: `role="none"` keeps it out
 * of the menu's items and out of the roving tabstop. An unstamped build prints
 * nothing rather than a version that is not one.
 */
export function AccountRelease() {
  const t = useT();
  if (!comparableRelease(SPA_RELEASE)) {
    return null;
  }
  return (
    <>
      <hr />
      <div className="acctversion t-caption" role="none">
        {t("shell.version", { version: SPA_RELEASE })}
      </div>
    </>
  );
}
