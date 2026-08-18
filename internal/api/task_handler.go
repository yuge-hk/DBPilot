package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/dbpilot/dbpilot/internal/task"
)

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	tasks, err := s.tasks.Store().List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if tasks == nil {
		tasks = []task.Task{}
	}
	writeOK(w, tasks)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, ErrBadRequest("invalid_id", "invalid id"))
		return
	}
	t, err := s.tasks.Store().Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, task.ErrNotFound) {
			writeError(w, ErrNotFound("task_not_found", "task not found"))
			return
		}
		writeError(w, ErrInternal(err))
		return
	}
	writeOK(w, t)
}
