#!/usr/bin/env bash
set -euo pipefail

find-sqlc-file() {
  local dir="$1"

  test -e "$dir/sqlc.yml"  && echo "$dir" && return
  test -e "$dir/sqlc.yaml" && echo "$dir" && return
  test -e "$dir/sqlc.json" && echo "$dir" && return

  [ '/' = "$dir" ] && return

  find-sqlc-file "$(dirname "$dir")"
}

initial_dir="$(dirname $1)"
dir=$(find-sqlc-file $initial_dir)
if [[ -z "$dir" ]]; then
  echo "sqlc file not found in $PWD"
  exit 1
elif [[ -n "$dir" ]]; then
  cd "$dir" && sqlc generate
fi
