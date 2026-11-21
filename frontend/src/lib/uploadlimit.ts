import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { INTL_LOCALE } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { throwProblem } from "./problem";
import { signInShowsRefusal } from "./session.read";

// The installation's own settings, and the one field on them a form has to act
// on: how large a file this deployment accepts.
//
// The number is the OPERATOR's, not the build's — an installation sets its own
// ceiling in its deployment file, so a constant compiled in here would be right
// only for whoever shipped the default. It arrives on the installation read
// every role may make.
//
// Why a client wants it at all, when the server refuses an oversize upload
// anyway: because refusing afterwards means the whole file crossed the wire
// first. Told the limit, a form can state it before a file is chosen and refuse
// in the instant one is.
//
// ONE query for both readers — the settings screen and any upload form — under
// one key. Two hooks on the same key would be worse than two keys: whichever
// mounted first would decide the behaviour of the other, and only sometimes.

/** The shared query key. Exported so a mutation can invalidate this read. */
export const INSTALLATION_SETTINGS_KEY = ["installation-settings"] as const;

/**
 * The installation's settings, as the whole query.
 *
 * The queryFn throws, so a screen showing this record can gate on `error` and
 * say what went wrong. A caller that only wants a field reads `data?.field` and
 * gets undefined either way — no second request, and no second opinion about
 * what a failure means.
 */
export const installationSettingsOptions = queryOptions({
  queryKey: INSTALLATION_SETTINGS_KEY,
  meta: signInShowsRefusal,
  queryFn: async () => {
    const { data, error, response } = await api.GET("/installation/settings");
    if (error || !response.ok) throwProblem(error);
    return data;
  },
});

export function useInstallationSettings(enabled = true) {
  return useQuery({ ...installationSettingsOptions, enabled });
}

/**
 * The largest upload this installation accepts, in bytes — or undefined while
 * the answer is still on its way, or if it never comes.
 *
 * Undefined is deliberately NOT a number. Guessing a limit before the server
 * has stated one produces this feature's own failure in reverse: a form that
 * refuses a file the installation would have taken, over a number the client
 * invented and the reader cannot argue with. With no answer the upload proceeds
 * and the refusal stays where it was before any of this existed — on the
 * server, which is the authority regardless.
 */
export function useMaxUploadBytes(): number | undefined {
  return useInstallationSettings().data?.max_upload_bytes;
}

/**
 * The ceiling as a reader reading a form would write it: decimal MB, exactly.
 *
 * Decimal because that is what the server enforces and what its refusal says. A
 * binary megabyte here would state a limit 4.8% larger than the real one, and
 * the reader who believed it would then be refused by a server that had already
 * told them twice, in two different numbers.
 */
export function formatUploadLimit(bytes: number, locale: Locale): string {
  const mb = bytes / 1_000_000;
  // Through Intl, so the decimal separator is the reader's own: a German reader
  // shown "12.5 MB" reads twelve and a half as twelve-thousand-five-hundred,
  // and the unit word rides Intl's vocabulary rather than a table here.
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unit: "megabyte",
    unitDisplay: "short",
    maximumFractionDigits: Number.isInteger(mb) ? 0 : 1,
  }).format(mb);
}

/** What a picked file is held to before it is sent. Unset, a rule is the server's alone. */
export type PickRules = Readonly<{
  maxBytes: number | undefined;
  // The kinds the server keeps, read as it reads them. A declared type must be
  // in `mediaTypes`; a file with none needs an ending in `extensions`.
  kinds?: Readonly<{
    extensions: readonly string[];
    mediaTypes: readonly string[];
  }>;
}>;

// The declared type without its parameters, or "" when it says nothing.
function declaredTypeOf(file: File): string {
  const media = (file.type.split(";")[0] ?? "").trim().toLowerCase();
  return media === "application/octet-stream" ? "" : media;
}

// The last ".ending" of a name, as the server reads it, or "" for none.
function endingOf(filename: string): string {
  const dot = filename.lastIndexOf(".");
  return dot >= 0 ? filename.slice(dot).toLowerCase() : "";
}

/**
 * Why a file the reader just picked cannot be sent, in their words, or null.
 *
 * Asked as the file is dropped, so a file the server would refuse never waits
 * in the form. The server still refuses it, so a stale rule here costs a request.
 */
export function usePickRefusal(
  rules: PickRules,
): (file: File) => string | null {
  const t = useT();
  const { locale } = useLocale();
  return (file) => {
    if (file.size === 0) {
      return t("files.pick.empty");
    }
    if (rules.maxBytes !== undefined && file.size > rules.maxBytes) {
      return t("docs.add.errTooLarge", {
        size: formatUploadLimit(rules.maxBytes, locale),
      });
    }
    if (rules.kinds) {
      const declared = declaredTypeOf(file);
      const kept =
        declared === ""
          ? rules.kinds.extensions.includes(endingOf(file.name))
          : rules.kinds.mediaTypes.includes(declared);
      if (!kept) return t("files.pick.unsupported");
    }
    return null;
  };
}
