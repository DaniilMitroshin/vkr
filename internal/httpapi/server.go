package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"vkr/internal/repository"
	"vkr/internal/service"
)

type Server struct {
	svc *service.Service
}

func New(svc *service.Service) http.Handler {
	s := &Server{svc: svc}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Post("/import/students", s.importStudents)
		r.Post("/import/choices", s.importChoices)
		r.Get("/students", s.listStudents)
		r.Post("/students/register", s.registerStudent)
		r.Get("/students/{id}/choices", s.studentChoices)
		r.Get("/students/{id}/enrollments", s.studentEnrollments)
		r.Get("/students/{id}/application.docx", s.applicationDocx)
		r.Post("/students/{id}/choices/{code}/submit", s.submitChoice)
		r.Get("/choices", s.listChoices)
		r.Get("/choices/{code}", s.choice)
		r.Get("/choices/{code}/options", s.choiceOptions)
		r.Post("/choices/{code}/auto-assign", s.autoAssign)
		r.Get("/export/results", s.exportResults)
	})
	return r
}

func (s *Server) importStudents(w http.ResponseWriter, r *http.Request) {
	filename, data, err := readUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	count, err := s.svc.ImportStudentsFile(r.Context(), filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": count})
}

func (s *Server) importChoices(w http.ResponseWriter, r *http.Request) {
	filename, data, err := readUpload(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	count, err := s.svc.ImportChoicesFile(r.Context(), filename, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": count})
}

func (s *Server) listStudents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	students, err := s.svc.ListStudents(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, students)
}

func (s *Server) registerStudent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TelegramID int64  `json:"telegram_id"`
		FullName   string `json:"full_name"`
		GroupCode  string `json:"group_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	student, err := s.svc.RegisterStudent(r.Context(), req.TelegramID, req.FullName, req.GroupCode)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, student)
}

func (s *Server) studentChoices(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	choices, err := s.svc.StudentChoices(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, choices)
}

func (s *Server) studentEnrollments(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	enrollments, err := s.svc.EnrollmentsForStudent(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, enrollments)
}

func (s *Server) applicationDocx(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	data, err := s.svc.ApplicationDocx(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", `attachment; filename="application.docx"`)
	_, _ = w.Write(data)
}

func (s *Server) submitChoice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		OptionIDs []int64 `json:"option_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	enrollments, err := s.svc.SubmitStudentChoice(r.Context(), id, chi.URLParam(r, "code"), req.OptionIDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, enrollments)
}

func (s *Server) listChoices(w http.ResponseWriter, r *http.Request) {
	choices, err := s.svc.ListChoices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, choices)
}

func (s *Server) choice(w http.ResponseWriter, r *http.Request) {
	choice, err := s.svc.Choice(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, choice)
}

func (s *Server) choiceOptions(w http.ResponseWriter, r *http.Request) {
	options, err := s.svc.ChoiceOptions(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, options)
}

func (s *Server) autoAssign(w http.ResponseWriter, r *http.Request) {
	count, err := s.svc.AutoAssignRequired(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assigned": count})
}

func (s *Server) exportResults(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		data, err := s.svc.ExportResultsJSON(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
		return
	}
	data, err := s.svc.ExportResultsCSV(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="results.csv"`)
	_, _ = w.Write(data)
}

func readUpload(r *http.Request) (string, []byte, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return "", nil, err
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return "", nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		return header.Filename, data, err
	}
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		filename = r.Header.Get("X-Filename")
	}
	if filename == "" {
		return "", nil, errors.New("filename is required for raw upload")
	}
	data, err := io.ReadAll(r.Body)
	return filename, data, err
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
