package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	githubURL string
	client    *http.Client
}

var Default = &Server{
	githubURL: "https://api.github.com",
	client: &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, previous []*http.Request) error {
			if len(previous) >= 5 || req.URL.Scheme != "https" || req.URL.Host != "api.github.com" {
				return http.ErrUseLastResponse
			}
			return nil
		},
	},
}

type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string { return e.message }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, err error) {
	var problem *apiError
	if errors.As(err, &problem) {
		writeJSON(w, problem.status, map[string]string{"error": problem.message})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Unexpected error"})
}

func (s *Server) github(ctx context.Context, method, path, token string, body, result any) error {
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.githubURL+path, input)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Cache-Control", "no-store")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return &apiError{http.StatusBadGateway, "Unable to reach GitHub."}
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		problem := struct{ Message string }{}
		_ = decoder.Decode(&problem)
		status := response.StatusCode
		if status < 400 || status >= 600 {
			status = http.StatusBadGateway
		}
		if status == http.StatusUnauthorized {
			problem.Message = "Invalid or expired GitHub token."
		}
		if problem.Message == "" {
			problem.Message = "GitHub request failed."
		}
		return &apiError{status, problem.Message}
	}
	if result == nil {
		_, err := io.Copy(io.Discard, response.Body)
		return err
	}
	if err := decoder.Decode(result); err != nil {
		return &apiError{http.StatusBadGateway, "GitHub returned an invalid response."}
	}
	return nil
}

func acceptAPI(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, &apiError{http.StatusMethodNotAllowed, "Method not allowed"})
		return false
	}
	return true
}

type githubUser struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type repositorySummary struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
}

type repositoryChoice struct {
	repositorySummary
	Owner   string `json:"owner"`
	CanPush bool   `json:"can_push"`
}

func (s *Server) Repositories(w http.ResponseWriter, r *http.Request) {
	if !acceptAPI(w, r) {
		return
	}
	token := strings.TrimSpace(r.Header.Get("X-GitHub-Token"))
	if token == "" || len(token) > 500 {
		writeError(w, &apiError{http.StatusUnauthorized, "Provide your GitHub personal access token."})
		return
	}
	ctx := r.Context()
	var user githubUser
	if err := s.github(ctx, http.MethodGet, "/user", token, nil, &user); err != nil {
		writeError(w, err)
		return
	}
	choices := make([]repositoryChoice, 0)
	for page := 1; page <= 3; page++ {
		var batch []struct {
			repositorySummary
			Owner       githubUser
			Permissions struct{ Push, Admin, Maintain bool }
		}
		path := fmt.Sprintf("/user/repos?affiliation=owner,collaborator,organization_member&sort=updated&per_page=100&page=%d", page)
		if err := s.github(ctx, http.MethodGet, path, token, nil, &batch); err != nil {
			writeError(w, err)
			return
		}
		for _, item := range batch {
			choices = append(choices, repositoryChoice{
				repositorySummary: item.repositorySummary,
				Owner:             item.Owner.Login,
				CanPush:           item.Permissions.Push || item.Permissions.Admin || item.Permissions.Maintain,
			})
		}
		if len(batch) < 100 {
			break
		}
	}
	writeJSON(w, http.StatusOK, struct {
		User         githubUser         `json:"user"`
		Repositories []repositoryChoice `json:"repositories"`
	}{user, choices})
}
