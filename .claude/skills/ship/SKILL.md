---
name: ship
description: Commit all current changes with an auto-generated message and push to the current branch on origin. Use when the user types /ship or says "ship it", "commit and push", "push my changes".
---

# ship

Commit every pending change with a message you write from the diff, then push to
the current branch (`origin`). The user has opted into pushing directly to the
current branch (currently `master`) — do not create a new branch.

## Steps

1. Run `git status -s`. If there is nothing to commit, say so and stop — do not
   create an empty commit.
2. Stage everything: `git add -A`.
3. Read the staged diff to write the message:
   - `git diff --cached --stat` for the shape, and skim `git diff --cached` for
     the substance.
   - Subject line: concise, imperative, ≤72 chars, conventional-commit style when
     it fits (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`). Describe the
     actual change, not "update files".
   - Add a short body only if the change spans several areas; one bullet each.
4. Commit. End the message with this trailer on its own line:

   ```
   Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
   ```

   Use `git commit -F -` with a heredoc so the multi-line message + trailer go in
   cleanly.
5. Push: `git push origin HEAD`. If the branch has no upstream, use
   `git push -u origin HEAD`.
6. Report the commit subject, short hash, and that the push succeeded. If the push
   is rejected (e.g. non-fast-forward), stop and show the error — do not force.

## Guardrails

- Never `git push --force` or `--force-with-lease` unless the user explicitly asks.
- Never amend or rewrite already-pushed commits.
- Don't touch `.env` or secrets — they're gitignored; if something sensitive is
  staged, flag it before committing.
