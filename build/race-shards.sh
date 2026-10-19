#!/usr/bin/env bash
# race-shards.sh DIR BIN N [FLAGS]: runs the tests of BIN, a package's test
# binary (go test -c), from the package's DIR in N processes at once, the
# tests split round-robin by name, and fails if any process does. FLAGS go
# to every process. A package's tests run one at a time unless they call
# t.Parallel, which t.Setenv rules out, so the gateway's suite under the
# race detector keeps one core busy; N processes keep N busy. Each process
# isolates its own home (the package's TestMain).
set -euo pipefail
pkg=$1 bin=$2 n=$3
shift 3
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
(cd "$pkg" && "$bin" -test.list '.*') | grep -E '^(Test|Example|Fuzz)' | sort >"$dir/all"
echo "$(wc -l <"$dir/all" | tr -d ' ') tests in $n processes"
pids=()
for i in $(seq 0 $((n - 1))); do
	awk -v n="$n" -v i="$i" 'NR % n == i' "$dir/all" >"$dir/s$i"
	re="^($(paste -sd'|' "$dir/s$i"))\$"
	(cd "$pkg" && "$bin" -test.run "$re" "$@" >"$dir/log$i" 2>&1) &
	pids+=($!)
done
fail=0
for i in "${!pids[@]}"; do
	if wait "${pids[$i]}"; then st=ok; else st=FAIL fail=1; fi
	echo "::group::shard $i: $st ($(wc -l <"$dir/s$i" | tr -d ' ') tests)"
	cat "$dir/log$i"
	echo "::endgroup::"
	if [ $st = FAIL ]; then grep -E '^(--- FAIL|FAIL|panic|WARNING: DATA RACE)' "$dir/log$i" || true; fi
done
exit $fail
