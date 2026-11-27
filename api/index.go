package handler

import (
	"github.com/canadian-ai/agent-clean/web"
	"net/http"
)

func Index(w http.ResponseWriter, r *http.Request) { web.Default.Home(w, r) }
