// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  asOAuthConnectProvider,
  type OAuthConnectProvider,
} from "../format/connectorprovider";
import { throwProblem } from "./problem";
import { problemCode } from "./problembody";

type ConnectRequest = components["schemas"]["ConnectConnectorRequest"];
type Provider = components["schemas"]["CaptureConnection"]["provider"];

export type ConnectReturnTo = NonNullable<ConnectRequest["return_to"]>;
export type PairedCalendar = NonNullable<ConnectRequest["also_connect"]>;
export type ImportWindow = NonNullable<ConnectRequest["import_window"]>;

export type ConnectAccountInput = Readonly<{
  provider: OAuthConnectProvider;
  /** The surface the consent round trip lands back on. */
  returnTo: ConnectReturnTo;
  /** The vendor's calendar, asked for straight after its mail grant. */
  alsoConnect?: PairedCalendar;
  /** How far back to import past email once the mail grant is stored. */
  importWindow?: ImportWindow;
}>;

/** What a press led to that the screen still has to draw. A redirect leaves
 *  the page, so only the refusal is ever read back. */
export type ConnectAccountResult =
  | Readonly<{ kind: "redirecting" }>
  | Readonly<{ kind: "not_configured"; provider: OAuthConnectProvider }>;

export function isOAuthConnectProvider(
  provider: Provider,
): provider is OAuthConnectProvider {
  return asOAuthConnectProvider(provider) !== null;
}

/** The body carries only what the caller asked for, because an absent field
 *  is itself an answer to the server: no calendar leg, no import. */
function connectBody(input: ConnectAccountInput): ConnectRequest {
  return {
    return_to: input.returnTo,
    ...(input.alsoConnect && { also_connect: input.alsoConnect }),
    ...(input.importWindow && { import_window: input.importWindow }),
  };
}

/**
 * The one way a screen starts a mail or calendar consent. Connections,
 * Meetings and onboarding all use it, so the body, the 501 reading and the
 * hand-off to the vendor cannot drift between them.
 *
 * A provider this installation never wired answers 501 not_implemented. That
 * is a documented configuration, so it returns a result naming the provider.
 * Every other problem throws.
 *
 * `onRedirect` runs just before the page leaves, for a caller that must
 * remember the attempt across the round trip.
 */
export function useConnectAccount(
  options?: Readonly<{ onRedirect?: (input: ConnectAccountInput) => void }>,
) {
  return useMutation({
    mutationFn: async (
      input: ConnectAccountInput,
    ): Promise<ConnectAccountResult> => {
      const { data, error, response } = await api.POST(
        "/connectors/{provider}/connect",
        {
          params: { path: { provider: input.provider } },
          body: connectBody(input),
        },
      );
      if (response.status === 501 && problemCode(error) === "not_implemented") {
        return { kind: "not_configured", provider: input.provider };
      }
      if (error) {
        throwProblem(error);
      }
      if (data.authorize_url) {
        options?.onRedirect?.(input);
        globalThis.location.assign(data.authorize_url);
      }
      return { kind: "redirecting" };
    },
  });
}

/** The provider whose last press found it unwired here, or null. */
export function notConfiguredProvider(
  result: ConnectAccountResult | undefined,
): OAuthConnectProvider | null {
  return result?.kind === "not_configured" ? result.provider : null;
}
