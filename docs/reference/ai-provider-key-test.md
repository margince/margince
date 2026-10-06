# Testing a provider key

What Settings → AI → Models' **Test** button asks each model provider, and how
to read the answer. Where keys are stored and which variables seed them is in
[configuration.md](configuration.md).

`POST /v1/ai/provider-keys/{provider}/test` (the Test button on Settings → AI →
Models) asks the vendor, with the key this installation stores, the cheapest
authenticated question it answers. It takes no body, never a candidate key, and
never a host: the host is the one a stored binding names for that provider, or
the adapter's default. No call is billed, and the vendor's own error text never
reaches the answer.

| provider | what Test sends | pass |
|---|---|---|
| `anthropic` | `GET {base}/v1/models`, `x-api-key` | 200, with the model count |
| `openai`, `openai_compatible`, `vllm` | `GET {base}/v1/models`, `Authorization: Bearer` (none for a keyless vLLM) | 200, with the model count |
| `gemini` | `GET {base}/models`, `x-goog-api-key`, every page | 200, with the model count |
| `ollama` | `GET {base}/api/tags`, no auth | 200, with the count of pulled models |
| `jev` | `GET /v1/models` beside its endpoint (TypeSafe's model list), `Authorization: Bearer`, on whatever host the endpoint names, OpenRouter included | 200, with the model count |
| `jev_compatible` on `openrouter.ai` | `GET https://openrouter.ai/api/v1/key`, `Authorization: Bearer` | 200, no count |
| `jev_compatible` elsewhere | `POST {endpoint}` with body `{}`, `Authorization: Bearer` when a key is held | 400 or 422, no count, reported as **unconfirmed** (`key_confirmed: false`). A 200 fails: no Jev server answers an empty request |

The two `jev_compatible` rows differ because "compatible" promises only the Jev
decision route. OpenRouter's model list is public and answers 200 to any key, so
Test asks its key endpoint instead. A self-hosted server publishes only the
decision route and refuses a request that names no model before anything is
decided. Its 400 or 422 means the server answered and did not refuse the key,
but it may have refused the empty body before reading the key. A 200 from that
address is something other than a Jev server.

With no key stored, a self-hosted pass reads "The server answered": no key was
sent, so none was accepted. With a key held, the screen says the server did not
refuse it and that a wrong key would show at the first decision.

A failure answers `ok: false` with one `reason`:

| reason | meaning |
|---|---|
| `no_key` | the provider takes a key and none is stored |
| `profile_forbids` | the installation profile forbids reaching this provider: a cloud provider under `sovereign`, or under `eu_hosted` a decision lane the routing validator refuses (`jev`, or `jev_compatible` on OpenRouter) |
| `no_endpoint` | `openai_compatible` or `jev_compatible` with no binding naming a host yet; bind one first |
| `auth_failed` | the vendor refused the key: 401 or 403, or Gemini's 400 `API_KEY_INVALID` |
| `rate_limited` | 429: the vendor is throttling the key, which may still be valid |
| `unreachable` | no answer within 5 seconds, a 5xx, a redirect, or any other status |
| `not_published` | an adapter this build does not carry |

The routing editor's model suggestions (`GET /v1/ai/available-models/{provider}`)
come from the same places: TypeSafe's list for `jev`, and for `jev_compatible` on
OpenRouter the broker's catalogue (asked with `output_modalities=all`, without
which it omits Jev) narrowed to `typesafe/*`. A self-hosted decision server
publishes no list, and the price sheet's rows are its suggestions.
