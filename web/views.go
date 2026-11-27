package web

const wizardMain = `
<main class="wizard">
  <nav class="steps" id="steps" aria-label="Cleanup steps">
    {{range .Steps}}<div class="step"><span class="num">{{.Number}}</span>{{.Label}}</div>{{end}}
  </nav>
  <div class="body" id="panel">
    <noscript>Enable JavaScript to connect to GitHub and preview cleanup changes.</noscript>
  </div>
</main>
`

const wizardTemplates = `
<template id="connect-panel">
  <h2>Connect to GitHub</h2>
  <p class="sub">Enter a personal access token to find the repositories you can access.</p>
  <form id="connect-form">
    <div class="field">
      <label for="token">GitHub personal access token</label>
      <input id="token" type="password" autocomplete="off" spellcheck="false" placeholder="github_pat_…" required>
      <div class="help">Use a <a href="https://github.com/settings/personal-access-tokens/new" target="_blank" rel="noopener noreferrer">fine-grained GitHub token ↗</a> scoped to your repositories. For cleanup, grant Contents: Read and write.</div>
    </div>
    <div class="notice"><strong>Private by design.</strong> Your token stays in this tab, is sent over HTTPS to GitHub through AgentClean, and is never stored in a database.</div>
    <div class="foot"><span class="muted">Step 1 of 5</span><button id="connect" class="btn primary" type="submit">Connect to GitHub →</button></div>
  </form>
</template>

<template id="repository-panel">
  <h2>Select a repository</h2>
  <p class="sub">Connected as <b id="login"></b>. Choose a repository to scan.</p>
  <div class="field">
    <label for="repo-search">Search repositories</label>
    <input id="repo-search" type="text" placeholder="Search by name…">
    <div class="help"><span id="repository-count"></span> repositories available. Showing up to 75 matches.</div>
  </div>
  <div class="repo-results" id="repo-results"></div>
  <div class="foot"><button class="btn" id="change-token" type="button">← Change token</button><span class="muted">Step 2 of 5</span></div>
</template>

<template id="repository-row">
  <button type="button" class="repo-item">
    <span><strong data-repo-name></strong><small data-repo-info></small></span>
    <span class="repo-arrow" aria-hidden="true">→</span>
  </button>
</template>

<template id="scan-panel">
  <h2>Scan results</h2>
  <p class="sub">Found attribution patterns in <span id="scanned-repo"></span> on <b id="scanned-branch"></b>. Choose which attribution to clean.</p>
  <div class="statgrid">
    <div class="stat"><b id="scan-count"></b><span>Commits scanned</span></div>
    <div class="stat"><b id="attribution-count"></b><span>Commits with attribution</span></div>
    <div class="stat"><b id="selected-count"></b><span>Selected for cleanup</span></div>
  </div>
  <div class="notice">
    <strong>Contributor check</strong>
    <p>Human Git author identities <span id="human-count"></span> · Recognized automated author identities <span id="automation-count"></span></p>
    <ul id="automation-list" hidden></ul>
    <p id="partial-audit" hidden>Only the most recent 100 commits were checked.</p>
    <label class="inlinecheck"><input id="unlink-automation" type="checkbox">Unlink automated authors and committers and remove their co-author trailers</label>
    <p>Automated commits will use the unlinked Repository Automation identity. Human authors retain their credits.</p>
  </div>
  <div class="options">
    {{range .Agents}}
      <label class="option"><input type="checkbox" data-agent="{{.ID}}" checked><span><strong>{{.Name}}</strong><small data-count>0 matching commits</small></span></label>
    {{end}}
  </div>
  <div class="warning" id="large-history" hidden>This history exceeds the browser-cleanup limit. You can scan recent commits and use the CLI for the full history.</div>
  <div class="foot"><button class="btn" id="back-to-repos" type="button">← Repositories</button><button class="btn primary" id="review" type="button">Review changes →</button></div>
</template>

<template id="review-panel">
  <h2>Review before cleaning</h2>
  <p class="sub"><span id="review-count"></span> commits have selected attribution. Source files stay intact.</p>
  <div id="changes" style="border:1px solid #33473a;border-radius:6px;max-height:340px;overflow:auto"></div>
  <div class="warning"><b>Important.</b> History rewriting changes commit hashes, invalidates commit signatures, and can affect collaborators, forks, open pull requests, and deployed references. AgentClean creates a backup branch before attempting a rewrite.</div>
  <section id="advanced-cleanup" hidden>
    <div id="rewrite-blocked" class="error"></div>
    <div class="notice"><strong>Advanced cleanup</strong><p>Use the local CLI for large or merge-heavy histories. It preserves source trees, human authors, commit order, and merge history.</p>
      <div id="cli-command" class="code"></div>
      <p id="local-backup-note" hidden>To publish the session-marker cleanup, use the CLI’s <code>--backup-file</code> option to keep the original history locally.</p>
      <p><a href="https://github.com/canadian-ai/agent-clean/tree/main/scripts" target="_blank" rel="noopener noreferrer">Get AgentClean CLI and documentation</a></p>
    </div>
  </section>
  <form id="rewrite-form">
    <label class="inlinecheck"><input id="accept" type="checkbox">I understand this rewrites published Git history, creates a backup branch, and may require collaborators to re-clone or reset.</label>
    <div class="field"><label for="confirm">Type REWRITE HISTORY to confirm</label><input type="text" id="confirm" autocomplete="off"></div>
    <div class="foot"><button class="btn" id="back-to-scan" type="button">← Scan</button><button id="execute" class="btn primary" type="submit">Create backup &amp; clean →</button></div>
  </form>
</template>

<template id="changed-commit">
  <div class="commit"><div style="flex:1;min-width:0">
    <div class="commit-title"></div><div class="meta" data-commit-meta></div>
    <div data-identity-changes></div><div data-attribution-badges></div>
    <details style="margin-top:10px" hidden><summary style="font-size:12px;color:#9dcdaa;cursor:pointer">View original → cleaned message</summary><div class="code"></div></details>
  </div></div>
</template>

<template id="clean-panel">
  <div class="success">
    <h2 style="margin-bottom:12px">Cleanup complete ✓</h2>
    <p>Updated <span id="clean-count"></span> commits in <b id="clean-repo"></b>.</p>
    <div id="backup-info" hidden><p>Backup branch <b id="backup-branch"></b></p><p>New HEAD <code id="new-head"></code></p></div>
    <p id="no-changes" hidden>No changes were needed.</p>
  </div>
  <p class="sub" style="margin-top:25px">Share the new history with collaborators. Existing clones may need to synchronize manually.</p>
  <div class="foot">
    <button class="btn" id="restart" type="button">Scan another repository</button>
    <a id="view-repo" class="btn primary" style="text-decoration:none" target="_blank" rel="noopener noreferrer">View repository ↗</a>
  </div>
</template>
`
