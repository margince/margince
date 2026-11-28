// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Card } from "../design-system/atoms";
import { Button } from "../design-system/button";
import { OffsiteLink } from "../design-system/offsitelink";
import { useT } from "../i18n";
import { isEmbedAddress } from "../lib/recordaddress";

/** True when this document is a unit's screen inside a frame the operator allows. */
export function isEmbedded(): boolean {
  const { pathname, hash } = globalThis.location;
  return isEmbedAddress(pathname, hash);
}

// The frame's host draws its own chrome, so the screen gets the app's
// background and scroll container and nothing around it.
export function EmbeddedFrame({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <div className="app railless embedded">
      <main className="main">
        <div className="scroll">{children}</div>
      </main>
    </div>
  );
}

// A provider sign-in refuses to run inside a frame, so the reader signs in in
// a tab of this origin and comes back; the session cookie is the same one.
export function EmbedSignIn({ onRetry }: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <EmbeddedFrame>
      <div className="wrap narrow">
        <Card title={t("auth.loginTitle")} level={1}>
          <p>{t("shell.embedSignInBody")}</p>
          <p>
            <OffsiteLink href={`${globalThis.location.origin}/`}>
              {t("shell.embedOpen")}
            </OffsiteLink>
          </p>
          <Button variant="primary" onClick={onRetry}>
            {t("common.retry")}
          </Button>
        </Card>
      </div>
    </EmbeddedFrame>
  );
}
