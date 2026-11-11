// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Checklist, ChecklistStep } from "../design-system/checklist";
import { OffsiteLink } from "../design-system/offsitelink";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { VENDOR_CONSOLE, type Vendor } from "../lib/oauth-app";

type ConsoleStep = Readonly<{
  title: MessageKey;
  body: MessageKey;
  link?: Readonly<{ href: string; label: MessageKey }>;
}>;

/** Where in the vendor's console the redirect URIs go, for every screen that
 * lists them. */
export const REDIRECTS_HOW_TO = {
  google: "oauthApp.drawer.google.step2Body",
  microsoft: "oauthApp.drawer.microsoft.step2Body",
} as const satisfies Record<Vendor, MessageKey>;

// The vendor's console work in the order the console asks for it. The redirect
// step sits where each vendor's screens put it, and pasting the pair is last.
const VENDOR_STEPS: Readonly<
  Record<
    Vendor,
    Readonly<{
      before: readonly ConsoleStep[];
      redirects: MessageKey;
      after: readonly ConsoleStep[];
      paste: Readonly<{ title: MessageKey; body: MessageKey }>;
    }>
  >
> = {
  google: {
    before: [
      {
        title: "oauthApp.drawer.google.step1Title",
        body: "oauthApp.drawer.google.step1Body",
        link: {
          href: VENDOR_CONSOLE.google,
          label: "oauthApp.drawer.google.step1Link",
        },
      },
      {
        title: "oauthApp.steps.google.apisTitle",
        body: "oauthApp.steps.google.apisBody",
        link: {
          href: "https://console.cloud.google.com/apis/library",
          label: "oauthApp.steps.google.apisLink",
        },
      },
    ],
    redirects: REDIRECTS_HOW_TO.google,
    after: [],
    paste: {
      title: "oauthApp.drawer.step3Title",
      body: "oauthApp.steps.google.pasteBody",
    },
  },
  microsoft: {
    before: [
      {
        title: "oauthApp.drawer.microsoft.step1Title",
        body: "oauthApp.drawer.microsoft.step1Body",
        link: {
          href: VENDOR_CONSOLE.microsoft,
          label: "oauthApp.drawer.microsoft.step1Link",
        },
      },
    ],
    redirects: REDIRECTS_HOW_TO.microsoft,
    after: [
      {
        title: "oauthApp.steps.microsoft.permissionsTitle",
        body: "oauthApp.steps.microsoft.permissionsBody",
      },
      {
        title: "oauthApp.steps.microsoft.secretTitle",
        body: "oauthApp.steps.microsoft.secretBody",
      },
    ],
    paste: {
      title: "oauthApp.steps.microsoft.pasteTitle",
      body: "oauthApp.steps.microsoft.pasteBody",
    },
  },
};

function ConsoleSteps({ steps }: Readonly<{ steps: readonly ConsoleStep[] }>) {
  const t = useT();
  return steps.map((step) => (
    <ChecklistStep
      key={step.title}
      state="open"
      title={t(step.title)}
      caption={t(step.body)}
    >
      {step.link && (
        <OffsiteLink href={step.link.href}>{t(step.link.label)}</OffsiteLink>
      )}
    </ChecklistStep>
  ));
}

/**
 * What an admin does in the vendor's console to make the company's app, step
 * by step, ending in the step that takes the client ID and secret. Settings
 * and first run both show it, so the two cannot tell an admin different steps.
 * Every step stays open: the product cannot see the vendor's console.
 */
export function OAuthAppSteps({
  provider,
  redirects,
  children,
}: Readonly<{
  provider: Vendor;
  /** The redirect URI table, loaded the way the host loads the app. */
  redirects: ReactNode;
  /** The fields that take the pair, under the last step. */
  children: ReactNode;
}>) {
  const t = useT();
  const steps = VENDOR_STEPS[provider];
  return (
    <Checklist>
      <ConsoleSteps steps={steps.before} />
      <ChecklistStep
        state="open"
        title={t("oauthApp.drawer.step2Title")}
        caption={t(steps.redirects)}
      >
        {redirects}
      </ChecklistStep>
      <ConsoleSteps steps={steps.after} />
      <ChecklistStep
        state="open"
        title={t(steps.paste.title)}
        caption={t(steps.paste.body)}
      >
        {children}
      </ChecklistStep>
    </Checklist>
  );
}
