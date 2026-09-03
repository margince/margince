# config/presets — ready-made model bindings

A preset is a `seeds.ai_routing` block you can copy into your own
`config/margince.yaml`, or point a certification run at with
`make e2e-ai ROUTING=config/presets/<name>.yaml`.

Presets are **not** read automatically. Nothing in the product loads this
directory: `MARGINCE_ENV` selects `margince.<env>.yaml` and no more. That is
deliberate — a file that binds a cloud vendor must be chosen, not inherited,
because the choice decides where this installation's text goes.

| preset | binds | needs |
|---|---|---|
| [`gemini_cloud.yaml`](gemini_cloud.yaml) | every tier to Gemini, embeddings to `gemini-embedding-001` | `GEMINI_API_KEY` |
| [`openrouter_cloud.yaml`](openrouter_cloud.yaml) | every tier to an OpenRouter-brokered model | `OPENAI_COMPATIBLE_API_KEY` |

`gemini_cloud.yaml` is the binding a dev stack bootstraps with today, lifted out
of `margince.dev.yaml` so it can be named and reused. The dev overlay still
carries its own copy — that file is the dev posture and has to stand alone.

On the OpenRouter preset's `routing:` block, and the measurements behind its
defaults: [docs/reference/openrouter.md](../../docs/reference/openrouter.md).
