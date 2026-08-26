/**
 * The providers onboarding offers, and what each one starts bound to.
 *
 * A DELIBERATE MIRROR of the server, not a second answer: every model named
 * here must carry a rate row in `SeedModelRates`, or a call on it reports
 * UNPRICED — a materially different signal from free, and a poor thing to hand
 * somebody in their first five minutes. `backend/frontendsetupproviders_test.go`
 * compares both directions.
 *
 * WHY ONLY TWO. A routing document REQUIRES an embeddings binding, and the two
 * listed here are the vendors that serve chat and embeddings from ONE key.
 * Anthropic publishes no embedding model at all and OpenAI's is not in the
 * price sheet, so offering either would walk a first-time admin into a form
 * they cannot complete, or quietly bind their retrieval lane to a fake. Both
 * remain available in Settings → AI, where an operator who already has two
 * vendor accounts can bind the lanes separately.
 *
 * WHY DEFAULTS RATHER THAN A CLOSED PICKER. The server accepts any model id its
 * vendor offers and publishes no catalogue, so a fixed dropdown would be this
 * file inventing a list and calling it the truth — wrong the week a vendor ships
 * a model. These are a starting point, and the field can be typed over.
 *
 * OpenRouter is a PRESET rather than a fifth adapter: on the wire it is
 * `openai_compatible`, which fails closed without a `base_url`, and asking an
 * admin to know that is asking them to know our adapter names.
 */
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
};

const PRESETS = {
  gemini: {
    label: "Google Gemini",
    provider: "gemini",
    keyEnv: "GEMINI_API_KEY",
    chatModel: "gemini-3.1-flash-lite",
    embedModel: "gemini-embedding-001",
  },
  openrouter: {
    label: "OpenRouter",
    provider: "openai_compatible",
    baseUrl: "https://openrouter.ai/api",
    keyEnv: "OPENAI_COMPATIBLE_API_KEY",
    chatModel: "mistralai/mistral-small-3.2-24b-instruct",
    embedModel: "openai/text-embedding-3-small",
  },
} as const;

export type SetupProviderId = keyof typeof PRESETS;

// Typed as the shared shape rather than left as the literal union: a caller
// reading `baseUrl` should not have to know which entries happen to carry one,
// which is the difference between an optional field and two different types.
export const SETUP_PROVIDERS: Readonly<Record<SetupProviderId, SetupProvider>> =
  PRESETS;

export const SETUP_PROVIDER_IDS = Object.keys(
  SETUP_PROVIDERS,
) as readonly SetupProviderId[];
