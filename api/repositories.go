package handler

import (
	"github.com/canadian-ai/agent-clean/web"
	"net/http"
)

func Repositories(w http.ResponseWriter, r *http.Request) { web.Default.Repositories(w, r) }
