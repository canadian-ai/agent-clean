"use strict";

const panels = ["connect", "repository", "scan", "review", "clean"];
const confirmation = "REWRITE HISTORY";
const agentNames = new Map(
  [...document.getElementById("scan-panel").content.querySelectorAll("[data-agent]")]
    .map(input => [input.dataset.agent, input.closest("label").querySelector("strong").textContent])
);

function initialState() {
  return {
    step: 0, token: "", connection: null, repositories: [], search: "",
    selectedRepo: null, scan: null, agents: new Set(agentNames.keys()),
    unlinkAutomation: false, accepted: false, typed: "", busy: false,
    error: "", result: null
  };
}

let state = initialState();
const byID = id => document.getElementById(id);
const setText = (id, value) => { byID(id).textContent = value; };
const clone = id => byID(id).content.cloneNode(true);

function go(step) {
  state.step = step;
  state.error = "";
  render();
}

function resetConfirmation() {
  state.accepted = false;
  state.typed = "";
}

function selectedAgent(agent) {
  return state.agents.has(agent) || (agent === "automation" && state.unlinkAutomation);
}

function eligible(commit) {
  return commit.lineMatches.some(line => line.agents.some(selectedAgent)) ||
    (state.unlinkAutomation && Object.keys(commit.automation).length > 0);
}

function cleanedMessage(commit) {
  const removed = new Set(commit.lineMatches.filter(line => line.agents.some(selectedAgent)).map(line => line.index));
  if (!removed.size) return commit.message;
  return commit.message.split("\n").filter((_, index) => !removed.has(index))
    .join("\n").replace(/\n{3,}/g, "\n\n").trimEnd();
}

async function api(path, body) {
  const headers = {"Content-Type": "application/json"};
  if (state.token) headers["X-GitHub-Token"] = state.token;
  const response = await fetch("/api/" + path, {method: "POST", headers, body: JSON.stringify(body)});
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "GitHub request failed");
  return data;
}

async function runRequest(request) {
  if (state.busy) return;
  state.busy = true;
  state.error = "";
  render();
  try {
    await request();
  } catch (error) {
    state.error = error.message;
  } finally {
    state.busy = false;
    render();
  }
}

function repositoryRequest(action) {
  const repo = state.selectedRepo;
  return {
    action, owner: repo.owner, repo: repo.name, branch: repo.default_branch || "main",
    agents: [...state.agents], unlinkAutomation: state.unlinkAutomation,
    head: state.scan?.head, confirm: state.typed
  };
}

function renderConnect() {
  const input = byID("token");
  input.value = state.token;
  input.disabled = state.busy;
  byID("connect").disabled = state.busy;
  if (state.busy) setText("connect", "Connecting…");
  byID("connect-form").addEventListener("submit", event => {
    event.preventDefault();
    state.token = input.value.trim();
    if (!state.token) {
      state.error = "Enter a GitHub personal access token.";
      render();
      return;
    }
    runRequest(async () => {
      const data = await api("repositories", {});
      state.connection = data.user;
      state.repositories = data.repositories;
      state.search = "";
      state.step = 1;
    });
  });
}

function renderRepositories() {
  setText("login", state.connection?.login || "GitHub user");
  setText("repository-count", state.repositories.length);
  const input = byID("repo-search");
  input.value = state.search;
  input.disabled = state.busy;
  input.addEventListener("input", () => {
    state.search = input.value;
    renderRepositoryRows();
  });
  byID("change-token").disabled = state.busy;
  byID("change-token").addEventListener("click", () => go(0));
  renderRepositoryRows();
}

