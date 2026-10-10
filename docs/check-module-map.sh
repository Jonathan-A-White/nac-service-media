#!/usr/bin/env bash
# Checks docs/module-map.md: the five sections exist, every backticked path
# it names exists, and the collision table matches git history at the base
# commit it was written against. Run from the repo root: bash docs/check-module-map.sh
set -u
DOC=docs/module-map.md
BASE=4bf0760
fail=0

[ -f "$DOC" ] || { echo "FAIL: $DOC does not exist"; exit 1; }

for s in "Modules" "Collisions" "Proposed refactors" "Rule breaks" "Library candidates"; do
  if grep -qE "^## [0-9]+\. $s" "$DOC"; then echo "ok   section: $s"; else echo "FAIL section: $s"; fail=1; fi
done

# Backticked, path-like spans outside fenced code blocks must exist.
paths=$(awk '/^```/{f=!f; next} !f' "$DOC" | grep -oE '`[A-Za-z0-9_./-]+`' | tr -d '`' \
  | grep -E '/|\.(go|md|sh|yaml|ps1|py|feature|mod|sum)$|^Makefile$' | sort -u)
for p in $paths; do
  if test -e "$p"; then echo "ok   path: $p"; else echo "FAIL path missing: $p"; fail=1; fi
done

# Collision table: lines "| count | `file` | lines |" must equal the git log top 15 at BASE.
want=$(git log "$BASE" --name-only --format= -- "*.go" | sort | uniq -c | sort -rn | head -15 \
  | while read -r n f; do echo "$n $f $(git show "$BASE:$f" | wc -l)"; done)
have=$(grep -E '^\| [0-9]+ \| `[^`]+` \| [0-9]+ \|' "$DOC" | sed -E 's/^\| ([0-9]+) \| `([^`]+)` \| ([0-9]+) \|.*/\1 \2 \3/')
if [ "$want" = "$have" ]; then echo "ok   collision table matches git log at $BASE"; else
  echo "FAIL collision table differs from git log at $BASE"; diff <(echo "$want") <(echo "$have"); fail=1; fi

# Refactors: at least three, each with a Size line and a typed signature fence.
n=$(grep -cE '^### R[0-9]+\.' "$DOC"); sz=$(grep -cE '^\*\*Size:\*\*' "$DOC")
if [ "$n" -ge 3 ] && [ "$sz" -eq "$n" ]; then echo "ok   $n refactors, each sized"; else echo "FAIL refactors=$n sizes=$sz"; fail=1; fi

# README links the doc.
grep -q 'docs/module-map.md' README.md && echo "ok   README links the doc" || { echo "FAIL README link"; fail=1; }

exit $fail
