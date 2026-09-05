-- Restore the generated column exactly as 0001 defined it, scales and all.
SET LOCAL lock_timeout = '5s';
ALTER TABLE deal DROP COLUMN amount_minor_base;
ALTER TABLE deal ADD COLUMN amount_minor_base bigint
  GENERATED ALWAYS AS ((round(((amount_minor)::numeric * fx_rate_to_base)))::bigint) STORED;
