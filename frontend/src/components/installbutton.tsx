// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button, type ButtonVariant } from "../design-system/atoms";
import { useT } from "../i18n";
import { logUnexpectedError } from "../lib/problem";
import type { InstallOutcome } from "../lib/pwa";

/** Install, with the browser's answer for a caller that keeps its own schedule. */
export function InstallButton({
  prompt,
  onAnswered,
  variant,
}: Readonly<{
  prompt: () => Promise<InstallOutcome>;
  onAnswered?: (outcome: InstallOutcome) => void;
  variant?: Extract<ButtonVariant, "primary" | "ghost">;
}>) {
  const t = useT();
  const [asking, setAsking] = useState(false);
  // Called inside the click: the browser opens its dialog only for a user gesture.
  const ask = () => {
    setAsking(true);
    prompt().then(
      (outcome) => {
        setAsking(false);
        onAnswered?.(outcome);
      },
      (reason: unknown) => {
        setAsking(false);
        logUnexpectedError(reason);
      },
    );
  };
  return (
    <Button variant={variant} pending={asking} onClick={ask}>
      {t("settings.installAppAction")}
    </Button>
  );
}
