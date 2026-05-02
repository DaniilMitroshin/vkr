# VKR Choice Bot

Система распределения студентов по дисциплинам: сервер на Go, PostgreSQL и клиент в виде Telegram-бота.

## Что реализовано

- импорт студентов из CSV и JSON, включая исходный JSON-подобный файл из `input`;
- импорт выборов и дисциплин из CSV и JSON;
- типы выборов `required_choice`, `elective`, `mobility`;
- контроль дедлайна, мест, количества дисциплин и суммы зачетных единиц;
- автоматическое распределение студентов без выбора для `required_choice`;
- REST API для проверки и интеграций;
- Telegram-бот для студентов и администратора;
- экспорт результатов в CSV/JSON;
- генерация заявления студента в `.docx`;
- хранение данных в PostgreSQL с сохранением между перезапусками Docker.

## Быстрый запуск через Docker

1. Создайте `.env`:

```bash
cp .env.example .env
```

2. Укажите в `.env` токен Telegram-бота и Telegram ID администратора:

```env
TELEGRAM_BOT_TOKEN=123456:telegram-token
ADMIN_TELEGRAM_IDS=123456789
```

Если токен оставить пустым, запустится только REST API.

Опционально можно включить автоимпорт на старте (после миграций):

```env
SEED_ON_START=true
SEED_STUDENTS_FILE=input/Контингент_5130904_201.json
SEED_CHOICES_FILE=input/Дисциплины_пример.json
```

Если `SEED_ON_START=false`, поведение остаётся прежним: импорт вручную через REST/бот.

3. Запустите проект:

```bash
docker compose up --build
```

REST API будет доступен на `http://localhost:8080`.

## Импорт тестовых данных через REST

```bash
curl -F "file=@input/Контингент_5130904_301.csv" http://localhost:8080/api/import/students
curl -F "file=@input/Дисциплины_пример.json" http://localhost:8080/api/import/choices
curl http://localhost:8080/api/students?limit=5
curl http://localhost:8080/api/choices
```

Регистрация студента через REST:

```bash
curl -X POST http://localhost:8080/api/students/register \
  -H "Content-Type: application/json" \
  -d '{"telegram_id":1001,"full_name":"Митрошин Даниил Викторович","group_code":"/20102"}'
```

После регистрации можно получить доступные выборы:

```bash
curl http://localhost:8080/api/students/1/choices
curl http://localhost:8080/api/choices/REQ-SEM7-AI/options
```

Подать выбор, подставив реальный `student_id` и `option_id`:

```bash
curl -X POST http://localhost:8080/api/students/1/choices/REQ-SEM7-AI/submit \
  -H "Content-Type: application/json" \
  -d '{"option_ids":[1]}'
```

Экспорт и заявление:

```bash
curl "http://localhost:8080/api/export/results?format=csv" -o results.csv
curl "http://localhost:8080/api/students/1/application.docx" -o application.docx
```

## Команды Telegram-бота

Студент:

- `/register` - регистрация по ФИО и группе;
- `/choices` - список доступных выборов;
- `/my` - текущие записи;
- `/statement` - получить заявление `.docx`.

Администратор:

- отправить CSV/JSON файл с подписью `/import_students`;
- отправить CSV/JSON файл с подписью `/import_choices`;
- `/students` - первые студенты в базе;
- `/auto REQ-SEM7-AI` - автоматическое распределение обязательного выбора;
- `/export_csv` - выгрузка результатов.

## Локальный запуск без Docker

Нужен PostgreSQL и Go. Укажите DSN для локальной базы:

```bash
export DATABASE_DSN='postgres://vkr:vkr@localhost:5432/vkr?sslmode=disable'
export HTTP_ADDR=':8080'
go run ./cmd/app
```

## Проверка

```bash
go test ./...
```
