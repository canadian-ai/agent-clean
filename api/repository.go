package handler

import (
	"github.com/canadian-ai/agent-clean/web"
	"net/http"
)

func Repository(w http.ResponseWriter, r *http.Request) { web.Default.Repository(w, r) }
