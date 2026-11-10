<!-- prose:plain -->
# Turn off core features

Set `features.disabled` in `margince.yaml` on the API and worker, then restart both.
The [configuration registry](../reference/configuration.md) lists the file settings.
There is no switch for this in Settings, on the command line or in the environment.

For example, to turn off Deals and Projects:

```yaml
features:
  disabled:
    - deals
    - projects
```

The allowed names are the `CoreFeature` values in [the API contract](../../backend/api/crm.yaml).
An unknown name stops startup with an error. An empty list keeps every core feature on.

A feature that is off has no main-menu entry or command-palette entry. Its pages,
API routes and agent calls refuse access. Other pages omit sections and reports
that need it. Existing role grants and data stay in place. Removing a name from the
list and restarting both programs makes that feature available under the same grants.

These are file settings for the whole installation. They do not add workspace settings
or erase data. Privacy, erasure and retention still work on the records kept there.
