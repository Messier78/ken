package http

import "net/http"

type stream struct {
}

func (s *stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
}
