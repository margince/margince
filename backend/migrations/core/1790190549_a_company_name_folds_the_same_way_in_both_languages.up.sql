SET LOCAL lock_timeout = '5s';

-- The CSV import's company-name fold, as a function the database can index.
--
-- The import resolves a spreadsheet's company column against the estate. It
-- did that by loading EVERY visible company into a Go map, once per dry run and
-- again per commit: a ten-row file against a workspace holding hundreds of
-- thousands of companies read all of them, and a run that linked nothing paid
-- the same price as one that linked everything. The upload cap bounds the file,
-- never the estate.
--
-- Asking per distinct name in the file is bounded by the file, and needs an
-- index on the folded name or it is one sequential scan per name instead of one
-- per run — the trade the old shape was avoiding.
--
-- The fold is DELIBERATELY WEAKER than the dedupe fold beside it. That one
-- strips legal suffixes so a review queue can ask a human whether `Acme Inc`
-- and `Acme GmbH` are one company; answering that question by importing would
-- link a contact to a company nobody said was the same. Case and whitespace
-- only: a file spelling a company differently from the CRM leaves a missing
-- link the report names, never a wrong link nobody sees.
--
-- \u00a0 IS IN THE CLASS BECAUSE [[:space:]] DOES NOT MATCH IT. Postgres reads
-- the em space and the ideographic space as space and the NO-BREAK space as an
-- ordinary character; Go's unicode.IsSpace, which employerKey folds through,
-- reads all three as space. Without the escape a company stored with a
-- no-break space in its name is unreachable by a file that spells it the same
-- way, and the run reports a missing link — which reads exactly like a company
-- nobody has. TestTheImportNameFoldAgreesInBothLanguages found it and holds it.
--
-- IMMUTABLE is what makes it indexable and is honest here: lower() is
-- collation-dependent in general, and this repeats the same bargain every
-- lower() index in this schema already makes. STRICT so a NULL legal_name
-- folds to NULL rather than the empty string, which would make every company
-- without one answer to the same key.
CREATE FUNCTION f_fold_import_name(text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN lower(btrim(regexp_replace($1, '[[:space:]\u00a0]+', ' ', 'g')));

-- One index per name the import reads, partial on the same predicate the
-- lookup carries: an archived or merged-away company is not a link target, and
-- excluding them here keeps the index the size of the live estate.
CREATE INDEX idx_company_import_display_name
    ON company (f_fold_import_name(display_name))
 WHERE archived_at IS NULL AND merged_into_id IS NULL;

CREATE INDEX idx_company_import_legal_name
    ON company (f_fold_import_name(legal_name))
 WHERE legal_name IS NOT NULL AND archived_at IS NULL AND merged_into_id IS NULL;
