import type { components } from "../../api/schema";

/**
 * The cached surfaces whose text is written by a model in the installation's
 * base language.
 *
 * Every one of these is served from a server-side cache keyed on a fingerprint
 * that now carries the language. So the server rewrites them correctly the
 * moment the setting changes — and the browser never asks, because its own
 * cache is keyed on the record id alone and `refetchOnWindowFocus` is off. An
 * admin changes the language, the page they are looking at keeps rendering the
 * old one, and nothing on screen says why.
 *
 * The list is declared once here rather than spelled at the mutation, because
 * the failure it prevents is forgetting to add to it. `aicaches.test.ts` walks
 * the screens and fails when a query key looks like one of these and is not
 * named, which is the half a list cannot hold on its own.
 */
export const LANGUAGE_DEPENDENT_QUERY_PREFIXES = [
  ["company-dossier"],
  ["company-growth-fit"],
  ["meetingBrief"],
  ["contactBrief"],
  ["deal-status"],
  ["deal-room-brief"],
  ["brief"],
] as const;

type Patch = components["schemas"]["UpdateInstallationSettingsRequest"];

/** What these texts are written in: the language, and the zone they count days in. */
export const SETTINGS_IN_SERVER_TEXT = [
  "base_language",
  "timezone",
] as const satisfies readonly (keyof Patch)[];
