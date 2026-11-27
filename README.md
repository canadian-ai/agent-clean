# AgentClean

**Your code. Your commits. Your attribution.**

A browser-based cleanup tool from Canadian AI Solutions.

## Features

- Scan a public GitHub repository without a token.
- Scan private repositories with a fine-grained GitHub personal access token.
- Detect optional attribution footers from Claude Code, Codex, Cursor, and Gemini CLI.
- Preview Claude session markers separately. Use the CLI to remove them and keep the original history in a local bundle.
- Optionally unlink recognized automated authors and committers, including Claude, GitHub Actions, Vercel, and GitHub bot accounts. Their commits use `Repository Automation <automation@repository.invalid>` and human authors keep their credits.
- Preview changes to commit messages, authors, and committers before rewriting.
- Rewrite eligible linear Git history in the browser after explicit confirmation, with a backup branch.
- For large histories, merges, or signed commits, use the local history-preserving cleanup CLI below.

## Running locally

Run the web app from the repository root with Go 1.25 or newer.

```sh
go run ./cmd/agentclean
```

Open http://127.0.0.1:3000. Go renders the HTML templates and handles the GitHub API routes. The browser script in `public/app.js` manages the five-step wizard and keeps the token in tab memory.

### Vercel

Use the repository root with the **Other** framework preset. Vercel builds the Go handlers in `api/`, rewrites `/` to the Go page handler, and serves assets from `public/`. Use `vercel dev --local` to check routing without linking a project.

Run web tests with `go test ./...`. The independent history cleanup CLI still uses Python and Git.

No application environment variables are required. Users optionally enter a GitHub fine-grained PAT scoped to the repository with **Contents: Read and write** permission for private scanning and write operations.

## Large-history cleanup (Git CLI)

The browser rewrite is deliberately limited to small, unsigned, linear histories.
For repositories like `canadian-ai/girl`, use the local CLI with
[`git-filter-repo`](https://github.com/newren/git-filter-repo).
By default it removes optional AI attribution lines from commit messages. Add
`--unlink-automation` to also unlink recognized automated author, committer, and
co-author accounts. The CLI preserves human identities, file trees, and merge
structure. Rewritten commits have new IDs and lose their original signatures.

```sh
python -m pip install git-filter-repo
python scripts/history-clean.py https://github.com/canadian-ai/girl.git --branch master
python scripts/history-clean.py https://github.com/canadian-ai/girl.git --branch master --apply --unlink-automation
# Only during a coordinated maintenance window, when ready to publish:
python scripts/history-clean.py https://github.com/canadian-ai/girl.git --branch master --apply --unlink-automation --push --confirm 'REWRITE HISTORY' --backup-file /path/to/private-backup.bundle
```

After verifying the tip tree, commit count, and merge count, the CLI atomically
creates `agentclean-backup-<timestamp>` and updates the target branch. Git's
`--force-with-lease` rejects the push if the target moved since the original scan.

To remove only Claude session markers, pass `--session-markers-only`. Publishing a
cleanup that removes session markers requires `--backup-file` so the previous
history is kept in a verified local Git bundle. Store the bundle in a private directory.
Rewriting affects the selected branch. Other branches, tags, forks, and old commit
links can still contain the original markers.
Coordinate with maintainers; pause CI/PR merges and update clones, open PRs,
release references and any signed-commit policies after rewriting.
The original signed history remains accessible through the backup branch.
GitHub can take about 24 hours to refresh contributor data. Existing tags and
branches retain their original history. An enabled workflow can add the same
account again if it continues to create commits under that identity.

Run CLI unit tests: `python -m unittest discover -s tests -p 'test_history_clean.py'`.

## Safety and limitations

- The cleanup edits matching commit attribution and preserves source files.
- The UI requires a confirmed preview, checked warning, and the exact phrase `REWRITE HISTORY` before rewriting.
- Rewriting creates a timestamped backup branch before updating the chosen branch.
- The browser app refuses protected branches, signed commits, merge histories, and histories of 100 or more commits; the local CLI handles large/merge histories with explicit backups and leases.
- The app rechecks the branch HEAD before applying the final ref update; nonetheless, repository owners should coordinate a maintenance window and ensure no concurrent pushes.
- Commit hashes necessarily change when rewriting, and rewritten commits lose cryptographic signatures.
- Repository tokens are held in browser-tab memory and sent only to the Vercel API over HTTPS as necessary; they are not persisted by this code.
- This is intentionally not a general-purpose rewriting engine for complex repository histories. For those, use a carefully reviewed local Git migration.

## Security model

GitHub credentials are supplied per request rather than stored in a database. The API uses them only for calls to GitHub, and the frontend clears the token after a successful cleanup. Host this only behind HTTPS and don't enable request-header logging. Because GitHub does not offer transactions for multi-commit rewrites, the backup branch is the recovery point if an operation fails.

## Future improvements

GitHub App authentication, scanning other branches, batching large histories through a job queue, dry-run export, and dedicated protection for open pull requests.

MIT licensed.
