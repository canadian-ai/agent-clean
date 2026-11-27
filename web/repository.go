package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const rewriteConfirmation = "REWRITE HISTORY"
const historyLimit = 100

var (
	repoPart   = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,199}$`)
	branchName = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,199}$`)
)

type repositoryRequest struct {
	Action           string   `json:"action"`
	Owner            string   `json:"owner"`
	Repo             string   `json:"repo"`
	Branch           string   `json:"branch"`
	Agents           []string `json:"agents"`
	UnlinkAutomation bool     `json:"unlinkAutomation"`
	Head             string   `json:"head"`
	Confirm          string   `json:"confirm"`
}

func (r repositoryRequest) valid() bool {
	return repoPart.MatchString(r.Owner) && repoPart.MatchString(r.Repo) &&
		r.Owner != "." && r.Owner != ".." && r.Repo != "." && r.Repo != ".." &&
		branchName.MatchString(r.Branch) && !strings.Contains(r.Branch, "..") &&
		!strings.Contains(r.Branch, "//") && !strings.HasPrefix(r.Branch, "/") &&
		!strings.HasSuffix(r.Branch, "/") && !strings.Contains("/"+r.Branch+"/", "/./")
}

func (r repositoryRequest) path() string { return "/repos/" + r.Owner + "/" + r.Repo }

type commitReference struct {
	SHA string `json:"sha"`
}

type githubCommit struct {
	Message      string            `json:"message"`
	Tree         commitReference   `json:"tree"`
	Author       *identity         `json:"author"`
	Committer    *identity         `json:"committer"`
	Parents      []commitReference `json:"parents"`
	Verification struct {
		Verified  bool   `json:"verified"`
		Signature string `json:"signature"`
	} `json:"verification"`
}

type listedCommit struct {
	SHA     string            `json:"sha"`
	Commit  githubCommit      `json:"commit"`
	Parents []commitReference `json:"parents"`
}

type scanRow struct {
	SHA         string            `json:"sha"`
	Message     string            `json:"message"`
	NextMessage string            `json:"nextMessage"`
	Removed     map[string]int    `json:"removed"`
	Automation  map[string]string `json:"automation"`
	LineMatches []lineMatch       `json:"lineMatches"`
	Parents     int               `json:"parents"`
	Author      string            `json:"author"`
	Date        string            `json:"date"`
	Signed      bool              `json:"signed"`
}

type scanResult struct {
	Repo             string            `json:"repo"`
	Branch           string            `json:"branch"`
	Head             string            `json:"head"`
	Protected        bool              `json:"protected"`
	TooLarge         bool              `json:"tooLarge"`
	Commits          []scanRow         `json:"commits"`
	Available        int               `json:"available"`
	Scanned          int               `json:"scanned"`
	ContributorCheck contributorReport `json:"contributorCheck"`
	UnlinkedIdentity identity          `json:"unlinkedIdentity"`
}

