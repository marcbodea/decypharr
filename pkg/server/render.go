package server

import (
	"net/http"

	"github.com/sirrobot01/decypharr/internal/customerror"
)

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	err := s.templates.ExecuteTemplate(w, name, data)
	if err == nil {
		return
	}

	if customerror.IsSilentError(err) {
		return
	}

	s.logger.Error().Err(err).Msg("template error")
}
