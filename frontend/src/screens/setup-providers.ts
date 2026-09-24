/**
 * The providers onboarding offers, and what each one starts bound to.
 *
 * A DELIBERATE MIRROR of the server, not a second answer: every model named
 * here must carry a rate row in `SeedModelRates`, or a call on it reports
 * UNPRICED — a materially different signal from free, and a poor thing to hand
 * somebody in their first five minutes. `backend/gates/frontendsetupproviders_test.go`
 * compares both directions.
 *
 * WHY ONLY THESE. A routing document REQUIRES an embeddings binding, and the
 * ones listed here are the vendors that serve chat and embeddings from ONE key.
 * Anthropic publishes no embedding model at all and OpenAI's is not in the
 * price sheet, so offering either would walk a first-time admin into a form
 * they cannot complete, or quietly bind their retrieval lane to a fake. Both
 * remain available in Settings → AI, where an operator who already has two
 * vendor accounts can bind the lanes separately.
 *
 * WHY DEFAULTS AND NOT A LIST. The two models named here are where each field
 * OPENS, not what it offers. What it offers comes from the server — the price
 * sheet, which is the one list that has to be right, since a model outside it
 * reports UNPRICED on every call. A list in this file would be the frontend
 * inventing a catalogue and calling it the truth, wrong the week a vendor ships
 * a model; and the field takes anything typed either way, because the server
 * accepts any id its vendor serves.
 *
 * OpenRouter is a PRESET rather than a fifth adapter: on the wire it is
 * `openai_compatible`, which fails closed without a `base_url`, and asking an
 * admin to know that is asking them to know our adapter names.
 */
import type { components } from "../api/schema";

export type SetupProvider = {
  /** What onboarding calls it. */
  readonly label: string;
  /** The adapter the wire names. */
  readonly provider: string;
  /** Required for the OpenAI-wire brokers, absent for a native vendor. */
  readonly baseUrl?: string;
  /** The variable the server reads this key from, shown so an operator can find it. */
  readonly keyEnv: string;
  /** Where the chat lanes start. Editable. */
  readonly chatModel: string;
  /** The embedding lane, a different model even on the same vendor. */
  readonly embedModel: string;
  /** Which credential the vendor takes: a pasted key, or a key file. */
  readonly credential: "api_key" | "service_account";
  /** The profile onboarding binds under — the one this choice was offered as. */
  readonly profile: components["schemas"]["AiRouting"]["profile"];
  /** Where Google processes the calls, for the one vendor bound by location. */
  readonly location?: string;
};

/**
 * The ids, as a tuple so a caller can narrow a value THROUGH it rather than
 * asserting one INTO it. The table below is typed by these, which makes the two
 * a compile-time pair in both directions: an id here with no preset fails, and
 * a preset whose id is not here fails too.
 */
export const SETUP_PROVIDER_IDS = [
  "gemini",
  "openrouter",
  "gemini_vertex",
] as const;

export type SetupProviderId = (typeof SETUP_PROVIDER_IDS)[number];

const PRESETS: Readonly<Record<SetupProviderId, SetupProvider>> = {
  gemini: {
    label: "Google Gemini",
    provider: "gemini",
    keyEnv: "GEMINI_API_KEY",
    chatModel: "gemini-3.1-flash-lite",
    embedModel: "gemini-embedding-001",
    credential: "api_key",
    profile: "eu_hosted",
  },
  openrouter: {
    label: "OpenRouter",
    provider: "openai_compatible",
    baseUrl: "https://openrouter.ai/api",
    keyEnv: "OPENAI_COMPATIBLE_API_KEY",
    chatModel: "mistralai/mistral-small-3.2-24b-instruct",
    embedModel: "openai/text-embedding-3-small",
    credential: "api_key",
    profile: "eu_hosted",
  },
  // The one choice that PROMISES something: eu_resident refuses any binding
  // that processes outside the EU, and Vertex at `eu` is one that does not.
  gemini_vertex: {
    label: "Gemini on Vertex AI",
    provider: "gemini_vertex",
    keyEnv: "GEMINI_VERTEX_SA_JSON",
    chatModel: "gemini-3.1-flash-lite",
    embedModel: "gemini-embedding-001",
    credential: "service_account",
    profile: "eu_resident",
    location: "eu",
  },
};

export const SETUP_PROVIDERS: Readonly<Record<SetupProviderId, SetupProvider>> =
  PRESETS;
