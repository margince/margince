// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useT } from "../i18n";
import {
  modelUnbound,
  useInstallationSetup,
} from "../lib/installation-setup-query";
import { problemCodeOf } from "../lib/problem";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import { AppLink } from "./applink";
import { ErrorLine } from "./errorline";

/** True for the one problem the server sends for work that needs a model. */
export function isAiNotConnected(error: unknown): boolean {
  return problemCodeOf(error) === "ai_not_configured";
}

/** "Connect an AI first" and the link to the page that connects one. */
export function ConnectAiFirst() {
  const t = useT();
  return (
    <>
      {t("connectAi.first")}{" "}
      <AppLink href={routeHash(settingsHref("models"))}>
        {t("connectAi.link")}
      </AppLink>
    </>
  );
}

/**
 * What a button that asks the AI to work shows before it is pressed. It turns
 * refused only once the installation's setup read says no model is connected,
 * so a failed or pending read never locks a button. The read is the one the
 * setup checklist already makes.
 */
export function useConnectAiGate(id: string): {
  reasonId: string | undefined;
  notice: ReactNode;
} {
  const setup = useInstallationSetup();
  if (!modelUnbound(setup.data)) {
    return { reasonId: undefined, notice: null };
  }
  return {
    reasonId: id,
    notice: (
      <span id={id} className="t-caption">
        <ConnectAiFirst />
      </span>
    ),
  };
}

/**
 * The line under a button whose AI work failed. A refusal for want of a model
 * says "Connect an AI first" with its link; any other failure reads as it
 * always did.
 */
export function ModelWorkError({ error }: Readonly<{ error: unknown }>) {
  if (isAiNotConnected(error)) {
    return (
      <ErrorLine>
        <ConnectAiFirst />
      </ErrorLine>
    );
  }
  return <ErrorLine error={error} />;
}