function renderRepositoryRows() {
  const list = byID("repo-results");
  list.replaceChildren();
  if (state.busy) {
    list.textContent = "Scanning repository…";
    return;
  }
  const query = state.search.toLowerCase();
  const matches = state.repositories.filter(repo => repo.full_name.toLowerCase().includes(query)).slice(0, 75);
  if (!matches.length) {
    list.textContent = "No repositories found for this token and search.";
    return;
  }
  for (const repo of matches) {
    const row = clone("repository-row");
    row.querySelector("[data-repo-name]").textContent = repo.full_name;
    row.querySelector("[data-repo-info]").textContent = (repo.private ? "Private" : "Public") + " · " + (repo.default_branch || "main");
    row.firstElementChild.addEventListener("click", () => {
      state.selectedRepo = repo;
      state.scan = null;
      state.unlinkAutomation = false;
      resetConfirmation();
      runRequest(async () => {
        state.scan = await api("repository", repositoryRequest("scan"));
        state.step = 2;
      });
    });
    list.append(row);
  }
}

function renderScan() {
  const scan = state.scan;
  const contributors = scan.contributorCheck;
  setText("scanned-repo", scan.repo);
  setText("scanned-branch", scan.branch);
  setText("scan-count", scan.scanned);
  setText("attribution-count", scan.available);
  setText("human-count", contributors.humanCount);
  setText("automation-count", contributors.autoApproved.length);
  byID("partial-audit").hidden = !contributors.partial;
  byID("large-history").hidden = scan.scanned < 100;
  const list = byID("automation-list");
  list.hidden = contributors.autoApproved.length === 0;
  for (const automation of contributors.autoApproved) {
    const item = document.createElement("li");
    item.textContent = automation.name + " · " + automation.category + " · " + automation.commits + " commits";
    list.append(item);
  }
  const checkbox = byID("unlink-automation");
  checkbox.checked = state.unlinkAutomation;
  checkbox.addEventListener("change", () => {
    state.unlinkAutomation = checkbox.checked;
    updateSelection();
  });
  for (const input of byID("panel").querySelectorAll("[data-agent]")) {
    const agent = input.dataset.agent;
    input.checked = state.agents.has(agent);
    const matches = scan.commits.filter(commit => commit.lineMatches.some(line => line.agents.includes(agent))).length;
    input.closest("label").querySelector("[data-count]").textContent = matches + " matching commits";
    input.addEventListener("change", () => {
      if (input.checked) state.agents.add(agent);
      else state.agents.delete(agent);
      updateSelection();
    });
  }
  byID("back-to-repos").addEventListener("click", () => go(1));
  byID("review").addEventListener("click", () => go(3));
  updateSelection();
}

function updateSelection() {
  resetConfirmation();
  const count = state.scan.commits.filter(eligible).length;
  setText("selected-count", count);
  byID("review").disabled = count === 0 && !(state.unlinkAutomation && state.scan.scanned >= 100);
}

function rewriteBlockReason(scan) {
  if (hasSelectedSessionMarkers(scan)) return "Use the CLI with a local backup for Claude session markers.";
  if (scan.protected) return "Branch is protected.";
  if (scan.scanned >= 100) return "History exceeds 99 commits.";
  if (scan.commits.some(commit => commit.signed || commit.parents > 1)) return "Merge commits or signed commits were detected.";
  return "";
}

function hasSelectedSessionMarkers(scan) {
  return state.agents.has("claude-session") && scan.commits.some(commit => commit.removed["claude-session"]);
}

function renderCommit(commit) {
  const fragment = clone("changed-commit");
  fragment.querySelector(".commit-title").textContent = commit.message.split("\n")[0];
  fragment.querySelector("[data-commit-meta]").textContent =
    [commit.sha.slice(0, 8), commit.author, commit.date?.slice(0, 10)].filter(Boolean).join(" · ");
  if (state.unlinkAutomation) {
    for (const [role, name] of Object.entries(commit.automation)) {
      const entry = document.createElement("div");
      entry.className = "meta";
      entry.textContent = (role === "author" ? "Author" : "Committer") + " · " + name + " → " + state.scan.unlinkedIdentity.name;
      fragment.querySelector("[data-identity-changes]").append(entry);
    }
  }
  const matchingAgents = new Set(commit.lineMatches.flatMap(line => line.agents.filter(selectedAgent)));
  for (const agent of matchingAgents) {
    const badge = document.createElement("span");
    badge.className = "pill";
    badge.textContent = agentNames.get(agent) || "Automated co-author";
    fragment.querySelector("[data-attribution-badges]").append(badge);
  }
  const after = cleanedMessage(commit);
  if (after !== commit.message) {
    fragment.querySelector("details").hidden = false;
    fragment.querySelector(".code").textContent = commit.message + "\n\n──────── AFTER ────────\n" + after;
  }
  return fragment;
}

