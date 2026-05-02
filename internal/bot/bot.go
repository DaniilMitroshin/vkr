package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"vkr/internal/domain"
	"vkr/internal/service"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	svc     *service.Service
	admins  map[int64]struct{}
	mu      sync.Mutex
	reg     map[int64]registration
	pending map[int64]map[string]map[int64]struct{}
}

type registration struct {
	Step     string
	FullName string
}

func New(token string, svc *service.Service, admins map[int64]struct{}) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	return &Bot{
		api:     api,
		svc:     svc,
		admins:  admins,
		reg:     make(map[int64]registration),
		pending: make(map[int64]map[string]map[int64]struct{}),
	}, nil
}

func (b *Bot) Run(ctx context.Context) error {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30
	updates := b.api.GetUpdatesChan(updateConfig)
	go func() {
		<-ctx.Done()
		b.api.StopReceivingUpdates()
	}()
	for update := range updates {
		if update.Message != nil {
			b.handleMessage(ctx, update.Message)
		}
		if update.CallbackQuery != nil {
			b.handleCallback(ctx, update.CallbackQuery)
		}
	}
	return ctx.Err()
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	if msg.Document != nil {
		b.handleDocument(ctx, msg)
		return
	}
	if msg.IsCommand() {
		switch msg.Command() {
		case "start", "help":
			b.send(msg.Chat.ID, helpText(b.isAdmin(msg.From.ID)))
		case "register":
			b.startRegistration(msg.Chat.ID)
		case "choices":
			b.sendChoices(ctx, msg.Chat.ID, msg.From.ID)
		case "my":
			b.sendEnrollments(ctx, msg.Chat.ID, msg.From.ID)
		case "statement":
			b.sendStatement(ctx, msg.Chat.ID, msg.From.ID)
		case "admin":
			b.send(msg.Chat.ID, adminText())
		case "students":
			b.adminStudents(ctx, msg)
		case "auto":
			b.adminAuto(ctx, msg)
		case "export_csv":
			b.adminExportCSV(ctx, msg)
		default:
			b.send(msg.Chat.ID, "Неизвестная команда. Нажмите /help.")
		}
		return
	}
	if b.continueRegistration(ctx, msg) {
		return
	}
	b.send(msg.Chat.ID, "Выберите команду: /choices, /my, /statement или /help.")
}

func (b *Bot) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	parts := strings.Split(cb.Data, ":")
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "choice":
		if len(parts) == 2 {
			b.showChoice(ctx, cb.Message.Chat.ID, cb.From.ID, parts[1])
		}
	case "pick":
		if len(parts) == 3 {
			optionID, _ := strconv.ParseInt(parts[2], 10, 64)
			b.pickOption(ctx, cb.Message.Chat.ID, cb.From.ID, parts[1], optionID)
		}
	case "submit":
		if len(parts) == 2 {
			b.submitPending(ctx, cb.Message.Chat.ID, cb.From.ID, parts[1])
		}
	}
	_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
}

func (b *Bot) startRegistration(chatID int64) {
	b.mu.Lock()
	b.reg[chatID] = registration{Step: "full_name"}
	b.mu.Unlock()
	b.send(chatID, "Введите ФИО точно как в списке студентов.")
}

func (b *Bot) continueRegistration(ctx context.Context, msg *tgbotapi.Message) bool {
	b.mu.Lock()
	state, ok := b.reg[msg.Chat.ID]
	b.mu.Unlock()
	if !ok {
		return false
	}
	switch state.Step {
	case "full_name":
		state.FullName = strings.TrimSpace(msg.Text)
		state.Step = "group"
		b.mu.Lock()
		b.reg[msg.Chat.ID] = state
		b.mu.Unlock()
		b.send(msg.Chat.ID, "Введите номер группы, например /20102 или 20102.")
	case "group":
		student, err := b.svc.RegisterStudent(ctx, msg.From.ID, state.FullName, msg.Text)
		if err != nil {
			b.send(msg.Chat.ID, "Не получилось зарегистрироваться: "+err.Error())
			return true
		}
		b.mu.Lock()
		delete(b.reg, msg.Chat.ID)
		b.mu.Unlock()
		b.send(msg.Chat.ID, fmt.Sprintf("Готово, %s, группа %s. Теперь можно открыть /choices.", student.FullName, student.GroupCode))
	}
	return true
}

