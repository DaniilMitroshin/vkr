CREATE TABLE IF NOT EXISTS admins (
    telegram_id bigint PRIMARY KEY,
    added_by bigint,
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE students ADD COLUMN IF NOT EXISTS full_name_key text;
ALTER TABLE students ADD COLUMN IF NOT EXISTS group_code_key text;

UPDATE students
SET full_name_key = lower(regexp_replace(replace(btrim(full_name), 'ё', 'е'), '\s+', ' ', 'g')),
    group_code_key = '/' || ltrim(btrim(group_code), '/')
WHERE full_name_key IS NULL OR group_code_key IS NULL;

WITH ranked AS (
    SELECT id, min(id) OVER (PARTITION BY full_name_key, group_code_key) AS keep_id
    FROM students
),
duplicate_enrollments AS (
    SELECT e.id
    FROM enrollments e
    JOIN ranked r ON r.id = e.student_id
    WHERE r.id <> r.keep_id
      AND EXISTS (
          SELECT 1
          FROM enrollments existing
          WHERE existing.student_id = r.keep_id
            AND existing.option_id = e.option_id
      )
)
DELETE FROM enrollments e
USING duplicate_enrollments d
WHERE e.id = d.id;

WITH ranked AS (
    SELECT id, min(id) OVER (PARTITION BY full_name_key, group_code_key) AS keep_id
    FROM students
)
UPDATE enrollments e
SET student_id = r.keep_id
FROM ranked r
WHERE e.student_id = r.id
  AND r.id <> r.keep_id;

WITH ranked AS (
    SELECT id, min(id) OVER (PARTITION BY full_name_key, group_code_key) AS keep_id
    FROM students
)
DELETE FROM students s
USING ranked r
WHERE s.id = r.id
  AND r.id <> r.keep_id;

ALTER TABLE students ALTER COLUMN full_name_key SET NOT NULL;
ALTER TABLE students ALTER COLUMN group_code_key SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS students_full_name_key_group_code_key_unique
    ON students(full_name_key, group_code_key);

ALTER TABLE choices ADD COLUMN IF NOT EXISTS program_name text NOT NULL DEFAULT '';
ALTER TABLE choices ADD COLUMN IF NOT EXISTS program_head text NOT NULL DEFAULT '';

ALTER TABLE choice_options ADD COLUMN IF NOT EXISTS option_title_key text;
ALTER TABLE choice_options ADD COLUMN IF NOT EXISTS semester text NOT NULL DEFAULT '';
ALTER TABLE choice_options ADD COLUMN IF NOT EXISTS teacher_name text NOT NULL DEFAULT '';

UPDATE choice_options
SET option_title_key = lower(regexp_replace(replace(btrim(title), 'ё', 'е'), '\s+', ' ', 'g'))
WHERE option_title_key IS NULL;

WITH ranked AS (
    SELECT id, min(id) OVER (PARTITION BY choice_id, option_title_key) AS keep_id
    FROM choice_options
),
duplicate_enrollments AS (
    SELECT e.id
    FROM enrollments e
    JOIN ranked r ON r.id = e.option_id
    WHERE r.id <> r.keep_id
      AND EXISTS (
          SELECT 1
          FROM enrollments existing
          WHERE existing.student_id = e.student_id
            AND existing.option_id = r.keep_id
      )
)
DELETE FROM enrollments e
USING duplicate_enrollments d
WHERE e.id = d.id;

WITH ranked AS (
    SELECT id, min(id) OVER (PARTITION BY choice_id, option_title_key) AS keep_id
    FROM choice_options
)
UPDATE enrollments e
SET option_id = r.keep_id
FROM ranked r
WHERE e.option_id = r.id
  AND r.id <> r.keep_id;

WITH ranked AS (
    SELECT id, min(id) OVER (PARTITION BY choice_id, option_title_key) AS keep_id
    FROM choice_options
)
DELETE FROM choice_options o
USING ranked r
WHERE o.id = r.id
  AND r.id <> r.keep_id;

ALTER TABLE choice_options ALTER COLUMN option_title_key SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS choice_options_choice_title_key_unique
    ON choice_options(choice_id, option_title_key);