function renderReview() {
  const scan = state.scan;
  const affected = scan.commits.filter(eligible);
  const blocked = rewriteBlockReason(scan);
  setText("review-count", affected.length);
  if (!affected.length) setText("changes", "No matching commits in this scan.");
  for (const commit of affected) byID("changes").append(renderCommit(commit));
  if (blocked) {
    byID("advanced-cleanup").hidden = false;
    setText("rewrite-blocked", "Browser cleanup unavailable. " + blocked);
    const repo = state.selectedRepo;
    const branch = repo.default_branch || "main";
    const sessionMarkers = hasSelectedSessionMarkers(scan);
    setText("cli-command", "python -m pip install git-filter-repo\npython scripts/history-clean.py https://github.com/" +
      repo.owner + "/" + repo.name + ".git --branch " + branch + " --apply" +
      (sessionMarkers ? " --session-markers-only" : state.unlinkAutomation ? " --unlink-automation" : ""));
    byID("local-backup-note").hidden = !sessionMarkers;
  }

  const accept = byID("accept");
  const input = byID("confirm");
  const execute = byID("execute");
  accept.checked = state.accepted;
  input.value = state.typed;
  accept.disabled = input.disabled = Boolean(blocked) || state.busy;
  byID("back-to-scan").disabled = state.busy;
  byID("back-to-scan").addEventListener("click", () => go(2));
  if (state.busy) setText("execute", "Cleaning…");

  function updateConfirmation() {
    state.accepted = accept.checked;
    state.typed = input.value;
    execute.disabled = Boolean(blocked) || state.busy || !state.accepted || state.typed !== confirmation;
  }

  accept.addEventListener("change", updateConfirmation);
  input.addEventListener("input", updateConfirmation);
  byID("rewrite-form").addEventListener("submit", event => {
    event.preventDefault();
    if (execute.disabled) return;
    runRequest(async () => {
      state.result = await api("repository", repositoryRequest("apply"));
      state.step = 4;
      state.token = "";
    });
  });
  updateConfirmation();
}

function renderClean() {
  const result = state.result;
  const repo = state.selectedRepo;
  setText("clean-count", result.changed);
  setText("clean-repo", repo.full_name);
  byID("backup-info").hidden = !result.backup;
  byID("no-changes").hidden = Boolean(result.backup);
  if (result.backup) {
    setText("backup-branch", result.backup);
    setText("new-head", result.head.slice(0, 12));
  }
  byID("view-repo").href = "https://github.com/" + encodeURIComponent(repo.owner) + "/" + encodeURIComponent(repo.name);
  byID("restart").addEventListener("click", () => {
    state = initialState();
    render();
  });
}

function render() {
  [...byID("steps").children].forEach((step, index) => {
    step.classList.toggle("active", index === state.step);
    step.classList.toggle("done", index < state.step);
    step.querySelector(".num").textContent = index < state.step ? "✓" : index + 1;
    if (index === state.step) step.setAttribute("aria-current", "step");
    else step.removeAttribute("aria-current");
  });
  const panel = byID("panel");
  panel.replaceChildren(clone(panels[state.step] + "-panel"));
  [renderConnect, renderRepositories, renderScan, renderReview, renderClean][state.step]();
  if (state.error) {
    const error = document.createElement("div");
    error.className = "error";
    error.setAttribute("role", "alert");
    error.textContent = state.error;
    panel.append(error);
  }
}

render();