func (b *Bot) sendChoices(ctx context.Context, chatID, telegramID int64) {
	student, err := b.svc.CurrentStudent(ctx, telegramID)
	if err != nil {
		b.send(chatID, "Сначала зарегистрируйтесь через /register.")
		return
	}
	choices, err := b.svc.StudentChoices(ctx, student.ID)
	if err != nil {
		b.send(chatID, "Не удалось получить выборы: "+err.Error())
		return
	}
	if len(choices) == 0 {
		b.send(chatID, "Для вашей группы пока нет доступных выборов.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, choice := range choices {
		title := fmt.Sprintf("%s (%s, до %s)", choice.Title, choice.Type, choice.Deadline.Format("02.01.2006"))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(title, "choice:"+choice.Code)))
	}
	msg := tgbotapi.NewMessage(chatID, "Доступные выборы:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, _ = b.api.Send(msg)
}

func (b *Bot) showChoice(ctx context.Context, chatID, telegramID int64, code string) {
	student, err := b.svc.CurrentStudent(ctx, telegramID)
	if err != nil {
		b.send(chatID, "Сначала зарегистрируйтесь через /register.")
		return
	}
	choices, err := b.svc.StudentChoices(ctx, student.ID)
	if err != nil || !choiceAllowed(choices, code) {
		b.send(chatID, "Этот выбор недоступен для вашей группы.")
		return
	}
	choice, err := b.svc.Choice(ctx, code)
	if err != nil {
		b.send(chatID, "Выбор не найден: "+err.Error())
		return
	}
	options, err := b.svc.ChoiceOptions(ctx, code)
	if err != nil {
		b.send(chatID, "Не удалось получить дисциплины: "+err.Error())
		return
	}
	lines := []string{fmt.Sprintf("%s\nТип: %s\nОграничение: %s\nДедлайн: %s", choice.Title, choice.Type, limitText(choice), choice.Deadline.Format("02.01.2006 15:04"))}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, option := range options {
		free := option.SeatsLimit - option.Occupied
		label := fmt.Sprintf("%s | мест: %d/%d", option.Title, free, option.SeatsLimit)
		if option.Credits > 0 {
			label += fmt.Sprintf(" | %d з.е.", option.Credits)
		}
		lines = append(lines, label)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("pick:%s:%d", code, option.ID))))
	}
	if choice.Type == domain.ChoiceTypeMobility || choice.MaxSelected > 1 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("Подтвердить выбранное", "submit:"+code)))
	}
	msg := tgbotapi.NewMessage(chatID, strings.Join(lines, "\n"))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, _ = b.api.Send(msg)
}

func (b *Bot) pickOption(ctx context.Context, chatID, telegramID int64, code string, optionID int64) {
	choice, err := b.svc.Choice(ctx, code)
	if err != nil {
		b.send(chatID, "Выбор не найден.")
		return
	}
	student, err := b.svc.CurrentStudent(ctx, telegramID)
	if err != nil {
		b.send(chatID, "Сначала зарегистрируйтесь через /register.")
		return
	}
	if choice.Type != domain.ChoiceTypeMobility && choice.MaxSelected == 1 {
		if _, err := b.svc.SubmitStudentChoice(ctx, student.ID, code, []int64{optionID}); err != nil {
			b.send(chatID, "Не удалось записаться: "+err.Error())
			return
		}
		b.send(chatID, "Запись сохранена. Посмотреть результат: /my.")
		return
	}
	b.mu.Lock()
	if b.pending[chatID] == nil {
		b.pending[chatID] = make(map[string]map[int64]struct{})
	}
	if b.pending[chatID][code] == nil {
		b.pending[chatID][code] = make(map[int64]struct{})
	}
	if _, ok := b.pending[chatID][code][optionID]; ok {
		delete(b.pending[chatID][code], optionID)
	} else {
		b.pending[chatID][code][optionID] = struct{}{}
	}
	selected := sortedIDs(b.pending[chatID][code])
	b.mu.Unlock()
	b.send(chatID, fmt.Sprintf("Черновик выбора: %v. Нажмите «Подтвердить выбранное».", selected))
}

