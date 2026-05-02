package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr         string
	DatabaseDSN      string
	TelegramBotToken string
	AdminTelegramIDs map[int64]struct{}
	Timezone         *time.Location
	SeedOnStart      bool
	SeedStudentsFile string
	SeedChoicesFile  string
}

func Load() Config {
	loc, err := time.LoadLocation(getenv("APP_TIMEZONE", "Europe/Moscow"))
	if err != nil {
		loc = time.Local
	}
	return Config{
		HTTPAddr:         getenv("HTTP_ADDR", ":8080"),
		DatabaseDSN:      getenv("DATABASE_DSN", "postgres://vkr:vkr@localhost:5432/vkr?sslmode=disable"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		AdminTelegramIDs: parseAdminIDs(os.Getenv("ADMIN_TELEGRAM_IDS")),
		Timezone:         loc,
		SeedOnStart:      parseBool(os.Getenv("SEED_ON_START")),
		SeedStudentsFile: strings.TrimSpace(os.Getenv("SEED_STUDENTS_FILE")),
		SeedChoicesFile:  strings.TrimSpace(os.Getenv("SEED_CHOICES_FILE")),
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseAdminIDs(raw string) map[int64]struct{} {
	result := make(map[int64]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err == nil {
			result[id] = struct{}{}
		}
	}
	return result
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
