CREATE OR REPLACE FUNCTION fold_identifier(value text) RETURNS text
 LANGUAGE sql IMMUTABLE PARALLEL SAFE
 RETURN identifier_spelling(lower(value));
