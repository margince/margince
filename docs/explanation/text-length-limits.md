<!-- prose:plain -->
<!-- describes: backend/internal/shared/kernel/values/textlimits.go backend/internal/platform/database/textbounds backend/internal/modules/customfields/engine.go -->
# Why text has a shared limit

A name needs less space than a document. Shared
[text limits](../../backend/internal/shared/kernel/values/textlimits.go) give each kind one bound.
The [column rules](../../backend/internal/platform/database/textbounds/columns.txt) say which bound
each text column uses. A rule may leave a field out only when it gives a reason. Complete mail and
other source text can keep their full contents.

The database checks text when it is first stored or changed. A long value saved before the rule
stays as it is. A change to another field does not force it to be cut. The check counts characters,
so text in another language gets the same room.

The SQL writer reads the [schema catalog](../../backend/migrations/testdata/head_catalog.txt).
A gate checks that every text column has a rule and that its write check is in the schema.
A new column without a rule fails that gate. A table that a custom migration
creates gets its checks from a custom migration, because custom migrations run after the core ones. The web app can use the
[generated limits](../../frontend/src/lib/textlimits_gen.ts), which come from the same Go values.

Each check reads the whole row instead of naming its columns, so a later migration can still drop a
column or change its type. The check on `activity` is the one exception. A copy of that row carries the
mail body, which has no limit, so its check names the limited columns and compares only those. The
database then refuses to drop one of them, or to change its type, while the check names it. Such a
migration [takes the check off first](../how-to/apply-migrations.md#change-a-limited-text-column-of-activity).

Custom text fields use the document bound. New fields get a check when they are added. Fields
that already exist get checks for changed values, without a scan or a rewrite of the saved text.
