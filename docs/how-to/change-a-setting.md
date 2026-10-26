<!-- prose:plain -->
# Change a setting

Each setting has one name in YAML, one in the environment and sometimes a flag. The
[configuration reference](../reference/configuration.md) lists them all. Read
[where a value comes from](../reference/configuration.md#where-a-value-comes-from) for the order in
which the sources win.

## Find the file

The api and the worker read `margince.yaml`, or the file that `--config` or `MARGINCE_CONFIG` names.
On a dev stack it is `config/margince.yaml`. `make dev` makes it from
[`config/margince.example.yaml`](../../config/margince.example.yaml) on the first run. The dev stack
also reads `config/margince.dev.yaml` over it through `--config-file`, and a key set in the dev file
wins.

The example file sets the workspace and the first admin. Then it lists every other setting at its
default, each line turned off with `# `. To set one:

1. Find the setting in the file.
2. Remove the first `# ` from its line and from the section lines above it.
3. Change the value, and save the file.
4. Start the api and the worker again. A running program does not read the file a second time.

A secret takes a reference such as `${file:/run/secrets/smtp_password}`, never the value itself.

## Turn on checks as you type

The example file starts with this line:

```yaml
# yaml-language-server: $schema=./margince.schema.json
```

A program that runs `yaml-language-server` reads it and checks the file as you type. In VS Code, add the
**YAML** extension. The check marks a key that no program reads, a value of the wrong kind,
and a currency or language that the start would refuse. A wrong time zone or country shows only when
the api reads the file, at the start or with `--check-config`. Keep the line, and keep
`margince.schema.json` next to the file.

## Check before you start

`--check-config` reads every source as a start does, prints what it read and stops. It opens no
database, so a deploy pipeline can run it before a rollout. It still needs a DSN to be set, as a
start does, but it never connects to it.

Each program checks only the settings it reads, so run it on the api **and** the worker. The api's
check passes a file with a wrong worker setting; it lists that setting as `not checked here`.

On a dev stack, one command runs both:

```bash
make check-config
```

It reads `config/margince.yaml`, `config/margince.dev.yaml` and `.env.local`. To check other files,
set `CONFIG=`, `CONFIG_FILE=` or `ENV_FILE=`; an empty `CONFIG_FILE=` or `ENV_FILE=` leaves that
file out.

The images start through an entrypoint that runs migrations first, so call each program in its image
directly:

```bash
docker run --rm --entrypoint margince-api -e MARGINCE_DSN=postgres://localhost/check \
  -v "$PWD/config:/app/config" \
  margince-api:local --config config/margince.yaml --check-config
docker run --rm --entrypoint margince-worker -e MARGINCE_DSN=postgres://localhost/check \
  -v "$PWD/config:/app/config" \
  margince-worker:local --config config/margince.yaml --check-config
```

Each exits 0 when every setting it reads is valid. It exits 1 and lists every setting that is not
valid. Read [check mode](../reference/configuration.md#where-a-value-comes-from) for what it prints.

## See what Margince read

- Start the api or the worker with `--config-trace`, or set `MARGINCE_CONFIG_TRACE=true`. Read
  [trace mode](../reference/configuration.md#where-a-value-comes-from) for what the log shows.
- In the app, open **Settings**, then **Configuration**. It lists each setting with its value and its
  **Source**. In the settings search, a key or a `MARGINCE_` name opens the page at that setting.

## Change a locked setting

A setting that a flag, the environment, an env file or a YAML file sets is locked in the app. Its
control is turned off and names the place that sets it. The
[settings handbook](../handbook/settings.md#why-cant-i-change-a-setting) shows the message.

1. Change the value in the place that the message names.
2. Start the api and the worker again.

To change the setting in the app instead, remove it from that place and start again. A value saved in
the app before is used again, or the default if none was saved.
