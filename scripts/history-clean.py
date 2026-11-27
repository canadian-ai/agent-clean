#!/usr/bin/env python3
"""Remove optional AI trailers and, on request, unlink automation identities.

Requires git-filter-repo: python -m pip install git-filter-repo
Preview by default; --apply rewrites a temporary clone. --push additionally
backs up the original HEAD and pushes with an explicit lease.
"""
import argparse
import datetime
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile

SESSION_MARKER = re.compile(rb"^[ \t]*claude-session:[ \t]*.*$", re.IGNORECASE)
TRAILERS = (
    rb"^[ \t]*co-authored-by:[ \t]*claude(?:[ \t]+(?:code|opus(?:[ \t]+[0-9.]+)?|sonnet(?:[ \t]+[0-9.]+)?|haiku(?:[ \t]+[0-9.]+)?))?[ \t]*<[^<>]+>\s*$",
    rb"^[ \t]*\xf0\x9f\xa4\x96[ \t]*generated with \[?claude code\]?.*$",
    rb"^[ \t]*co-authored-by:[ \t]*(?:openai[ \t]+)?codex[ \t]*<[^<>]+>\s*$",
    rb"^[ \t]*(?:generated|assisted) (?:with|by) (?:openai[ \t]+)?codex\b.*$",
    rb"^[ \t]*co-authored-by:[ \t]*cursor[ \t]*<[^<>]+>\s*$",
    rb"^[ \t]*(?:generated|assisted) (?:with|by) cursor\b.*$",
    rb"^[ \t]*co-authored-by:[ \t]*(?:google[ \t]+)?gemini(?:[ \t]+cli)?[ \t]*<[^<>]+>\s*$",
    rb"^[ \t]*co-authored-by:[ \t]*(?:codesmith|opencode(?:-agent)?(?:\[bot\])?)[ \t]*<[^<>]+>\s*$",
    rb"^[ \t]*(?:generated|assisted) (?:with|by) gemini(?: cli)?\b.*$",
)
PATTERNS = (SESSION_MARKER, *(re.compile(pattern, re.IGNORECASE) for pattern in TRAILERS))


def clean_message(message, unlink_automation=False, session_markers_only=False):
    """Remove matching attribution lines from a commit message."""
    lines = message.split(b"\n")
    patterns = (SESSION_MARKER,) if session_markers_only else PATTERNS
    filtered = [line for line in lines if not any(p.match(line) for p in patterns)
                and not (unlink_automation and automated_coauthor(line))]
    if len(filtered) == len(lines):
        return message
    if session_markers_only:
        return b"\n".join(filtered)
    return re.sub(rb"\n{3,}", b"\n\n", b"\n".join(filtered)).rstrip(b"\n") + b"\n"


def git(*args, cwd=None, capture=True):
    return subprocess.run(
        ["git", *args], cwd=cwd, check=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.PIPE if capture else None,
    ).stdout



# Match known addresses and GitHub bot accounts. A human name alone is never evidence.
AGENT_EMAILS = {"noreply@anthropic.com", "codesmith-bot@users.noreply.github.com"}
BOT_LOGIN = re.compile(r"^(?:[\w-]+\[bot\]|github-actions|dependabot|renovate|vercel)$", re.ASCII)
UNLINKED_NAME = b"Repository Automation"
UNLINKED_EMAIL = b"automation@repository.invalid"


def contributor_category(name, email):
    email = email.lower()
    if re.fullmatch(r"automation@[a-z0-9-]+\.invalid", email):
        return "unlinked-automation"
    if email in AGENT_EMAILS:
        return "ai-agent"
    login, _, host = email.partition("@")
    login = re.sub(r"^[0-9]+\+", "", login)
    if host == "users.noreply.github.com" and BOT_LOGIN.fullmatch(login):
        return "automation"
    return "human"


def unlink_identity(name, email):
    if contributor_category(name.decode("utf-8", "replace"), email.decode("utf-8", "replace")) in ("ai-agent", "automation"):
        return UNLINKED_NAME, UNLINKED_EMAIL
    return name, email


def automated_coauthor(line):
    match = re.fullmatch(rb"[ \t]*co-authored-by:[ \t]*[^<>]*<([^<>]+)>\s*", line, re.I)
    return bool(match and contributor_category("", match[1].decode("utf-8", "replace")) in ("ai-agent", "automation"))


def clean_commit(commit, unlink_automation, session_markers_only):
    commit.message = clean_message(commit.message, unlink_automation, session_markers_only)
    if unlink_automation:
        commit.author_name, commit.author_email = unlink_identity(commit.author_name, commit.author_email)
        commit.committer_name, commit.committer_email = unlink_identity(commit.committer_name, commit.committer_email)


