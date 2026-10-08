<!-- prose:plain -->
# custom/: the folder for a fork's own migrations

Margince ships this folder empty. The migrations a fork's agents write go here, as an
`<YYYYMMDDHHMMSS>_<name>.up.sql` file and a `.down.sql` file for each one. The table
`schema_migrations_custom` tracks them, and they run after every `core/` migration.
A custom column's name starts with `x_`, so it can never have the same name as a column
that a new Margince version adds.
