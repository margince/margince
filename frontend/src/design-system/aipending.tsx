// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Sparkles } from "lucide-react";
import { PendingBody } from "./atoms";
import "./aipending.css";

/**
 * AiPending is the wait while Margince reads or writes. Specks drift where the
 * answer will go, and `AiReveal` carries it in when it lands.
 *
 * `label` is shown as well as spoken. A cold model call can take twenty
 * seconds, and a mute grey block that long looks broken.
 */
export function AiPending({
  label,
  lines = 3,
}: Readonly<{
  label: string;
  // How many rows of content will stand here once the read answers — a
  // height reservation, so the card does not jump when the findings land.
  lines?: number;
}>) {
  return (
    <div className="aipending">
      <span className="aipending-tile" aria-hidden="true">
        <Sparkles />
      </span>
      <div className="aipending-body">
        <PendingBody label={label} lines={lines} visible />
      </div>
    </div>
  );
}