func (b *Bot) submitPending(ctx context.Context, chatID, telegramID int64, code string) {
	student, err := b.svc.CurrentStudent(ctx, telegramID)
	if err != nil {
		b.send(chatID, "Сначала зарегистрируйтесь через /register.")
		return
	}
	b.mu.Lock()
	selected := sortedIDs(b.pending[chatID][code])
	b.mu.Unlock()
	if len(selected) == 0 {
		b.send(chatID, "Сначала выберите хотя бы одну дисциплину.")
		return
	}
	if _, err := b.svc.SubmitStudentChoice(ctx, student.ID, code, selected); err != nil {
		b.send(chatID, "Не удалось сохранить выбор: "+err.Error())
		return
	}
	b.mu.Lock()
	delete(b.pending[chatID], code)
	b.mu.Unlock()
	b.send(chatID, "Выбор сохранен. Посмотреть все записи: /my.")
}

func (b *Bot) sendEnrollments(ctx context.Context, chatID, telegramID int64) {
	student, err := b.svc.CurrentStudent(ctx, telegramID)
	if err != nil {
		b.send(chatID, "Сначала зарегистрируйтесь через /register.")
		return
	}
	enrollments, err := b.svc.EnrollmentsForStudent(ctx, student.ID)
	if err != nil {
		b.send(chatID, "Не удалось получить записи: "+err.Error())
		return
	}
	if len(enrollments) == 0 {
		b.send(chatID, "Пока нет выбранных дисциплин.")
		return
	}
	var lines []string
	for _, e := range enrollments {
		line := fmt.Sprintf("%s: %s", e.Choice.Title, e.Option.Title)
		if e.Option.Credits > 0 {
			line += fmt.Sprintf(" (%d з.е.)", e.Option.Credits)
		}
		lines = append(lines, line)
	}
	b.send(chatID, strings.Join(lines, "\n"))
}

func (b *Bot) sendStatement(ctx context.Context, chatID, telegramID int64) {
	student, err := b.svc.CurrentStudent(ctx, telegramID)
	if err != nil {
		b.send(chatID, "Сначала зарегистрируйтесь через /register.")
		return
	}
	data, err := b.svc.ApplicationDocx(ctx, student.ID)
	if err != nil {
		b.send(chatID, "Не удалось сформировать заявление: "+err.Error())
		return
	}
	msg := tgbotapi.NewDocument(chatID, tgbotapi.FileBytes{Name: "application.docx", Bytes: data})
	_, _ = b.api.Send(msg)
}

func (b *Bot) handleDocument(ctx context.Context, msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		b.send(msg.Chat.ID, "Загрузка файлов доступна только администратору.")
		return
	}
	caption := strings.TrimSpace(msg.Caption)
	if !strings.Contains(caption, "/import_students") && !strings.Contains(caption, "/import_choices") {
		b.send(msg.Chat.ID, "Для импорта отправьте файл с подписью /import_students или /import_choices.")
		return
	}
	url, err := b.api.GetFileDirectURL(msg.Document.FileID)
	if err != nil {
		b.send(msg.Chat.ID, "Не удалось получить файл: "+err.Error())
		return
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.send(msg.Chat.ID, "Не удалось скачать файл: "+err.Error())
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		b.send(msg.Chat.ID, "Не удалось прочитать файл: "+err.Error())
		return
	}
	if strings.Contains(caption, "/import_students") {
		count, err := b.svc.ImportStudentsFile(ctx, msg.Document.FileName, data)
		if err != nil {
			b.send(msg.Chat.ID, "Ошибка импорта студентов: "+err.Error())
			return
		}
		b.send(msg.Chat.ID, fmt.Sprintf("Импортировано студентов: %d", count))
		return
	}
	count, err := b.svc.ImportChoicesFile(ctx, msg.Document.FileName, data)
	if err != nil {
		b.send(msg.Chat.ID, "Ошибка импорта дисциплин: "+err.Error())
		return
	}
	b.send(msg.Chat.ID, fmt.Sprintf("Импортировано выборов: %d", count))
}

