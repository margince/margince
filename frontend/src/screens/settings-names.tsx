import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Button, Field, TextInput } from "../design-system/atoms";
import { SettingRow } from "../design-system/settingrow";
import type { Toast } from "../design-system/toast";
import { greetingNameOf } from "../format/greetingname";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem, useMe } from "./common";

// The two names on the Account card: the one colleagues see you by and the one
// they greet you by. Both are the caller's own and read back from `/me`.

/**
 * The name colleagues see you by.
 *
 * The saved answer is read back from `/me` rather than kept here, so the shell's
 * account chip and the roster agree with this row the moment it lands.
 */
export function DisplayNameSettingRow({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const me = useMe();
  const queryClient = useQueryClient();
  const stored = me.data?.user.display_name ?? "";
  // null means "not editing" — the row shows the stored name until the reader
  // types, so a `/me` refetch cannot overwrite what they are in the middle of.
  const [draft, setDraft] = useState<string | null>(null);
  const shown = draft ?? stored;
  const save = useMutation({
    mutationFn: async (next: string) => {
      const { data, error } = await api.PUT("/me/display-name", {
        body: { display_name: next },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (saved) => {
      // Keep the saved name visible if the account refetch is delayed or fails.
      setDraft(saved?.display_name ?? null);
      void queryClient.invalidateQueries({ queryKey: ["scheduling-profile"] });
      toast.show(t("settings.saved"));
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });
  // Trimmed for the comparison as well as for the send, or a name with a
  // trailing space reads as a change and the server answers that it is not one.
  const trimmed = shown.trim();
  const dirty = trimmed !== stored;
  // Counted in CHARACTERS, which is what the contract's `maxLength` means and
  // what the server checks with `utf8.RuneCountInString`. `String.length` would
  // count UTF-16 units and refuse a name the server admits.
  const tooLong = [...trimmed].length > 255;
  const refusal = save.error ? problemMessageOf(save.error, t) : undefined;
  return (
    <SettingRow
      label={t("settings.displayName")}
      description={t("settings.displayNameHelp")}
      layout="stack"
      control={(row) => (
        // The catalogued pairing of field and verb, as the pipeline rows use.
        <div className="form-stack settingrow-measure">
          <Field label={t("settings.displayName")} labelHidden error={refusal}>
            {(field) => (
              <TextInput
                {...field}
                aria-labelledby={row["aria-labelledby"]}
                aria-describedby={[field, row]
                  .map((owner) => owner["aria-describedby"])
                  .filter(Boolean)
                  .join(" ")}
                value={shown}
                // No `maxLength`: UTF-16 units, not the runes `tooLong` counts.
                onChange={(event) => setDraft(event.target.value)}
              />
            )}
          </Field>
          <Button
            disabled={!dirty || trimmed === "" || tooLong || save.isPending}
            onClick={() => save.mutate(trimmed)}
          >
            {t("settings.displayNameSave")}
          </Button>
        </div>
      )}
    />
  );
}

/**
 * The name colleagues greet you by, when the first word of the display name is
 * not it. Empty is a real answer: it clears the choice, and greetings go back
 * to that first word.
 */
export function GreetingNameSettingRow({ toast }: Readonly<{ toast: Toast }>) {
  const t = useT();
  const me = useMe();
  const queryClient = useQueryClient();
  const stored = me.data?.user.greeting_name ?? "";
  // null means "not editing", as in DisplayNameSettingRow.
  const [draft, setDraft] = useState<string | null>(null);
  const shown = draft ?? stored;
  const save = useMutation({
    mutationFn: async (next: string) => {
      const { data, error } = await api.PUT("/me/greeting-name", {
        body: { greeting_name: next === "" ? null : next },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (saved) => {
      setDraft(saved?.greeting_name ?? "");
      toast.show(t("settings.saved"));
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });
  const trimmed = shown.trim();
  const dirty = trimmed !== stored;
  // Characters, not UTF-16 units, as the server counts them.
  const tooLong = [...trimmed].length > 100;
  const refusal = save.error ? problemMessageOf(save.error, t) : undefined;
  return (
    <SettingRow
      label={t("settings.greetingName")}
      description={t("settings.greetingNameHelp")}
      layout="stack"
      control={(row) => (
        <div className="form-stack settingrow-measure">
          <Field label={t("settings.greetingName")} labelHidden error={refusal}>
            {(field) => (
              <TextInput
                {...field}
                aria-labelledby={row["aria-labelledby"]}
                aria-describedby={[field, row]
                  .map((owner) => owner["aria-describedby"])
                  .filter(Boolean)
                  .join(" ")}
                value={shown}
                // What greetings fall back to while this is empty.
                placeholder={
                  greetingNameOf({
                    display_name: me.data?.user.display_name,
                  }) ?? undefined
                }
                onChange={(event) => setDraft(event.target.value)}
              />
            )}
          </Field>
          <Button
            disabled={!dirty || tooLong || save.isPending}
            onClick={() => save.mutate(trimmed)}
          >
            {t("settings.greetingNameSave")}
          </Button>
        </div>
      )}
    />
  );
}
