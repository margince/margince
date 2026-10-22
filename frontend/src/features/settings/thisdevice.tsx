// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { Button } from "../../design-system/atoms";
import { SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { logUnexpectedError } from "../../lib/problem";
import type { InstallOutcome, InstallState } from "../../lib/pwa";

/** The install row, absent where the browser cannot install: a capability
 *  this device lacks, not a refusal. */
export function InstallSettingRow({
  install,
}: Readonly<{ install: InstallState }>) {
  if (install.kind === "unavailable") {
    return null;
  }
  return <InstallRow install={install} />;
}

function InstallRow({
  install,
}: Readonly<{ install: Exclude<InstallState, { kind: "unavailable" }> }>) {
  const t = useT();
  const standIn = useRef<HTMLSpanElement>(null);
  const shown = useRef(install.kind);
  // Focus that fell with the Install button goes to what took its place, never
  // to <body>; focus the reader put anywhere else stays there.
  useEffect(() => {
    const buttonWent =
      shown.current === "available" && install.kind !== "available";
    shown.current = install.kind;
    const active = document.activeElement;
    if (buttonWent && (active === null || active === document.body)) {
      standIn.current?.focus();
    }
  }, [install.kind]);
  const label = t("settings.installApp");
  switch (install.kind) {
    case "available":
      return (
        <SettingRow
          label={label}
          description={t("settings.installAppHelp")}
          control={<InstallButton prompt={install.prompt} />}
        />
      );
    case "dismissed":
      return (
        <SettingRow
          label={label}
          description={
            <span ref={standIn} tabIndex={-1}>
              {t("settings.installAppDismissed")}
            </span>
          }
          control={null}
        />
      );
    case "manual-ios":
      return (
        <SettingRow
          label={label}
          description={t("settings.installAppManual")}
          control={null}
        />
      );
    case "installed":
      return (
        <SettingRow
          label={label}
          value={
            <span ref={standIn} tabIndex={-1}>
              {t("settings.installAppInstalled")}
            </span>
          }
          control={null}
        />
      );
  }
}

function InstallButton({
  prompt,
}: Readonly<{ prompt: () => Promise<InstallOutcome> }>) {
  const t = useT();
  const [asking, setAsking] = useState(false);
  // Called inside the click: the browser opens its dialog only for a user gesture.
  const ask = () => {
    setAsking(true);
    prompt().then(
      () => setAsking(false),
      (reason: unknown) => {
        setAsking(false);
        logUnexpectedError(reason);
      },
    );
  };
  return (
    <Button pending={asking} onClick={ask}>
      {t("settings.installAppAction")}
    </Button>
  );
}