def contributor_audit(repo, branch, mailmap=True):
    """Report actual Git author/committer identities, separate from trailers.

    It applies Git .mailmap aliases to consolidate equivalent human identities.
    Bot authors are auto-classified, not credited to a human. This is a
    Git-history approximation, not an exact GitHub Contributors API count.
    """
    fields = "%aN%x00%aE%x00%cN%x00%cE" if mailmap else "%an%x00%ae%x00%cn%x00%ce"
    raw = git("log", "--format=" + fields + "%x00%x1e", branch, cwd=repo)
    authors = {}
    committers = {}
    for record in raw.split(b"\x1e"):
        fields = record.strip(b"\n").split(b"\x00")
        if len(fields) < 4:
            continue
        author = tuple(v.decode("utf-8", "replace") for v in fields[:2])
        committer = tuple(v.decode("utf-8", "replace") for v in fields[2:4])
        authors[author] = authors.get(author, 0) + 1
        committers[committer] = committers.get(committer, 0) + 1

    def rows(identities):
        return sorted(
            (
                {"name": name, "email": email, "commits": count,
                 "category": contributor_category(name, email)}
                for (name, email), count in identities.items()
            ),
            key=lambda row: (-row["commits"], row["name"], row["email"]),
        )

    return {"authors": rows(authors), "committers": rows(committers)}


def print_contributor_check(audit):
    print("Contributor identity check (Git history, not GitHub UI):")
    for role in ("authors", "committers"):
        bots = [x for x in audit[role] if x["category"] != "human"]
        print(f"  {role}: {len(audit[role])} identities; {len(bots)} automation/AI identities")
        for item in bots:
            print(f"    [{item['category']}] {item['name']} <{item['email']}>: {item['commits']} commits")
    humans = [x for x in audit["authors"] if x["category"] == "human"]
    print(f"  Human identities (mailmap-normalized): {len(humans)}")
    if any(x["category"] in ("ai-agent", "automation") for x in audit["authors"]):
        print("  Recognized automated authors can be unlinked with --unlink-automation.")
    else:
        print("  CONTRIBUTOR CHECK: PASS (no recognized automated Git authors)")


def summarize(repo, branch, unlink_automation=False, session_markers_only=False):
    head = git("rev-parse", branch, cwd=repo).strip().decode()
    tree = git("rev-parse", branch + "^{tree}", cwd=repo).strip().decode()
    count = int(git("rev-list", "--count", branch, cwd=repo).strip())
    merges = int(git("rev-list", "--min-parents=2", "--count", branch, cwd=repo).strip())
    raw = git("log", "--format=%B%x00", branch, cwd=repo)
    matches = sum(clean_message(msg, unlink_automation, session_markers_only) != msg
                  for msg in raw.split(b"\x00") if msg.strip())
    return dict(head=head, tree=tree, commits=count, merges=merges, matches=matches)


def save_local_backup(repo, old, backup, destination):
    """Write a verified, independent Git bundle without overwriting an existing path."""
    git("update-ref", backup, old, cwd=repo)
    with tempfile.TemporaryDirectory(prefix=".agentclean-", dir=destination.parent) as scratch:
        bundle = pathlib.Path(scratch) / "backup.bundle"
        git("bundle", "create", str(bundle), backup, cwd=repo, capture=False)
        git("bundle", "verify", str(bundle), cwd=repo, capture=False)
        os.link(bundle, destination)


