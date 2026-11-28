CREATE FUNCTION identifier_spelling(value text) RETURNS text
 LANGUAGE sql IMMUTABLE PARALLEL SAFE
 RETURN normalize(btrim(drop_hidden_characters(value), U&'\0020\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'), NFC);

CREATE FUNCTION fold_identifier(value text) RETURNS text
 LANGUAGE sql IMMUTABLE PARALLEL SAFE
 RETURN identifier_spelling(lower(value));

-- Own domains have a text primary key. An indexed opaque cursor keeps the
-- domain itself out of correction progress.
CREATE FUNCTION identifier_spelling_row_id(value text) RETURNS uuid
 LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
 RETURN substr(encode(sha256(convert_to(value, 'UTF8')), 'hex'), 1, 32)::uuid;
