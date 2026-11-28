// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ComponentProps } from "react";
import { Card } from "../design-system/atoms";
import { Button } from "../design-system/button";
import { OffsiteLink } from "../design-system/offsitelink";
import { useT } from "../i18n";
import { EMBED_PATH, isEmbedAddress } from "../lib/recordaddress";
import { AuthScreen } from "./authscreens";
import { RaillessFrame } from "./railless";

/** True when this document is a unit's screen inside a frame the operator allows. */
export function isEmbedded(): boolean {
  const { pathname, hash } = globalThis.location;
  return isEmbedAddress(pathname, hash);
}

/** True on the embed path with an address that names no unit's screen. */
export function unservedEmbedAddress(): boolean {
  return globalThis.location.pathname === EMBED_PATH && !isEmbedded();
}

// A provider sign-in refuses to run inside a frame, so a framed reader signs in
// in a tab of this origin and comes back. The session cookie reaches the frame
// only where the browser counts it as same-site, as for an extension with host
// permission for this origin.
export function SignInScreen({
  onRetry,
  ...signIn
}: Readonly<ComponentProps<typeof AuthScreen> & { onRetry: () => void }>) {
  if (!isEmbedded()) {
    return (
      <RaillessFrame>
        <AuthScreen {...signIn} />
      </RaillessFrame>
    );
  }
  return <EmbedSignIn onRetry={onRetry} />;
}

function EmbedSignIn({ onRetry }: Readonly<{ onRetry: () => void }>) {
  const t = useT();
  return (
    <RaillessFrame>
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
    </RaillessFrame>
  );
}
