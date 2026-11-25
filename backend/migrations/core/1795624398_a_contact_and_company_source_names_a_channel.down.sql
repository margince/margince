SET LOCAL lock_timeout = '3s';

DROP TRIGGER company_source_channel ON company;
DROP TRIGGER contact_source_channel ON contact;
DROP FUNCTION move_party_source_onto_its_channel();
DROP FUNCTION party_source_channel(text, text);
DROP FUNCTION party_source_producer(text);
ALTER TABLE company DROP CONSTRAINT company_source_check;
ALTER TABLE contact DROP CONSTRAINT contact_source_check;
