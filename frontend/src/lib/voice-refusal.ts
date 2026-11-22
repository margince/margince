import { problemCode, problemDetails } from "./problembody";

// The voice corpus's refusal vocabulary: which server codes mean a source was
// refused for a reason both intake surfaces word for themselves.

/** Why the server would not take a source, as a category both surfaces phrase
 * in their own words. `null` means the refusal carried no code either surface
 * knows: the caller states the server's own detail instead of inventing one. */
export type RefusalReason =
  | "unattributed"
  | "speaker"
  | "unsupported"
  | "tooLarge";

/** The server's largest JSON request body, voice source text included, as a
 *  mirror of httperr.MaxBodyBytes that frontendvoicesourcecap_test holds. */
export const VOICE_SOURCE_MAX_BYTES = 1_048_576;

const refusalReasons: Record<string, RefusalReason> = {
  unattributed_transcript: "unattributed",
  speaker_label_required: "unattributed",
  speaker_not_found: "speaker",
  unsupported_format: "unsupported",
  body_too_large: "tooLarge",
};

// Every stable machine code an RFC 7807 body carries: the top-level `code` plus
// any per-field `details.errors[].code`.
function problemCodes(problem: unknown): string[] {
  const top = problemCode(problem);
  const errors = problemDetails(problem)?.errors;
  const fieldCodes = Array.isArray(errors)
    ? errors.map(problemCode).filter((code) => code !== null)
    : [];
  return top === null ? fieldCodes : [top, ...fieldCodes];
}

// The refusal category a 422 names, or null when no code here recognizes it.
export function refusalOf(problem: unknown): RefusalReason | null {
  for (const code of problemCodes(problem)) {
    const reason = refusalReasons[code];
    if (reason !== undefined) {
      return reason;
    }
  }
  return null;
}
