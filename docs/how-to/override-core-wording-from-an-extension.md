<!-- prose:plain -->
# Override core wording from an extension

Use this page to change a word or a sentence that the core shows, such as "Deals" to "Opportunities". No file the core owns changes. For wording on the unit's own screens, read the language part of [add-an-extension.md](add-an-extension.md) instead. An override is a core key with new text. The unit's own copy is a new key under the unit's own prefix.

## Steps

1. **Find the core key.** Core keys are the lines of `frontend/src/i18n/en.ts`, one key to a line. Search the English text you want to change and copy its key, for example `nav.deals`.
2. **Write one file for each language you change.** Put it at `extensions/<name>/frontend/i18n-overrides/<locale>.json`. Each file is one flat JSON object of core key to text:

   ```json
   { "nav.deals": "Opportunities" }
   ```

3. **Keep any parts of the text.** A value slot such as `{count}`, and each count form (`_one` and `_other`) that the core text has, must stay.
4. **Build.** `make gen` reads the files of every enabled unit and writes `frontend/src/composition/extlocales.gen.ts`, which is generated and never edited by hand.
5. **Run the frontend tests.** `make check-fe` runs the gates of `frontend/src/i18n/overrides.test.ts`.

## What the build decides

| Case | Result |
|---|---|
| A language has no override file | That language keeps the core's text. A file may cover one language only. |
| A key the core list does not have | `make gen` refuses it and names the unit, the file and the key. |
| Two units override one key in one language | `make gen` refuses it and names both units. |
| A consent, erasure, approval or sign-in key | Allowed. No key is exempt, so the installation owns what it changes. |
| A value slot or a count form is missing from the new text | The frontend test lane fails. |

The screen looks in the overrides first, then in the core list, then in the unit's own copy.

## What an override does not reach

An override changes frontend wording only. It does not rename a record type. It leaves AI prompts, agent tools, backend text, the offline page and the docs alone.

## Keep an override true

The installation owns the wording of every key it overrides, and the core does not carry its later changes into that key. At each core upgrade, read the changes to `en.ts` for the keys you override. Three limits hold whatever the gates say:

- **Grammar is yours.** A new word can need another article, case or count form. The gates check parts and count forms, not whether the sentence reads right.
- **Longer text can break a layout.** Buttons and table headers do not grow without limit.
- **An override can go stale.** If the core changes what a key means, your text still says the old thing.

The generator is `backend/tools/gen-composition/extoverrides.go`. The facts above are in its code and tests, not copied here.
