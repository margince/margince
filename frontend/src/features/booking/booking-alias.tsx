// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { type Translator, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemFieldErrorsOf, unwrap } from "../../lib/problem";

type Profile = components["schemas"]["SchedulingProfile"];

// The refusal codes PUT /scheduling/profile/alias answers on `alias`.
const ALIAS_ERRORS: Readonly<Record<string, MessageKey>> = {
  too_short: "scheduling.alias.error.too_short",
  too_long: "scheduling.alias.error.too_long",
  invalid: "scheduling.alias.error.invalid",
  reserved: "scheduling.alias.error.reserved",
  taken: "scheduling.alias.error.taken",
  no_booking_link: "scheduling.alias.error.no_booking_link",
};

/** The server's normalization, so the preview shows the address it will save. */
export function normalizeAlias(raw: string): string {
  return raw.trim().toLowerCase();
}

/** The path an alias makes. Only the server's public_url is a whole address. */
export function aliasPath(alias: string): string {
  return `/book/${alias}`;
}

/**
 * The address guests use now: the server's public_url, or the bare path when
 * no public origin is configured, as the other link surfaces show it.
 */
export function currentBookingAddress(profile: Profile, alias: string): string {
  return profile.public_url ?? aliasPath(alias);
}

function aliasError(t: Translator, error: unknown): string | undefined {
  const problem = problemFieldErrorsOf(error).find((p) => p.field === "alias");
  if (!problem) return undefined;
  const key = ALIAS_ERRORS[problem.code];
  return key ? t(key) : problem.message;
}

function useSaveAlias() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (alias: string) =>
      unwrap(await api.PUT("/scheduling/profile/alias", { body: { alias } })),
    onSuccess: (profile) => {
      client.setQueryData(["scheduling-profile"], profile);
    },
  });
}

/**
 * Choose, change or remove the host's short booking address. Changing or
 * removing one is asked first, because the old address stops working at once.
 */
export function BookingAliasField({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const current = profile.alias ?? "";
  const [draft, setDraft] = useState(current);
  const [confirming, setConfirming] = useState<string | null>(null);
  const save = useSaveAlias();
  const next = normalizeAlias(draft);
  const write = (alias: string) =>
    save.mutate(alias, {
      onSuccess: (saved) => {
        setDraft(saved.alias ?? "");
        setConfirming(null);
      },
      onError: () => setConfirming(null),
    });
  const submit = (alias: string) => {
    if (current !== "" && alias !== current) setConfirming(alias);
    else write(alias);
  };
  const fieldError = aliasError(t, save.error);
  return (
    <>
      <Field
        label={t("scheduling.alias.label")}
        hint={t("scheduling.alias.help")}
        error={fieldError}
      >
        {(control) => (
          <div className="meeting-address">
            <TextInput
              {...control}
              value={draft}
              maxLength={200}
              autoComplete="off"
              spellCheck={false}
              onChange={(event) => setDraft(event.target.value)}
            />
            <Button
              disabled={next === current || next === "" || save.isPending}
              onClick={() => submit(next)}
            >
              {t("scheduling.alias.save")}
            </Button>
            {current !== "" && (
              <Button disabled={save.isPending} onClick={() => submit("")}>
                {t("scheduling.alias.clear")}
              </Button>
            )}
          </div>
        )}
      </Field>
      {next !== "" && next !== current && (
        <p className="t-caption">
          {t("scheduling.alias.preview", { path: aliasPath(next) })}
        </p>
      )}
      {!fieldError && <ErrorLine error={save.error} />}
      <ConfirmModal
        open={confirming !== null}
        onClose={() => setConfirming(null)}
        title={
          confirming === ""
            ? t("scheduling.alias.clearTitle")
            : t("scheduling.alias.changeTitle")
        }
        confirmLabel={
          confirming === ""
            ? t("scheduling.alias.clear")
            : t("scheduling.alias.save")
        }
        pending={save.isPending}
        onConfirm={() => confirming !== null && write(confirming)}
      >
        <p>
          {t("scheduling.alias.changeHelp", {
            old: currentBookingAddress(profile, current),
          })}
        </p>
      </ConfirmModal>
    </>
  );
}
