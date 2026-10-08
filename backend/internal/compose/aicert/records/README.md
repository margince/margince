<!-- prose:plain -->
# records/

The committed results that certify a model for a task, one file for each task, provider, model
and `env` together: `<task>/<provider>_<model>_<env>.json`. `WriteRecord` writes a file and
`LoadRecords` reads it. The JSON keeps a fixed field order and a line break after the last line,
so the same `Record` leaves no diff. The `verdict`, the counts and
the version fields keep the same value on every run. The mean time to answer, the token count and
the cost change with network noise on every run. So a diff in only these fields does not change
the result.
