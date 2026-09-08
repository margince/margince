-- The record type is called company, in the two tables this namespace owns.
--
-- Core renamed the record type and moved every value it owns. These two are
-- fork-local — upstream never writes here — and each files a row by the
-- record's runtime class, under an OPEN vocabulary that names no values. So
-- nothing in the schema says 'company' is now among them, and nothing would
-- have failed: an import would simply stop recognising the rows it had already
-- mapped, and the mirror would stop matching the records it can show.

UPDATE import_record_map SET object = 'company' WHERE object = 'organization';
UPDATE mirror_visibility SET object_class = 'company' WHERE object_class = 'organization';