func (b *Bot) adminStudents(ctx context.Context, msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		b.send(msg.Chat.ID, "Команда доступна только администратору.")
		return
	}
	students, err := b.svc.ListStudents(ctx, 30)
	if err != nil {
		b.send(msg.Chat.ID, "Ошибка: "+err.Error())
		return
	}
	var lines []string
	for _, s := range students {
		lines = append(lines, fmt.Sprintf("%d | %s | %s", s.ID, s.GroupCode, s.FullName))
	}
	if len(lines) == 0 {
		lines = append(lines, "Студентов пока нет.")
	}
	b.send(msg.Chat.ID, strings.Join(lines, "\n"))
}

func (b *Bot) adminAuto(ctx context.Context, msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		b.send(msg.Chat.ID, "Команда доступна только администратору.")
		return
	}
	code := strings.TrimSpace(msg.CommandArguments())
	count, err := b.svc.AutoAssignRequired(ctx, code)
	if err != nil {
		b.send(msg.Chat.ID, "Ошибка автодораспределения: "+err.Error())
		return
	}
	b.send(msg.Chat.ID, fmt.Sprintf("Автоматически распределено студентов: %d", count))
}

func (b *Bot) adminExportCSV(ctx context.Context, msg *tgbotapi.Message) {
	if !b.isAdmin(msg.From.ID) {
		b.send(msg.Chat.ID, "Команда доступна только администратору.")
		return
	}
	data, err := b.svc.ExportResultsCSV(ctx)
	if err != nil {
		b.send(msg.Chat.ID, "Ошибка экспорта: "+err.Error())
		return
	}
	file := tgbotapi.FileBytes{Name: "results_" + time.Now().Format("20060102_150405") + ".csv", Bytes: data}
	_, _ = b.api.Send(tgbotapi.NewDocument(msg.Chat.ID, file))
}

func (b *Bot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = b.api.Send(msg)
}

func (b *Bot) isAdmin(id int64) bool {
	_, ok := b.admins[id]
	return ok
}

func sortedIDs(values map[int64]struct{}) []int64 {
	result := make([]int64, 0, len(values))
	for id := range values {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func choiceAllowed(choices []domain.Choice, code string) bool {
	for _, choice := range choices {
		if choice.Code == code {
			return true
		}
	}
	return false
}

func limitText(choice domain.Choice) string {
	if choice.Type == domain.ChoiceTypeMobility {
		return fmt.Sprintf("%d-%d з.е.", choice.MinSelected, choice.MaxSelected)
	}
	return fmt.Sprintf("%d-%d дисциплин", choice.MinSelected, choice.MaxSelected)
}

func helpText(admin bool) string {
	text := "Команды:\n/register - регистрация\n/choices - доступные выборы\n/my - мои записи\n/statement - заявление DOCX"
	if admin {
		text += "\n\nАдминистратор: /admin"
	}
	return text
}

func adminText() string {
	return "Админ-команды:\n/students - первые студенты\n/import_students - отправьте CSV/JSON файлом с этой подписью\n/import_choices - отправьте CSV/JSON файлом с этой подписью\n/auto CODE - автодораспределение обязательного выбора\n/export_csv - выгрузка результатов"
}
