<!-- prose:plain -->
# Testing a provider key

This page shows what the **Test** button on Settings → AI → Models asks each model provider, and how
to read the answer. Where keys are stored, and which settings fill them first, is in
[configuration.md](configuration.md).

`POST /v1/ai/provider-keys/{provider}/test` (the Test button on Settings → AI → Models) asks the
vendor the cheapest question that needs a key, using the key this installation stores. It takes no
body, never a new key to test, and never a host. The host is the one a stored binding names for that
provider, or the default host of the adapter. No call costs money, and the vendor's own error text
never reaches the answer.

| provider | what Test sends | pass |
|---|---|---|
| `anthropic` | `GET {base}/v1/models`, `x-api-key` | 200, with the model count |
| `openai`, `openai_compatible`, `vllm` | `GET {base}/v1/models`, `Authorization: Bearer` (no key for a vLLM server without one) | 200, with the model count |
| `gemini` | `GET {base}/models`, `x-goog-api-key`, every page | 200, with the model count |
| `ollama` | `GET {base}/api/tags`, no key | 200, with the count of models on the server |
| `jev` | `GET /v1/models` next to its endpoint (the TypeSafe model list), `Authorization: Bearer`, on the host the endpoint names, OpenRouter included | 200, with the model count |
| `jev_compatible` on `openrouter.ai` | `GET https://openrouter.ai/api/v1/key`, `Authorization: Bearer` | 200, no count |
| `jev_compatible` on another host | `POST {endpoint}` with body `{}`, `Authorization: Bearer` when a key is stored | 400 or 422, no count, reported as **not confirmed** (`key_confirmed: false`). A 200 fails: no Jev server answers an empty request |

The two `jev_compatible` rows are not the same because "compatible" promises only the Jev decision
route. The OpenRouter model list is public and answers 200 to every key, so Test asks its key
endpoint instead. A server you run on your own shows only the decision route. It refuses a request
that names no model before it decides anything. Its 400 or 422 means the server answered and did
not refuse the key, but it may have refused the empty body before it read the key. A 200 from that
address comes from something that is not a Jev server.

With no key stored, a pass on your own server reads "The server answered": no key was sent, so no
key was accepted. With a key stored, the screen says the server did not refuse it, and that a wrong
key would show at the first decision.

A failed test answers `ok: false` with one `reason`:

| reason | meaning |
|---|---|
| `no_key` | the provider takes a key and no key is stored |
| `profile_forbids` | the installation profile does not let Margince reach this provider: a cloud provider under `sovereign` |
| `no_endpoint` | `openai_compatible` or `jev_compatible` with no binding that names a host yet; add one first |
| `auth_failed` | the vendor refused the key: 401 or 403, or a Gemini 400 `API_KEY_INVALID` |
| `rate_limited` | 429: the vendor is slowing the key down, and the key may still be good |
| `unreachable` | no answer within 5 seconds, a `5xx`, a redirect, or some other status |
| `not_published` | an adapter this build does not have |

The models the routing editor lists (`GET /v1/ai/available-models/{provider}`) come from the same
places. For `jev` they come from the TypeSafe list. For `jev_compatible` on OpenRouter they come
from its catalog, asked with `output_modalities=all` (without it, the catalog leaves out Jev), and
then cut down to `typesafe/*`. A decision server you run on your own shows no list, so the rows of
the price sheet are the models it lists.
