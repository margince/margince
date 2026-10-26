-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

UPDATE role SET permissions = permissions #- '{objects,deal}'
    WHERE is_system AND key = 'service_ingest';
