<!-- prose:plain -->
# config: the files a program reads

This folder holds four kinds of file. `make config-gen` writes each generated file again, so an
edit to it is lost. A tracked file is written by hand and changed through a pull request. A file of
yours is ignored by git. A secret file is ignored by git too, and holds one value.

| File | Kind | What you do with it |
|---|---|---|
| [`margince.example.yaml`](margince.example.yaml) | generated | Copy it to start your own file. It holds only the workspace and the first admin. `make dev` copies it to `margince.yaml` on the first run. |
| [`margince.defaults.yaml`](margince.defaults.yaml) | generated | Copy a setting from it into your file. It holds every setting at its default. Compare your file with it to see what you changed. |
| [`margince.schema.json`](margince.schema.json) | generated | Leave it next to your file. An editor checks your file against it. |
| [`margince.dev.yaml`](margince.dev.yaml) | tracked | What a dev stack does differently. `make dev` reads it over `margince.yaml`. |
| [`presets/`](presets/README.md) | tracked | Copy a model binding (`seeds.ai_routing`) from it. No program reads the folder. |
| `margince.yaml` | yours | Edit it. The api, the worker and `cmd/migrate` read it as the base file. |
| `ai-routing.yaml` | yours | A model binding for the certification lanes, if you keep one. Nothing reads it unless you name it, as in `make e2e-ai ROUTING=config/ai-routing.yaml`. |
| `margince-admin-password` | secret | The first admin's password. `make dev` writes it on the first run, and `margince.yaml` names it. |
| `margince-setup-token` | secret | The one-time setup token the api writes when it first boots on an empty database with no `bootstrap_admin`. |
| `margince-license` | secret | A license token, if you keep one here. `license.token` names it as `${file:...}`. |

## The order a value is read in

A program reads the base file (`--config`, `margince.yaml` on a dev stack), then the file
`--config-file` names over it (`margince.dev.yaml` on a dev stack). The `.env` file, a variable and a
flag each win over both files. The full order, and how a secret is written, is in
[Where a value comes from](../docs/reference/configuration.md#where-a-value-comes-from).

Name a second YAML file only through `--config-file`. A file such as `margince.dev.yaml` beside the
base file stops the boot unless `--config-file` names it. So a copy left there by mistake stops the
boot instead of being read.
