package web

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var agentOrder = []string{"claude", "claude-session", "codex", "cursor", "codesmith", "opencode", "gemini"}

var agentPatterns = map[string]*regexp.Regexp{
	"claude":         regexp.MustCompile(`(?i)^\s*(co-authored-by:\s*claude(?:\s+(?:code|(?:opus|sonnet|haiku)(?:\s+[0-9.]+)?))?\s*<[^<>]+>\s*|🤖\s*generated with \[?claude code\]?.*)$`),
	"claude-session": regexp.MustCompile(`(?i)^\s*claude-session:\s*.*$`),
	"codex":          regexp.MustCompile(`(?i)^\s*(co-authored-by:\s*(?:openai\s+)?codex\s*<[^<>]+>\s*|(?:generated|assisted) (?:with|by) (?:openai\s+)?codex\b.*)$`),
	"cursor":         regexp.MustCompile(`(?i)^\s*(co-authored-by:\s*cursor\s*<[^<>]+>\s*|(?:generated|assisted) (?:with|by) cursor\b.*)$`),
	"codesmith":      regexp.MustCompile(`(?i)^\s*co-authored-by:\s*codesmith\s*<[^<>]+>\s*$`),
	"opencode":       regexp.MustCompile(`(?i)^\s*co-authored-by:\s*opencode(?:-agent)?(?:\[bot\])?\s*<[^<>]+>\s*$`),
	"gemini":         regexp.MustCompile(`(?i)^\s*(co-authored-by:\s*(?:google\s+)?gemini(?:\s+cli)?\s*<[^<>]+>\s*|(?:generated|assisted) (?:with|by) gemini(?: cli)?\b.*)$`),
}

var (
	githubBotEmail = regexp.MustCompile(`^(?:\d+\+)?(?:[\w-]+\[bot\]|github-actions|dependabot|renovate|vercel)@users\.noreply\.github\.com$`)
	unlinkedEmail  = regexp.MustCompile(`^automation@[a-z0-9-]+\.invalid$`)
	coauthorLine   = regexp.MustCompile(`(?i)^\s*co-authored-by:\s*[^<>]*<([^<>]+)>\s*$`)
	blankLines     = regexp.MustCompile(`\n{3,}`)
)

var unlinkedIdentity = identity{Name: "Repository Automation", Email: "automation@repository.invalid"}

type identity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date,omitempty"`
}

func contributorCategory(email string) string {
	email = strings.ToLower(email)
	switch {
	case unlinkedEmail.MatchString(email):
		return "unlinked-automation"
	case email == "noreply@anthropic.com" || email == "codesmith-bot@users.noreply.github.com":
		return "ai-agent"
	case githubBotEmail.MatchString(email):
		return "automation"
	default:
		return "human"
	}
}

func isLinkedAutomation(email string) bool {
	category := contributorCategory(email)
	return category == "ai-agent" || category == "automation"
}

func unlinkIdentity(person *identity) *identity {
	if person == nil || !isLinkedAutomation(person.Email) {
		return person
	}
	result := unlinkedIdentity
	result.Date = person.Date
	return &result
}

type lineMatch struct {
	Index  int      `json:"index"`
	Agents []string `json:"agents"`
}

func matchLines(message string) []lineMatch {
	matches := make([]lineMatch, 0)
	for i, original := range strings.Split(message, "\n") {
		line := strings.TrimSuffix(original, "\r")
		var agents []string
		for _, agent := range agentOrder {
			if agentPatterns[agent].MatchString(line) {
				agents = append(agents, agent)
			}
		}
		if parts := coauthorLine.FindStringSubmatch(line); parts != nil && isLinkedAutomation(parts[1]) {
			agents = append(agents, "automation")
		}
		if len(agents) > 0 {
			matches = append(matches, lineMatch{i, agents})
		}
	}
	return matches
}

func cleanMessage(message string, matches []lineMatch, agents []string, unlinkAutomation bool) (string, map[string]int) {
	removed := make(map[string]int)
	selected := make(map[string]bool)
	for _, agent := range agents {
		selected[agent] = true
	}
	selected["automation"] = unlinkAutomation
	drop := make(map[int]bool)
	for _, match := range matches {
		// The browser can select multiple overlapping rules. Drop a line only once.
		for _, agent := range match.Agents {
			if selected[agent] {
				drop[match.Index] = true
				removed[agent]++
				break
			}
		}
	}
	if len(drop) == 0 {
		return message, removed
	}
	var kept []string
	for i, line := range strings.Split(message, "\n") {
		if !drop[i] {
			kept = append(kept, line)
		}
	}
	cleaned := blankLines.ReplaceAllString(strings.Join(kept, "\n"), "\n\n")
	return strings.TrimRightFunc(cleaned, func(r rune) bool { return unicode.IsSpace(r) || r == '\uFEFF' }), removed
}

type contributor struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Category string `json:"category"`
	Commits  int    `json:"commits"`
}

type contributorReport struct {
	Authors             []contributor `json:"authors"`
	Committers          []contributor `json:"committers"`
	Partial             bool          `json:"partial"`
	AutoApproved        []contributor `json:"autoApproved"`
	HumanCount          int           `json:"humanCount"`
	HasAutomatedAuthors bool          `json:"hasAutomatedAuthors"`
}

func contributorCheck(commits []listedCommit, partial bool) contributorReport {
	report := contributorReport{
		Authors:      contributorRows(commits, func(c githubCommit) *identity { return c.Author }),
		Committers:   contributorRows(commits, func(c githubCommit) *identity { return c.Committer }),
		Partial:      partial,
		AutoApproved: make([]contributor, 0),
	}
	for _, row := range report.Authors {
		switch row.Category {
		case "human":
			report.HumanCount++
		case "automation", "ai-agent":
			report.AutoApproved = append(report.AutoApproved, row)
		}
	}
	report.HasAutomatedAuthors = len(report.AutoApproved) > 0
	return report
}

func contributorRows(commits []listedCommit, who func(githubCommit) *identity) []contributor {
	type key struct{ name, email string }
	indices := make(map[key]int)
	rows := make([]contributor, 0)
	for _, entry := range commits {
		person := who(entry.Commit)
		id := key{name: "Unknown"}
		if person != nil {
			id.email = person.Email
			if person.Name != "" {
				id.name = person.Name
			}
		}
		index, exists := indices[id]
		if !exists {
			index = len(rows)
			indices[id] = index
			rows = append(rows, contributor{Name: id.name, Email: id.email, Category: contributorCategory(id.email)})
		}
		rows[index].Commits++
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Commits > rows[j].Commits })
	return rows
}
