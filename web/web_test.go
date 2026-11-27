package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHome(t *testing.T) {
	response := httptest.NewRecorder()
	Default.Home(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("home returned %d", response.Code)
	}
	for _, fragment := range []string{
		`<title>AgentClean`, `src="/app.js"`, `id="scan-panel"`, `data-agent="claude"`, `id="rewrite-form"`,
	} {
		if !strings.Contains(response.Body.String(), fragment) {
			t.Errorf("home is missing %s", fragment)
		}
	}
	if strings.Contains(response.Body.String(), "ZgotmplZ") {
		t.Error("template contains a rejected value")
	}
}

func TestClassificationAndCleanup(t *testing.T) {
	for email, want := range map[string]string{
		"noreply@anthropic.com":                        "ai-agent",
		"github-actions[bot]@users.noreply.github.com": "automation",
		"12345+vercel[bot]@users.noreply.github.com":   "automation",
		"claude@example.test":                          "human",
		"automation@girl.invalid":                      "unlinked-automation",
	} {
		if found := contributorCategory(email); found != want {
			t.Errorf("%s: %s, want %s", email, found, want)
		}
	}
	const human = "Co-authored-by: Claude Vercel <claude@example.test>"
	const bot = "Co-authored-by: Vercel <123+vercel[bot]@users.noreply.github.com>"
	message := "Keep human coauthors\n\n" + human + "\n" + bot + "\n"
	matches := matchLines(message)
	if got, _ := cleanMessage(message, matches, agentOrder, false); got != message {
		t.Errorf("unexpected removal with unlinking off: %q", got)
	}
	if got, _ := cleanMessage(message, matches, agentOrder, true); got != "Keep human coauthors\n\n"+human {
		t.Errorf("unexpected removal with unlinking on: %q", got)
	}
	const session = "Claude-Session: example"
	const claude = "Co-authored-by: Claude Code <bot@example.test>"
	message = "Subject\n\n" + session + "\n" + claude
	matches = matchLines(message)
	if got, _ := cleanMessage(message, matches, []string{"claude-session"}, false); got != "Subject\n\n"+claude {
		t.Errorf("session selection did not preserve other attribution: %q", got)
	}
}

