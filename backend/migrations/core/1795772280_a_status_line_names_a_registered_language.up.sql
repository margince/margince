-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A status line's language is a stored language code, so it names a row of the
-- language table like every other language column.
SET LOCAL lock_timeout = '3s';

ALTER TABLE standing_line
    ADD CONSTRAINT standing_line_language_fkey
        FOREIGN KEY (language) REFERENCES language(code) ON DELETE RESTRICT ON UPDATE RESTRICT NOT VALID;
