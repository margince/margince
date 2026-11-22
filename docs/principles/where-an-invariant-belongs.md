<!-- prose:plain -->
# Where an invariant belongs

**The program checks a value first and tells the user what is wrong. The database holds the rule a
second time, for the writer that did not ask the program.**

Both layers have a job, and each job is one the other layer cannot do.

| Layer | Its job | Why the other layer cannot do it |
|---|---|---|
| The program | Check the value before the write, and answer with the field, a code and a sentence | The database answers with the name of a `CHECK`, and a user cannot read that |
| The database | Refuse a bad row from any writer | An import, a migration that corrects rows, a test fixture or a second program does not run the program's check |

## What the database holds

- **Keys and links:** each `PRIMARY KEY`, each `FOREIGN KEY` with what happens on delete and update,
  and `NOT NULL`. Each key that says two rows may not be the same is here too.
- **Rules about two rows at one time,** such as two meetings of one host that may not overlap. The
  program cannot hold these, because it does not see a write that runs at the same time.
- **Closed word lists.** A column that takes one word of a fixed list has a CHECK with that list.
- **One spelling of a value in a key.** An address or a domain in such a key is stored in one form,
  and a CHECK holds that form. A key over two spellings of one address holds it twice.
- **Defaults each row needs,** such as its id and the two times that say when it was written.

## What the program holds

- The same word lists and spellings, checked first, so the user reads a sentence and not an error.
- Format, range and rules across fields, each with a message the contract can state.

A rule that only the program holds is not held for an import or a migration that corrects rows. Accept that for a rule
about format. Do not accept it for a key, a link, a word list or a spelling in a key.

## How to check it

A rule in two places can drift, so a gate reads both and fails when they differ.

- `backend/gates/enumsync_test.go` reads each CHECK word list from the schema and the list the
  program has, and fails when the two are not the same.
- `backend/gates/canonicaladdresscolumns_test.go` fails an address or domain column in such a key
  that has no CHECK for its form.
- `backend/gates/updatedatshape_test.go` fails a column for the time of the last change that can be
  empty or has no default.

When you add a closed word list, add the CHECK and the entry in `enumsync_test.go` in the same change.

## What this does not ask for

- It does not ask for a CHECK on each format rule. A phone number or a URL is checked in the program.
- It does not ask you to drop a CHECK that is there. The ones in the schema stay as the second line.
- It does not make the database the place for the message. The program still answers the user.
