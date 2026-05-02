CREATE TABLE IF NOT EXISTS schema_migrations (
    version bigint PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS students (
    id bigserial PRIMARY KEY,
    full_name text NOT NULL,
    group_code text NOT NULL,
    telegram_id bigint UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT students_full_name_group_unique UNIQUE (full_name, group_code)
);

CREATE TABLE IF NOT EXISTS choices (
    id bigserial PRIMARY KEY,
    code text NOT NULL UNIQUE,
    title text NOT NULL,
    type text NOT NULL CHECK (type IN ('elective', 'required_choice', 'mobility')),
    deadline timestamptz NOT NULL,
    min_selected integer NOT NULL DEFAULT 0,
    max_selected integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS choice_groups (
    choice_id bigint NOT NULL REFERENCES choices(id) ON DELETE CASCADE,
    group_code text NOT NULL,
    PRIMARY KEY (choice_id, group_code)
);

CREATE TABLE IF NOT EXISTS choice_options (
    id bigserial PRIMARY KEY,
    choice_id bigint NOT NULL REFERENCES choices(id) ON DELETE CASCADE,
    title text NOT NULL,
    seats_limit integer NOT NULL CHECK (seats_limit >= 0),
    credits integer NOT NULL DEFAULT 0 CHECK (credits >= 0),
    info_url text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT choice_options_choice_title_unique UNIQUE (choice_id, title)
);

CREATE TABLE IF NOT EXISTS enrollments (
    id bigserial PRIMARY KEY,
    student_id bigint NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    option_id bigint NOT NULL REFERENCES choice_options(id) ON DELETE CASCADE,
    source text NOT NULL DEFAULT 'student',
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT enrollments_student_option_unique UNIQUE (student_id, option_id)
);

CREATE INDEX IF NOT EXISTS idx_students_group_code ON students(group_code);
CREATE INDEX IF NOT EXISTS idx_choice_groups_group_code ON choice_groups(group_code);
CREATE INDEX IF NOT EXISTS idx_choice_options_choice_id ON choice_options(choice_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_student_id ON enrollments(student_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_option_id ON enrollments(option_id);
