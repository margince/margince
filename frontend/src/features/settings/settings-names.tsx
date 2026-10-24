import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Field, TextInput } from "../../design-system/atoms";
import { describedByIds } from "../../design-system/describedby";
import { useSaveDraft, useSavePage } from "../../design-system/savepage";
import { SettingRow } from "../../design-system/settingrow";
import type { Toast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { greetingNameOf } from "../../format/greetingname";
import { type Locale, useLocale, useT } from "../../i18n";
import { isImeCommit } from "../../lib/imecommit";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";

// The two names on the Profile card: the one colleagues see you by and the one
// they greet you by. They commit together through the page's save bar.

const DISPLAY_MAX_RUNES = 255;
const GREETING_MAX_RUNES = 100;

type NameDraft = Readonly<{
  shown: string;
  set: (next: string) => void;
  /** Why this value cannot be sent, or undefined when it can. */
  invalid: string | undefined;
}>;

export type AccountNames = Readonly<{
  display: NameDraft;
  greeting: NameDraft;
  greetingFallback: string | undefined;
}>;

type NamesPatch = Readonly<{ display?: string; greeting?: string }>;
type NameField = "display" | "greeting";

// Characters, which is what the contract's `maxLength` means and what the
// server counts; `String.length` counts UTF-16 units instead.
function runeCount(text: string): number {
  return [...text].length;
}

/**
 * The drafts of both names, handed to the page's save bar. A null draft means
 * "not edited", so a `/me` refetch never overwrites what is being typed.
 */
export function useAccountNames(toast: Toast): AccountNames {
  const t = useT();
  const { locale } = useLocale();
  const me = useMe();
  const queryClient = useQueryClient();
  const storedDisplay = me.data?.user.display_name ?? "";
  const storedGreeting = me.data?.user.greeting_name ?? "";
  const [displayDraft, setDisplay] = useState<string | null>(null);
  const [greetingDraft, setGreeting] = useState<string | null>(null);
  // Trimmed before comparing, or a trailing space reads as a change the server
  // then answers is none.
  const display = (displayDraft ?? storedDisplay).trim();
  const greeting = (greetingDraft ?? storedGreeting).trim();
  const displayChanged = display !== storedDisplay;
  const greetingChanged = greeting !== storedGreeting;

  const { refusedText, attempt, setRefusal } = useNameRefusal();
  const save = useMutation({
    mutationFn: async (patch: NamesPatch) => {
      setRefusal(null);
      await sendNames(patch, attempt, t);
    },
    onSuccess: async () => {
      toast.show(t("settings.saved"));
      // Awaited before the drafts drop, so the fields never flash the old names.
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      void queryClient.invalidateQueries({ queryKey: ["scheduling-profile"] });
      setDisplay(null);
      setGreeting(null);
    },
  });

  const displayInvalid =
    display === ""
      ? t("record.fieldRequired")
      : tooLong(display, DISPLAY_MAX_RUNES, t, locale);
  const greetingInvalid = tooLong(greeting, GREETING_MAX_RUNES, t, locale);
  useSaveDraft({
    dirty: displayChanged || greetingChanged,
    // The field already says what is wrong; the bar holds Save until it is fixed.
    invalid:
      (displayChanged && displayInvalid !== undefined) ||
      (greetingChanged && greetingInvalid !== undefined),
    save: () =>
      save.mutateAsync({
        display: displayChanged ? display : undefined,
        greeting: greetingChanged ? greeting : undefined,
      }),
    discard: () => {
      save.reset();
      setRefusal(null);
      setDisplay(null);
      setGreeting(null);
    },
  });

  return {
    display: {
      shown: displayDraft ?? storedDisplay,
      set: (next) => {
        setRefusal(null);
        setDisplay(next);
      },
      invalid: shownInvalid(
        displayChanged,
        displayInvalid,
        refusedText("display"),
      ),
    },
    greeting: {
      shown: greetingDraft ?? storedGreeting,
      set: (next) => {
        setRefusal(null);
        setGreeting(next);
      },
      invalid: shownInvalid(
        greetingChanged,
        greetingInvalid,
        refusedText("greeting"),
      ),
    },
    // What greetings fall back to while the greeting name is empty.
    greetingFallback:
      greetingNameOf({ display_name: me.data?.user.display_name }) ?? undefined,
  };
}

// The field the server refused, so its sentence stands under that field and the
// other one is not blamed. The bar still carries the one summary error.
function useNameRefusal() {
  const t = useT();
  const [refusal, setRefusal] = useState<{
    field: NameField;
    error: unknown;
  } | null>(null);
  const attempt = async (field: NameField, write: () => Promise<unknown>) => {
    try {
      await write();
    } catch (error) {
      setRefusal({ field, error });
      throw error;
    }
  };
  const refusedText = (field: NameField) =>
    refusal?.field === field ? problemMessageOf(refusal.error, t) : undefined;
  return { refusedText, attempt, setRefusal };
}

// A draft's own fault shows only once it is edited; a server refusal stays.
function shownInvalid(
  changed: boolean,
  own: string | undefined,
  refused: string | undefined,
) {
  return (changed ? own : undefined) ?? refused;
}

async function sendNames(
  patch: NamesPatch,
  attempt: (field: NameField, write: () => Promise<unknown>) => Promise<void>,
  t: ReturnType<typeof useT>,
) {
  if (patch.display !== undefined) {
    const display_name = patch.display;
    await attempt("display", async () =>
      unwrap(await api.PUT("/me/display-name", { body: { display_name } }), t),
    );
  }
  if (patch.greeting !== undefined) {
    const greeting_name = patch.greeting === "" ? null : patch.greeting;
    await attempt("greeting", async () =>
      unwrap(
        await api.PUT("/me/greeting-name", { body: { greeting_name } }),
        t,
      ),
    );
  }
}

function tooLong(
  text: string,
  max: number,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string | undefined {
  return runeCount(text) > max
    ? t("settings.nameTooLong", { max: formatNumber(max, locale) })
    : undefined;
}

/** Both name rows, editing the drafts `useAccountNames` holds. */
export function NameSettingRows({ names }: Readonly<{ names: AccountNames }>) {
  const t = useT();
  return (
    <>
      <NameSettingRow
        label={t("settings.displayName")}
        description={t("settings.displayNameHelp")}
        draft={names.display}
      />
      <NameSettingRow
        label={t("settings.greetingName")}
        description={t("settings.greetingNameHelp")}
        draft={names.greeting}
        placeholder={names.greetingFallback}
      />
    </>
  );
}

function NameSettingRow({
  label,
  description,
  draft,
  placeholder,
}: Readonly<{
  label: string;
  description: string;
  draft: NameDraft;
  placeholder?: string;
}>) {
  const page = useSavePage();
  return (
    <SettingRow
      label={label}
      description={description}
      control={(row) => (
        <Field
          label={label}
          labelHidden
          error={draft.invalid}
          className="settingrow-measure"
        >
          {(field) => (
            <TextInput
              {...field}
              aria-labelledby={row["aria-labelledby"]}
              aria-describedby={describedByIds(
                field["aria-describedby"],
                row["aria-describedby"],
              )}
              value={draft.shown}
              placeholder={placeholder}
              // No `maxLength`: it counts UTF-16 units, not characters.
              onChange={(event) => draft.set(event.target.value)}
              onKeyDown={(event) => {
                // Enter commits a changed name, as a rename does elsewhere. The
                // Enter that ends an IME composition picks the word instead.
                if (
                  event.key === "Enter" &&
                  !isImeCommit(event) &&
                  page?.dirty &&
                  !page.invalid &&
                  !page.pending
                ) {
                  void page.save();
                }
              }}
            />
          )}
        </Field>
      )}
    />
  );
}
