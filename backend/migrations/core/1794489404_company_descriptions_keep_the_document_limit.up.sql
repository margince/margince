SET LOCAL lock_timeout = '2s';
ALTER TABLE company DROP CONSTRAINT company_description_length;
ALTER TABLE company ADD CONSTRAINT company_description_length CHECK (description IS NULL OR length(description) <= 20000) NOT VALID;
