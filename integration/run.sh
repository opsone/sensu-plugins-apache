#!/usr/bin/env bash
# Runs check-apache-status against a real Apache (httpd event MPM, 20 workers) in Docker.
set -euo pipefail
cd "$(dirname "$0")"

image=check-apache-integration
runner=check-apache-runner-$$

docker build -q -t "$image" . >/dev/null
cleanup() { docker rm -f "$runner" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$runner" "$image" >/dev/null
for _ in $(seq 30); do docker exec "$runner" bash -c 'exec 3<>/dev/tcp/127.0.0.1/80' 2>/dev/null && break; sleep 1; done

# Holds n workers busy with slow CGI requests (in the background, for 120 s).
hold() {
  docker exec -d "$runner" bash -c "for i in \$(seq $1); do
    (exec 3<>/dev/tcp/127.0.0.1/80; printf 'GET /cgi-bin/sleep HTTP/1.0\r\n\r\n' >&3; cat <&3 >/dev/null) &
  done; wait"
  sleep 2
}

failures=0
expect() {
  local want=$1 desc=$2; shift 2
  local out rc=0
  # Run as an unprivileged user, like sensu-agent does.
  out=$(docker exec "$runner" runuser -u nobody -- "$@" 2>&1) || rc=$?
  out=$(tail -1 <<<"$out")
  if [[ $rc -eq $want ]]; then
    echo "ok   [$rc] $desc -> $out"
  else
    echo "FAIL [$rc, want $want] $desc -> $out"
    failures=$((failures + 1))
  fi
}

args=(check-apache-status -H 127.0.0.1 -p 80 -q apache-status)
expect 3 "without host" check-apache-status -p 80 -q apache-status
expect 3 "without port" check-apache-status -H 127.0.0.1 -q apache-status
expect 3 "without path" check-apache-status -H 127.0.0.1 -p 80
expect 3 "warning above critical" "${args[@]}" -w 90 -c 80
expect 0 "idle server" "${args[@]}" -m 20
expect 0 "capacity from scoreboard" "${args[@]}"
expect 0 "leading slash in path" "${args[@]}" -q /apache-status
expect 1 "path not found (404)" "${args[@]}" -q nope
expect 1 "redirected path (301)" "${args[@]}" -q old-status
expect 1 "denied path (403)" "${args[@]}" -q denied-status
expect 1 "not a status page (200)" "${args[@]}" -q index.html
expect 2 "server error (500)" "${args[@]}" -q cgi-bin/broken
expect 2 "connection refused" "${args[@]}" -p 1
expect 2 "unreachable host (timeout)" "${args[@]}" -H 192.0.2.1 --timeout 1

hold 16 # 16 + the status request = 17/20 (85 %)
expect 1 "85% busy" "${args[@]}" -m 20
expect 0 "85% busy, higher capacity" "${args[@]}" -m 150
# Not 20/20: with every worker busy, the status request itself may not be served.
expect 2 "85% busy, critical at 85%" "${args[@]}" -m 20 -w 50 -c 85

exit $failures
