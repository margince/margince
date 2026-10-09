import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Button, Field, TextInput } from "../design-system/atoms";
import { SettingRow } from "../design-system/settingrow";
import type { Toast } from "../design-system/toast";
import { greetingNameOf } from "../format/greetingname";
import { useT } from "../i18n";
import { problemMessageOf, unwrap, useMe } from "./common";

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
  return (
    <NameSettingRow
      toast={toast}
      stored={me.data?.user.display_name ?? ""}
      label={t("settings.displayName")}
      description={t("settings.displayNameHelp")}
      saveLabel={t("settings.displayNameSave")}
      maxRunes={255}
      emptyAllowed={false}
      save={async (next) => {
        const data = unwrap(
          await api.PUT("/me/display-name", {
            body: { display_name: next },
          }),
          t,
        );
        void queryClient.invalidateQueries({
          queryKey: ["scheduling-profile"],
        });
        return data?.display_name ?? null;
      }}
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
  return (
    <NameSettingRow
      toast={toast}
      stored={me.data?.user.greeting_name ?? ""}
      label={t("settings.greetingName")}
      description={t("settings.greetingNameHelp")}
      saveLabel={t("settings.greetingNameSave")}
      maxRunes={100}
      emptyAllowed
      // What greetings fall back to while this is empty.
      placeholder={
        greetingNameOf({ display_name: me.data?.user.display_name }) ??
        undefined
      }
      save={async (next) => {
        const data = unwrap(
          await api.PUT("/me/greeting-name", {
            body: { greeting_name: next === "" ? null : next },
          }),
          t,
        );
        return data?.greeting_name ?? "";
      }}
    />
  );
}

/**
 * One editable name on the Account card. `save` sends the trimmed text and
 * answers the name as stored, which the row keeps visible if the `/me` refetch
 * is slow or fails.
 */
function NameSettingRow({
  toast,
  stored,
  label,
  description,
  saveLabel,
  maxRunes,
  emptyAllowed,
  placeholder,
  save: send,
}: Readonly<{
  toast: Toast;
  stored: string;
  label: string;
  description: string;
  saveLabel: string;
  maxRunes: number;
  emptyAllowed: boolean;
  placeholder?: string;
  save: (next: string) => Promise<string | null>;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  // null means "not editing" — the row shows the stored name until the reader
  // types, so a `/me` refetch cannot overwrite what they are in the middle of.
  const [draft, setDraft] = useState<string | null>(null);
  const shown = draft ?? stored;
  const save = useMutation({
    mutationFn: send,
    onSuccess: (saved) => {
      setDraft(saved);
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
  const tooLong = [...trimmed].length > maxRunes;
  const refusal = save.error ? problemMessageOf(save.error, t) : undefined;
  return (
    <SettingRow
      label={label}
      description={description}
      layout="stack"
      control={(row) => (
        // The catalogued pairing of field and verb, as the pipeline rows use.
        <div className="form-stack settingrow-measure">
          <Field label={label} labelHidden error={refusal}>
            {(field) => (
              <TextInput
                {...field}
                aria-labelledby={row["aria-labelledby"]}
                aria-describedby={[field, row]
                  .map((owner) => owner["aria-describedby"])
                  .filter(Boolean)
                  .join(" ")}
                value={shown}
                placeholder={placeholder}
                // No `maxLength`: UTF-16 units, not the runes `tooLong` counts.
                onChange={(event) => setDraft(event.target.value)}
              />
            )}
          </Field>
          <Button
            disabled={!dirty || (!emptyAllowed && trimmed === "") || tooLong}
            pending={save.isPending}
            onClick={() => save.mutate(trimmed)}
          >
            {saveLabel}
          </Button>
        </div>
      )}
    />
  );
}
