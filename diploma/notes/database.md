# База данных

## Источник схемы

Схема БД описана SQL-миграциями в каталоге `migrations/`. Миграции выполняются автоматически при старте приложения функцией `RunMigrations` из `internal/repository/postgres.go`.

## Таблицы

| Таблица | Назначение |
|---|---|
| `schema_migrations` | Учет примененных миграций |
| `students` | Хранение студентов, групп и Telegram ID |
| `choices` | Хранение сущностей выбора |
| `choice_groups` | Связь выбора с группами студентов |
| `choice_options` | Варианты дисциплин внутри выбора |
| `enrollments` | Записи студентов на выбранные дисциплины |
| `admins` | Telegram ID администраторов |

## Основные связи

- `choices` 1:N `choice_options`
- `choices` 1:N `choice_groups`
- `students` 1:N `enrollments`
- `choice_options` 1:N `enrollments`
- `enrollments` связывает студента с выбранным вариантом дисциплины

## Основные ограничения

- `students.telegram_id` уникален.
- Пара `students.full_name_key`, `students.group_code_key` уникальна.
- `choices.code` уникален.
- `choices.type` ограничен значениями `elective`, `required_choice`, `mobility`.
- `choice_options.seats_limit >= 0`.
- `choice_options.credits >= 0`.
- Пара `choice_options.choice_id`, `choice_options.option_title_key` уникальна.
- Пара `enrollments.student_id`, `enrollments.option_id` уникальна.

## Текстовое описание ER-диаграммы

ER-диаграмма должна содержать сущности `Student`, `Choice`, `ChoiceOption`, `ChoiceGroup`, `Enrollment`, `Admin` и `SchemaMigration`. Центральная связь проходит через таблицу `enrollments`, которая связывает студентов с конкретными вариантами дисциплин. Таблица `choice_groups` задает доступность выбора для учебных групп. Таблица `admins` хранит Telegram ID пользователей с административными правами.
