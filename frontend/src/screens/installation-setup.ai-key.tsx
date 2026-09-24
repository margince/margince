// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import { Field, TextInput } from "../design-system/atoms";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { ServiceAccountKeyField } from "./service-account-key";
import type { SetupProvider } from "./setup-providers";
import { VertexLocationField } from "./vertex-location";

// The model step's credential and where it is bound: the key (pasted, or a
// service-account file for Vertex), the location Vertex is bound at, and the
// write that binds every lane under the choice's own profile.

export function useBindModels() {
  return useMutation({
    // Same reasoning as the provider-key mutation: nothing here is a secret,
    // but the two settle together and a stale binding on screen after a
    // success is the same confusion.
    gcTime: 0,
    mutationFn: async (vars: {
      provider: string;
      baseUrl?: string;
      location?: string;
      profile: SetupProvider["profile"];
      chatModel: string;
      embedModel: string;
    }) => {
      // Every chat tier on the one model the reader chose. A tier left unbound
      // degrades honestly at runtime, but an onboarding that bound only some of
      // them would have the product answer for one task and refuse another with
      // no way for the reader to tell which they had configured.
      const placement = {
        ...(vars.baseUrl ? { base_url: vars.baseUrl } : {}),
        ...(vars.location ? { location: vars.location } : {}),
      };
      const binding = {
        provider: vars.provider,
        model: vars.chatModel,
        ...placement,
      };
      const { error } = await api.PUT("/ai/routing", {
        body: {
          // The choice's own profile rather than a question: asking a
          // first-time admin to choose a location ladder before they have
          // bound anything is asking a question they cannot yet answer, and
          // each choice was offered as the one it binds under.
          profile: vars.profile,
          tiers: {
            local_small: binding,
            cheap_cloud: binding,
            premium: binding,
            frontier: binding,
          },
          embeddings: {
            provider: vars.provider,
            model: vars.embedModel,
            ...placement,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    // No invalidation: re-reading the setup report moves the screen on, and
    // the binding is the moment the ignition marks. The refetch waits for the
    // reader to press past it (`onDone`); a reload mid-sequence finds
    // `ai_models` configured and loses nothing but the ceremony.
  });
}

// A choice that enforces residency says so in its name: it is the one thing
// that sets it apart from the plain Gemini choice above it.
export function choiceLabel(preset: SetupProvider, t: Translator): string {
  return preset.profile === "eu_resident"
    ? `${preset.label} (${t("firstRun.ai.euResidency")})`
    : preset.label;
}

function keyFieldError(
  attempted: boolean,
  secret: string,
  refusal: MessageKey | undefined,
  t: Translator,
): string | undefined {
  if (attempted && secret.trim() === "") {
    return t("firstRun.needed");
  }
  return refusal ? t(refusal) : undefined;
}

/** The credential the chosen vendor takes, and where Vertex is bound. */
export function AiKeyFields({
  preset,
  secret,
  refusal,
  attempted,
  location,
  disabled,
  onSecret,
  onLocation,
}: Readonly<{
  preset: SetupProvider;
  secret: string;
  refusal: MessageKey | undefined;
  attempted: boolean;
  location: string;
  disabled: boolean;
  onSecret: (secret: string) => void;
  onLocation: (location: string) => void;
}>) {
  const t = useT();
  return (
    <>
      {preset.credential === "service_account" ? (
        <ServiceAccountKeyField
          value={secret}
          disabled={disabled}
          hint={t("firstRun.ai.keyHint")}
          error={keyFieldError(attempted, secret, refusal, t)}
          onChange={onSecret}
        />
      ) : (
        <Field
          label={t("firstRun.ai.key")}
          hint={t("firstRun.ai.keyHint")}
          error={keyFieldError(attempted, secret, undefined, t)}
        >
          {(control) => (
            <TextInput
              {...control}
              // A password field so the browser does not offer to remember a
              // credential this app never stores client-side, and a screenshare
              // does not carry it.
              type="password"
              autoComplete="off"
              value={secret}
              disabled={disabled}
              onChange={(e) => onSecret(e.target.value)}
            />
          )}
        </Field>
      )}
      {preset.location !== undefined && (
        <VertexLocationField
          value={location}
          profile={preset.profile}
          disabled={disabled}
          noKeyHint={t("firstRun.ai.locationBeforeKey")}
          onChange={onLocation}
        />
      )}
    </>
  );
}
