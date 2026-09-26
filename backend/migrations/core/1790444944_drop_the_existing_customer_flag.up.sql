-- The UWG §7(3) existing-customer flag is not a feature Margince offers. No
-- production path ever wrote a row, and three of its four conditions were free
-- text nothing compared. The exception now refuses outright in the verdict; a
-- later version that offers it adds its table beside the writer that fills it.
SET LOCAL lock_timeout = '3s';

DROP TABLE consent_existing_customer_flag;
