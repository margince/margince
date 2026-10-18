-- The mapped units stay keys: the free text they came from is not kept, and a
-- key still reads as the unit it names.

SET LOCAL lock_timeout = '3s';

ALTER TABLE offer_line_item ALTER COLUMN unit SET DEFAULT 'unit';
ALTER TABLE product ALTER COLUMN unit SET DEFAULT 'unit';
