-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A configured mask goes with the offer it names. The foreign key refuses the
-- catalog row's removal while one stands, and a mask outliving the pair would
-- claim a withholding the catalog no longer admits.
DELETE FROM field_mask WHERE object = 'partner' AND field = 'margin_tier';

DELETE FROM maskable_field WHERE object = 'partner' AND field = 'margin_tier';
