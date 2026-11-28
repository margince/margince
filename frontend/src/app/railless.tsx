// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";

// The rail-less page frame, the shape Shell renders for onboarding and booking.
// The pre-session screens and an embedded unit screen get the app background
// and scroll container from it.
export function RaillessFrame({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <div className="app railless">
      <main className="main">
        <div className="scroll">{children}</div>
      </main>
    </div>
  );
}