func TestRepositoryPicker(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("GitHub token was not forwarded")
		}
		switch r.URL.Path {
		case "/user":
			fmt.Fprint(w, `{"login":"owner","avatar_url":"https://example.test/avatar"}`)
		case "/user/repos":
			if r.URL.Query().Get("page") != "1" {
				t.Errorf("unexpected page %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `[{"id":4,"full_name":"owner/project","name":"project","owner":{"login":"owner"},"private":true,"default_branch":"master","permissions":{"maintain":true}}]`)
		default:
			t.Errorf("unexpected GitHub route %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	server := &Server{githubURL: upstream.URL, client: upstream.Client()}
	request := httptest.NewRequest(http.MethodPost, "/api/repositories", strings.NewReader("{}"))
	request.Header.Set("X-GitHub-Token", "test-token")
	response := httptest.NewRecorder()
	server.Repositories(response, request)
	var result struct {
		User         githubUser
		Repositories []repositoryChoice
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.User.Login != "owner" || len(result.Repositories) != 1 {
		t.Fatalf("unexpected response %d %s", response.Code, response.Body)
	}
	choice := result.Repositories[0]
	if choice.Name != "project" || choice.FullName != "owner/project" || choice.Owner != "owner" ||
		choice.ID != 4 || !choice.CanPush || !choice.Private || choice.DefaultBranch != "master" {
		t.Errorf("unexpected GitHub repository mapping: %+v", choice)
	}
}

func TestRepositoryRewrite(t *testing.T) {
	for _, tc := range []struct {
		name, liveHead, signature string
		wantStatus                int
	}{
		{"human author with bot committer", "old", "", http.StatusOK},
		{"branch moved after preview", "moved", "", http.StatusConflict},
		{"unverified signed commit", "old", "signature", http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const originalMessage = "Keep original message\n"
			const date = "2026-01-01T00:00:00Z"
			const path = "/repos/owner/project"
			author := &identity{Name: "Kampouse", Email: "kampouse@example.test", Date: date}
			bot := &identity{Name: "Vercel", Email: "123+vercel[bot]@users.noreply.github.com", Date: date}
			original := githubCommit{Message: originalMessage, Tree: commitReference{"tree"}, Author: author, Committer: bot}
			original.Verification.Signature = tc.signature
			var written newCommit
			var backup, updated bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET " + path + "/branches/main":
					fmt.Fprint(w, `{"commit":{"sha":"old"},"protected":false}`)
				case "GET " + path + "/commits":
					if r.URL.Query().Get("sha") != "old" {
						t.Error("scan did not pin commit lookup to the preview head")
					}
					writeJSON(w, http.StatusOK, []listedCommit{{SHA: "old", Commit: original, Parents: []commitReference{{"human"}}}})
				case "POST " + path + "/git/refs":
					var ref struct{ Ref, SHA string }
					if err := json.NewDecoder(r.Body).Decode(&ref); err != nil {
						t.Error(err)
					}
					backup = strings.HasPrefix(ref.Ref, "refs/heads/agentclean-backup-") && ref.SHA == "old"
					fmt.Fprint(w, "{}")
				case "GET " + path + "/git/commits/old":
					if !backup {
						t.Error("began writing without backup")
					}
					raw := original
					raw.Parents = []commitReference{{"human"}}
					writeJSON(w, http.StatusOK, raw)
				case "POST " + path + "/git/commits":
					if err := json.NewDecoder(r.Body).Decode(&written); err != nil {
						t.Error(err)
					}
					fmt.Fprint(w, `{"sha":"new"}`)
				case "GET " + path + "/git/ref/heads/main":
					fmt.Fprintf(w, `{"object":{"sha":%q}}`, tc.liveHead)
				case "PATCH " + path + "/git/refs/heads/main":
					var ref struct {
						SHA   string
						Force bool
					}
					if err := json.NewDecoder(r.Body).Decode(&ref); err != nil {
						t.Error(err)
					}
					updated = ref.SHA == "new" && ref.Force
					fmt.Fprint(w, "{}")
				default:
					t.Errorf("unexpected GitHub route %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			server := &Server{githubURL: upstream.URL, client: upstream.Client()}

			request := httptest.NewRequest(http.MethodPost, "/api/repository", strings.NewReader(
				`{"owner":"owner","repo":"project","branch":"main"}`))
			response := httptest.NewRecorder()
			server.Repository(response, request)
			var scan scanResult
			if err := json.Unmarshal(response.Body.Bytes(), &scan); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || scan.Head != "old" || len(scan.Commits) != 1 ||
				scan.Commits[0].Automation["committer"] != "Vercel" || scan.ContributorCheck.HumanCount != 1 ||
				scan.ContributorCheck.Committers[0].Category != "automation" {
				t.Fatalf("unexpected preview %d %s", response.Code, response.Body)
			}
			request = httptest.NewRequest(http.MethodPost, "/api/repository", strings.NewReader(
				`{"action":"apply","owner":"owner","repo":"project","branch":"main","head":"old","unlinkAutomation":true,"confirm":"REWRITE HISTORY"}`))
			request.Header.Set("X-GitHub-Token", "test-token")
			response = httptest.NewRecorder()
			server.Repository(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("apply returned %d, want %d: %s", response.Code, tc.wantStatus, response.Body)
			}
			if response.Code != http.StatusOK {
				if updated || (tc.signature != "" && backup) {
					t.Error("updated refs despite blocked cleanup")
				}
				return
			}
			if !backup || !updated || written.Message != originalMessage || written.Tree != "tree" ||
				written.Author == nil || *written.Author != *author || written.Committer == nil ||
				written.Committer.Email != unlinkedIdentity.Email || written.Committer.Date != date ||
				len(written.Parents) != 1 || written.Parents[0] != "human" {
				t.Errorf("unexpected commit or ref update: %+v", written)
			}
		})
	}
}
