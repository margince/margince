// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LogOut } from "lucide-react";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { CORE_RELEASE, comparableRelease, SPA_RELEASE } from "../lib/release";
import { useLogout } from "../lib/session";
import type { RowSeat } from "./accountseats";

// The sign-out row, and the build this bundle was made from. The help menu
// prints the build, and a problem report names its release.

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
 * What this bundle knows of its build. A stamped core release marks an instance
 * build, whose own release may still be unstamped.
 */
export type BuildRelease =
  | Readonly<{ kind: "instance"; core: string; instance?: string }>
  | Readonly<{ kind: "core"; release: string }>
  | Readonly<{ kind: "unstamped" }>;

export function buildRelease(): BuildRelease {
  if (comparableRelease(CORE_RELEASE)) {
    return {
      kind: "instance",
      core: CORE_RELEASE,
      instance: comparableRelease(SPA_RELEASE) ? SPA_RELEASE : undefined,
    };
  }
  return comparableRelease(SPA_RELEASE)
    ? { kind: "core", release: SPA_RELEASE }
    : { kind: "unstamped" };
}

/**
 * The release a problem report names. The tracker is the core's, so an instance
 * build names the core release it pins.
 */
export function reportedRelease(build: BuildRelease): string | undefined {
  switch (build.kind) {
    case "instance":
      return build.core;
    case "core":
      return build.release;
    default:
      return undefined;
  }
}

/**
 * The build, so a reader reporting a problem can say which one they are on. A
 * statement, not a row: `role="none"` keeps it out of the menu's items. An
 * instance build names both repositories, even when they agree.
 */
export function AccountRelease() {
  const t = useT();
  const build = buildRelease();
  const lines: string[] = [];
  if (build.kind === "instance") {
    if (build.instance) {
      lines.push(t("shell.instanceVersion", { version: build.instance }));
    }
    lines.push(t("shell.coreVersion", { version: build.core }));
  } else if (build.kind === "core") {
    lines.push(t("shell.version", { version: build.release }));
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