def rewrite(source, branch, push=False, unlink_automation=False, session_markers_only=False, backup_file=None):
    if shutil.which("git-filter-repo") is None:
        raise RuntimeError("Install git-filter-repo first: python -m pip install git-filter-repo")
    # A single-branch clone includes all ancestors and merge parents of this branch.
    with tempfile.TemporaryDirectory(prefix="agentclean-") as directory:
        repo = pathlib.Path(directory) / "repo"
        git("clone", "--no-tags", "--single-branch", "--branch", branch,
            source, str(repo), capture=False)
        before = summarize(repo, "HEAD", unlink_automation, session_markers_only)
        original_audit = contributor_audit(repo, "HEAD", mailmap=False)
        print_contributor_check(contributor_audit(repo, "HEAD"))
        linked = any(row["category"] in ("ai-agent", "automation")
                     for rows in original_audit.values() for row in rows)
        if before["matches"] == 0 and not (unlink_automation and linked):
            print("No matching attribution; no rewrite needed.")
            return
        if push and backup_file is None:
            messages = git("log", "--format=%B", "HEAD", cwd=repo)
            if any(SESSION_MARKER.match(line) for line in messages.splitlines()):
                raise RuntimeError("Use --backup-file to publish a Claude session marker cleanup with a local backup")
        old = before["head"]
        # Validate that the target branch has not moved since the initial clone.
        remote_ref = "refs/heads/" + branch
        advertised = git("ls-remote", "--heads", "origin", remote_ref, cwd=repo).decode().split()
        if not advertised or advertised[0] != old:
            raise RuntimeError("Remote branch changed during clone; aborting")
        script_path = str(pathlib.Path(__file__).resolve().parent)
        callback = (
            "import importlib, sys\n"
            f"if {script_path!r} not in sys.path: sys.path.insert(0, {script_path!r})\n"
            f"importlib.import_module('history-clean').clean_commit(commit, {unlink_automation!r}, {session_markers_only!r})\n"
        )
        # Rewrite the checked-out local branch, not the remote-tracking ref.
        # Disabling pruning is essential: an attribution-only edit must not
        # change the number of commits or the merge topology.
        subprocess.run(
            ["git", "filter-repo", "--force", "--refs", "refs/heads/" + branch,
             "--prune-empty", "never", "--prune-degenerate", "never",
             "--replace-refs", "delete-no-add",
             "--preserve-commit-encoding", "--preserve-commit-hashes", "--commit-callback", callback],
            cwd=repo, check=True,
        )
        # filter-repo operates on a local branch; don't assume HEAD moved.
        after = summarize(repo, branch, unlink_automation, session_markers_only)
        updated_audit = contributor_audit(repo, branch, mailmap=False)
        for role, rows in original_audit.items():
            expected = {}
            for row in rows:
                key = (row["name"], row["email"])
                if unlink_automation and row["category"] in ("ai-agent", "automation"):
                    key = (UNLINKED_NAME.decode(), UNLINKED_EMAIL.decode())
                expected[key] = expected.get(key, 0) + row["commits"]
            actual = {(row["name"], row["email"]): row["commits"] for row in updated_audit[role]}
            if actual != expected:
                raise RuntimeError(f"Unexpected change in {role} attribution; no push")
        print_contributor_check(contributor_audit(repo, branch))
        if after["head"] == old:
            raise RuntimeError("Filter did not update the selected branch; no push")
        for invariant in ("tree", "commits", "merges"):
            if before[invariant] != after[invariant]:
                raise RuntimeError(f"History invariant {invariant} changed; no push")
        if after["matches"] != 0:
            raise RuntimeError("Matching agent attribution remains; no push")
        print(f"Verified: {before['commits']} commits, {before['merges']} merges, unchanged tip tree.")
        print(f"Rewritten HEAD: {after['head']}; original HEAD: {old}")
        if not push:
            print("Local rewrite verified; remote unchanged. Use --apply --push to publish.")
            return
        stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        backup = f"refs/heads/agentclean-backup-{stamp}"
        # A backup exists on the remote before the force-with-lease update.
        # The source clone is local and may have a stale origin after the filter.
        # Push explicitly to the source URL; never reuse a removed remote.
        lease = f"--force-with-lease={remote_ref}:{old}"
        update = f"{after['head']}:{remote_ref}"
        if backup_file is not None:
            save_local_backup(repo, old, backup, backup_file.resolve())
            print(f"Verified local backup: {backup_file}", flush=True)
            git("push", lease, source, update, cwd=repo, capture=False)
        else:
            git("push", "--atomic", lease, f"--force-with-lease={backup}:",
                source, f"{old}:{backup}", update, cwd=repo, capture=False)
            print(f"Remote backup: {backup}")
        print(f"Published rewritten {branch}.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("repository", help="Git URL, e.g. https://github.com/owner/repo.git")
    parser.add_argument("--branch", default="master", help="Branch to inspect or rewrite")
    parser.add_argument("--apply", action="store_true", help="Create and verify temporary rewrite")
    selection = parser.add_mutually_exclusive_group()
    selection.add_argument("--unlink-automation", action="store_true",
                           help="Replace recognized bot authors and committers with an unlinked automation identity")
    selection.add_argument("--session-markers-only", action="store_true",
                           help="Remove only Claude-Session markers while preserving all other attribution")
    parser.add_argument("--backup-file", type=pathlib.Path,
                        help="With --push, save a local Git bundle instead of creating a remote backup branch")
    parser.add_argument("--push", action="store_true", help="Publish with a backup and lease (requires --apply)")
    parser.add_argument("--confirm", default="", help="Must equal REWRITE HISTORY when --push is set")
    args = parser.parse_args()
    if args.push and (not args.apply or args.confirm != "REWRITE HISTORY"):
        parser.error("--push requires --apply --confirm 'REWRITE HISTORY'")
    if args.backup_file is not None and not args.push:
        parser.error("--backup-file requires --push")
    if not args.apply:
        with tempfile.TemporaryDirectory(prefix="agentclean-preview-") as directory:
            git("clone", "--no-tags", "--single-branch", "--branch", args.branch,
                args.repository, directory, capture=False)
            stats = summarize(directory, "HEAD", args.unlink_automation, args.session_markers_only)
            print(f"{stats['commits']} commits; {stats['merges']} merge commits; "
                  f"{stats['matches']} commits with removable attribution.")
            print_contributor_check(contributor_audit(directory, "HEAD"))
            print("Read-only preview. To rewrite locally, use --apply.")
        return
    rewrite(args.repository, args.branch, args.push, args.unlink_automation, args.session_markers_only, args.backup_file)


if __name__ == "__main__":
    try:
        main()
    except (subprocess.CalledProcessError, RuntimeError, OSError) as exc:
        print(f"AgentClean stopped: {exc}", file=sys.stderr)
        sys.exit(1)
