-- A product's and an offer line's unit is a key the client shows in the
-- reader's language: piece, hour, day, month or flat_fee. Any other text is a
-- unit somebody typed, and it stays as typed.
--
-- The rows written while the unit was free text are mapped to a key where
-- their spelling names one, in any of the three interface languages. A
-- spelling this list does not know keeps its text, so no unit is lost. The
-- old default, 'unit', becomes 'piece'.

SET LOCAL lock_timeout = '3s';

CREATE TEMPORARY TABLE unit_spelling (spelling text PRIMARY KEY, unit_key text NOT NULL) ON COMMIT DROP;

INSERT INTO unit_spelling (spelling, unit_key) VALUES
    ('unit', 'piece'), ('units', 'piece'), ('piece', 'piece'), ('pieces', 'piece'),
    ('pc', 'piece'), ('pcs', 'piece'), ('pc.', 'piece'), ('pcs.', 'piece'),
    ('each', 'piece'), ('ea', 'piece'), ('item', 'piece'), ('items', 'piece'),
    ('stück', 'piece'), ('stueck', 'piece'), ('stk', 'piece'), ('stk.', 'piece'),
    ('cái', 'piece'),
    ('hour', 'hour'), ('hours', 'hour'), ('h', 'hour'), ('hr', 'hour'), ('hrs', 'hour'),
    ('stunde', 'hour'), ('stunden', 'hour'), ('std', 'hour'), ('std.', 'hour'),
    ('giờ', 'hour'),
    ('day', 'day'), ('days', 'day'), ('person day', 'day'), ('person-day', 'day'),
    ('man day', 'day'), ('man-day', 'day'), ('tag', 'day'), ('tage', 'day'),
    ('personentag', 'day'), ('personentage', 'day'), ('pt', 'day'), ('ngày', 'day'),
    ('month', 'month'), ('months', 'month'), ('monat', 'month'), ('monate', 'month'),
    ('tháng', 'month'),
    ('flat fee', 'flat_fee'), ('flat-fee', 'flat_fee'), ('flat_fee', 'flat_fee'),
    ('flat rate', 'flat_fee'), ('lump sum', 'flat_fee'), ('fixed price', 'flat_fee'),
    ('pauschal', 'flat_fee'), ('pauschale', 'flat_fee'), ('festpreis', 'flat_fee'),
    ('trọn gói', 'flat_fee');

-- Only rows whose unit changes, so a row already holding its key keeps its
-- version and an editor open on it is not sent a conflict.
UPDATE product
   SET unit = s.unit_key
  FROM unit_spelling s
 WHERE lower(btrim(product.unit)) = s.spelling
   AND product.unit <> s.unit_key;

UPDATE offer_line_item
   SET unit = s.unit_key
  FROM unit_spelling s
 WHERE lower(btrim(offer_line_item.unit)) = s.spelling
   AND offer_line_item.unit <> s.unit_key;

ALTER TABLE product ALTER COLUMN unit SET DEFAULT 'piece';
ALTER TABLE offer_line_item ALTER COLUMN unit SET DEFAULT 'piece';
