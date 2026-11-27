// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen } from "@testing-library/react";
import type userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../../api/schema";
import { type Locale, LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export const SETTINGS_EDITOR: GrantSpec = { installation_settings: ["update"] };

type BackendOptions = {
  locked?: boolean;
  lockedReason?: string;
  /** What the server refuses the PATCH with, as an RFC-7807 validation body. */
  refuse?: readonly {
    field: string;
    code: string;
    message: string;
    max?: number;
    params?: Readonly<Record<string, string>>;
  }[];
  /** The lock the read reports once the PATCH was refused: a rate froze it meanwhile. */
  lockAfterRefusal?: components["schemas"]["BaseCurrencyLock"];
  /** The settings a boot source sets, as the read reports them. */
  locks?: unknown[];
  /** What an accepted PATCH waits on before it answers. */
  hold?: Promise<void>;
};

// backendFor answers /me with the given grants and /installation/settings with
// the given state, capturing any PATCH body so the wire shape is assertable.
export function backendFor(allow: GrantSpec, opts: BackendOptions = {}) {
  let state: Record<string, unknown> = {
    name: "Brandt Automotive",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
    base_currency_locked: opts.locked ?? false,
    call_whatsapp: true,
    call_telegram: true,
    call_zalo: true,
    individual_performance: true,
    forecast_self_correcting: true,
    forecast_likely_threshold: 90,
    ...(opts.lockedReason
      ? { base_currency_locked_reason: opts.lockedReason }
      : {}),
    ...(opts.locks ? { locks: opts.locks } : {}),
  };
  let capturedPatch: Record<string, unknown> | null = null;
  const answerPatch = async (body: Record<string, unknown>) => {
    capturedPatch = body;
    // Once the currency locked, only a patch still carrying it is refused.
    const sendsCurrency = Object.hasOwn(body, "base_currency");
    if (opts.refuse && (!opts.lockAfterRefusal || sendsCurrency)) {
      if (opts.lockAfterRefusal) {
        state = {
          ...state,
          base_currency_locked: true,
          base_currency_lock: opts.lockAfterRefusal,
        };
      }
      // The shape httperr.Validation actually emits: one top-level code
      // for every 422, and the rule that fired named per field.
      return jsonResponse(
        {
          code: "validation_error",
          detail: "The installation could not be saved.",
          details: { errors: opts.refuse },
        },
        422,
      );
    }
    await opts.hold;
    state = { ...state, ...body };
    return jsonResponse(state);
  };
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url, "http://localhost");
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (url.pathname.endsWith("/installation/settings/fiscal-year-impact")) {
        return jsonResponse({
          saved_questions: 14,
          reports_with_kept_periods: 1,
          reports_with_relative_periods: 3,
        });
      }
      if (url.pathname.endsWith("/installation/settings")) {
        return req.method === "PATCH"
          ? answerPatch(await req.json())
          : jsonResponse(state);
      }
      throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
    },
  );
  // Another admin clears what held the currency.
  const unlock = () => {
    state = {
      ...state,
      base_currency_locked: false,
      base_currency_lock: undefined,
    };
  };
  return { fetchMock, patch: () => capturedPatch, unlock };
}

export function render(
  node: ReactNode,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  locale?: Locale,
) {
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// Opens the dialog through a row's own Edit verb, the only way a reader gets in.
export async function openFrom(
  user: ReturnType<typeof userEvent.setup>,
  fact: RegExp,
): Promise<HTMLElement> {
  const edit = await screen.findByRole("button", { name: fact });
  await user.click(edit);
  return screen.getByRole("dialog");
}
