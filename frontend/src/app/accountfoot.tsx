// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LogOut } from "lucide-react";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { CORE_RELEASE, comparableRelease, SPA_RELEASE } from "../lib/release";
import { useLogout } from "../lib/session";
import type { RowSeat } from "./accountseats";

// The way out of the account menu, and the release the reader is on.
// Apart from account.tsx so the menu's roving walk and its rows read on their own.

/**
 * The way out, with the guard against a second POST while the first is in
 * flight.
 */
export function SignOutRow({ seat }: Readonly<{ seat: RowSeat }>) {
  const t = useT();
  const logout = useLogout();
  return (
    <>
      <button
        type="button"
        className="acctrow"
        role="menuitem"
        {...seat}
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
 * The release a problem report names. The tracker is the core's, so an instance
 * build names the core release it pins rather than its own.
 */
export function reportedRelease(): string | undefined {
  if (comparableRelease(CORE_RELEASE)) {
    return CORE_RELEASE;
  }
  return comparableRelease(SPA_RELEASE) ? SPA_RELEASE : undefined;
}

/**
 * The releases this bundle was built from, so a reader reporting a problem can
 * say which build it is in. A statement, not a row: `role="none"` keeps it out
 * of the menu's items and out of the roving tabstop. A stamped core release
 * marks an instance build, which names both repositories even when they agree.
 * An unstamped build says so rather than printing a version that is not one.
 */
export function AccountRelease() {
  const t = useT();
  const lines: string[] = [];
  if (comparableRelease(CORE_RELEASE)) {
    if (comparableRelease(SPA_RELEASE)) {
      lines.push(t("shell.instanceVersion", { version: SPA_RELEASE }));
    }
    lines.push(t("shell.coreVersion", { version: CORE_RELEASE }));
  } else if (comparableRelease(SPA_RELEASE)) {
    lines.push(t("shell.version", { version: SPA_RELEASE }));
  } else {
    lines.push(t("shell.help.devBuild"));
  }
  return (
    <div className="acctversion t-caption" role="none">
      {lines.map((line) => (
        <div key={line}>{line}</div>
      ))}
    </div>
  );
}