func (s *Server) Repository(w http.ResponseWriter, r *http.Request) {
	if !acceptAPI(w, r) {
		return
	}
	var input repositoryRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	if err := decoder.Decode(&input); err != nil {
		writeError(w, &apiError{http.StatusBadRequest, "Invalid JSON request"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, &apiError{http.StatusBadRequest, "Invalid JSON request"})
		return
	}
	if input.Branch == "" {
		input.Branch = "main"
	}
	if !input.valid() {
		writeError(w, &apiError{http.StatusBadRequest, "Invalid GitHub repository reference"})
		return
	}
	token := strings.TrimSpace(r.Header.Get("X-GitHub-Token"))
	if len(token) > 500 {
		writeError(w, &apiError{http.StatusBadRequest, "Invalid token"})
		return
	}
	if input.Action == "apply" {
		result, err := s.apply(r.Context(), input, token)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	result, err := s.scan(r.Context(), input, token)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) scan(ctx context.Context, request repositoryRequest, token string) (scanResult, error) {
	var branch struct {
		Commit    commitReference
		Protected bool
	}
	prefix := request.path()
	if err := s.github(ctx, http.MethodGet, prefix+"/branches/"+url.PathEscape(request.Branch), token, nil, &branch); err != nil {
		return scanResult{}, err
	}
	if branch.Commit.SHA == "" {
		return scanResult{}, &apiError{http.StatusBadGateway, "GitHub did not return a branch head."}
	}
	commitQuery := prefix + "/commits?sha=" + url.QueryEscape(branch.Commit.SHA)
	var commits []listedCommit
	if err := s.github(ctx, http.MethodGet, commitQuery+"&per_page=100", token, nil, &commits); err != nil {
		return scanResult{}, err
	}
	tooLarge := false
	if len(commits) == historyLimit {
		var older []commitReference
		if err := s.github(ctx, http.MethodGet, commitQuery+"&per_page=1&page=101", token, nil, &older); err != nil {
			return scanResult{}, err
		}
		tooLarge = len(older) > 0
	}
	rows := make([]scanRow, 0, len(commits))
	available := 0
	for _, item := range commits {
		commit := item.Commit
		lines := matchLines(commit.Message)
		next, removed := cleanMessage(commit.Message, lines, agentOrder, true)
		row := scanRow{
			SHA: item.SHA, Message: commit.Message, NextMessage: next, Removed: removed,
			Automation: make(map[string]string), LineMatches: lines, Parents: len(item.Parents),
			Author: "Unknown", Signed: commit.Verification.Verified || commit.Verification.Signature != "",
		}
		if commit.Author != nil {
			if commit.Author.Name != "" {
				row.Author = commit.Author.Name
			}
			row.Date = commit.Author.Date
			if isLinkedAutomation(commit.Author.Email) {
				row.Automation["author"] = commit.Author.Name
			}
		}
		if commit.Committer != nil && isLinkedAutomation(commit.Committer.Email) {
			row.Automation["committer"] = commit.Committer.Name
		}
		if len(removed) > 0 {
			available++
		}
		rows = append(rows, row)
	}
	return scanResult{
		Repo: request.Owner + "/" + request.Repo, Branch: request.Branch, Head: branch.Commit.SHA,
		Protected: branch.Protected, TooLarge: tooLarge, Commits: rows, Available: available,
		Scanned: len(rows), ContributorCheck: contributorCheck(commits, tooLarge),
		UnlinkedIdentity: unlinkedIdentity,
	}, nil
}

type applyResult struct {
	Changed int     `json:"changed"`
	Rebuilt int     `json:"rebuilt,omitempty"`
	Head    string  `json:"head"`
	Backup  *string `json:"backup"`
	OldHead string  `json:"oldHead,omitempty"`
}

type newCommit struct {
	Message   string    `json:"message"`
	Tree      string    `json:"tree"`
	Parents   []string  `json:"parents"`
	Author    *identity `json:"author,omitempty"`
	Committer *identity `json:"committer,omitempty"`
}

func (s *Server) apply(ctx context.Context, request repositoryRequest, token string) (applyResult, error) {
	if token == "" {
		return applyResult{}, &apiError{http.StatusUnauthorized, "A GitHub fine-grained token is required for cleanup"}
	}
	agents := make([]string, 0, len(request.Agents))
	removeSessionMarkers := false
	for _, agent := range request.Agents {
		if _, ok := agentPatterns[agent]; ok {
			agents = append(agents, agent)
			removeSessionMarkers = removeSessionMarkers || agent == "claude-session"
		}
	}
	if len(agents) == 0 && !request.UnlinkAutomation {
		return applyResult{}, &apiError{http.StatusBadRequest, "Select at least one type of attribution"}
	}
	if request.Confirm != rewriteConfirmation {
		return applyResult{}, &apiError{http.StatusBadRequest, "Explicit confirmation required"}
	}
	result, err := s.scan(ctx, request, token)
	if err != nil {
		return applyResult{}, err
	}
	if result.Head != request.Head {
		return applyResult{}, &apiError{http.StatusConflict, "Branch changed since preview. Scan again."}
	}
	if result.TooLarge || result.Scanned >= historyLimit {
		return applyResult{}, &apiError{http.StatusUnprocessableEntity, "This branch exceeds the 99-commit safety limit"}
	}
	if result.Protected {
		return applyResult{}, &apiError{http.StatusForbidden, "Protected branches cannot be rewritten by AgentClean"}
	}

	// Only rebuild from the earliest selected commit forward. Earlier objects stay intact.
	earliest := -1
	changed := 0
	for i, row := range result.Commits {
		if removeSessionMarkers && row.Removed["claude-session"] > 0 {
			return applyResult{}, &apiError{http.StatusUnprocessableEntity, "Use the CLI with a local backup to remove Claude session markers."}
		}
		if row.Parents > 1 || row.Signed {
			return applyResult{}, &apiError{http.StatusUnprocessableEntity, "Merge commits or signed commits are not supported. No changes made."}
		}
		updated, _ := cleanMessage(row.Message, row.LineMatches, agents, request.UnlinkAutomation)
		if updated != row.Message || (request.UnlinkAutomation && len(row.Automation) > 0) {
			earliest = i
			changed++
		}
	}
	if changed == 0 {
		return applyResult{Head: result.Head}, nil
	}

	prefix := request.path()
	backup := "agentclean-backup-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	ref := struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}{"refs/heads/" + backup, result.Head}
	if err := s.github(ctx, http.MethodPost, prefix+"/git/refs", token, ref, nil); err != nil {
		return applyResult{}, err
	}

	parent := ""
	for i := earliest; i >= 0; i-- {
		row := result.Commits[i]
		var original githubCommit
		if err := s.github(ctx, http.MethodGet, prefix+"/git/commits/"+url.PathEscape(row.SHA), token, nil, &original); err != nil {
			return applyResult{}, err
		}
		if original.Tree.SHA == "" || original.Author == nil || original.Committer == nil ||
			len(original.Parents) > 1 || original.Verification.Verified || original.Verification.Signature != "" {
			return applyResult{}, &apiError{http.StatusUnprocessableEntity, "GitHub returned an unsupported commit. Backup exists at " + backup + "."}
		}
		if i == earliest && len(original.Parents) == 1 {
			parent = original.Parents[0].SHA
		}
		message, _ := cleanMessage(original.Message, matchLines(original.Message), agents, request.UnlinkAutomation)
		rebuilt := newCommit{
			Message: message, Tree: original.Tree.SHA, Parents: []string{},
			Author: original.Author, Committer: original.Committer,
		}
		if parent != "" {
			rebuilt.Parents = []string{parent}
		}
		if request.UnlinkAutomation {
			rebuilt.Author = unlinkIdentity(rebuilt.Author)
			rebuilt.Committer = unlinkIdentity(rebuilt.Committer)
		}
		var next commitReference
		if err := s.github(ctx, http.MethodPost, prefix+"/git/commits", token, rebuilt, &next); err != nil {
			return applyResult{}, err
		}
		if next.SHA == "" {
			return applyResult{}, &apiError{http.StatusBadGateway, "GitHub did not return the rewritten commit."}
		}
		parent = next.SHA
	}

	// The REST ref update has no expected-head parameter. Recheck immediately before updating.
	var current struct{ Object commitReference }
	branch := strings.Join(escapePathSegments(request.Branch), "/")
	if err := s.github(ctx, http.MethodGet, prefix+"/git/ref/heads/"+branch, token, nil, &current); err != nil {
		return applyResult{}, err
	}
	if current.Object.SHA != result.Head {
		return applyResult{}, &apiError{http.StatusConflict, "Branch moved. Backup exists at " + backup + "; no branch update performed."}
	}
	update := struct {
		SHA   string `json:"sha"`
		Force bool   `json:"force"`
	}{parent, true}
	if err := s.github(ctx, http.MethodPatch, prefix+"/git/refs/heads/"+branch, token, update, nil); err != nil {
		return applyResult{}, err
	}
	return applyResult{Changed: changed, Rebuilt: earliest + 1, Head: parent, Backup: &backup, OldHead: result.Head}, nil
}

func escapePathSegments(path string) []string {
	segments := strings.Split(path, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	return segments
}
