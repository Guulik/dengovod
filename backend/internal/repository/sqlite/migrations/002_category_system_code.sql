ALTER TABLE categories ADD COLUMN system_code TEXT
    CHECK(system_code IS NULL OR (system_code = 'SALARY' AND kind = 'INCOME'));

UPDATE categories SET system_code = 'SALARY'
WHERE name = 'Зарплата' AND kind = 'INCOME' AND is_default = 1;

CREATE UNIQUE INDEX idx_categories_user_system_code
    ON categories(user_id, system_code) WHERE system_code IS NOT NULL;
