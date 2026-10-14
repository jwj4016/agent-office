#!/usr/bin/env bash
# Measures the performance scenario (spec §11) on macOS or Linux with an
# installed or built app: two services, 20 assignees, a 50-step flow.
#
#   scripts/perf.sh <app executable> [active-seconds] [idle-seconds]
#
# Three launches: idle (no runs), active (two AI steps working the whole
# time, test AI with 20-second steps) and one more launch for startup.
# Reported: process start → first screen with data, CPU (from cumulative
# CPU time, app + its WebKit processes), resident memory (sum of RSS, so
# shared pages are counted per process) and screen lag (a 100ms timer's
# delay inside the page). Results are printed and saved as perf-<os>.json.
# Keep the window in front: a hidden or covered window throttles its
# timers and stops the office animation (hiddenReports counts those).
set -euo pipefail
exe=${1:?app executable}
active=${2:-60}
idle=${3:-30}
work=$(mktemp -d)
data="$work/성능 데이터"
trap 'kill "${pid:-0}" 2>/dev/null || true; rm -rf "$work"' EXIT

"$exe" --perf-seed -data-dir "$data" >/dev/null

webkit_pids() {
  if [ "$(uname)" = Darwin ]; then pgrep -f 'com.apple.WebKit' | sort || true
  else pgrep -f 'WebKit(Web|Network|GPU)Process' | sort || true; fi
}

# seconds of CPU time used so far by the given pids
cpu_seconds() {
  ps -o time= -p "$(echo "$@" | tr ' ' ,)" 2>/dev/null | awk '
    { n = split($1, p, /[:-]/); s = 0
      if (n == 4) s = p[1]*86400 + p[2]*3600 + p[3]*60 + p[4]
      else if (n == 3) s = p[1]*3600 + p[2]*60 + p[3]
      else s = p[1]*60 + p[2]
      t += s } END { printf "%.2f", t }'
}
rss_mb() { ps -o rss= -p "$(echo "$@" | tr ' ' ,)" | awk '{ t += $1 } END { printf "%.0f", t / 1024 }'; }

launch() { # $1 = log file, $2 = runs on|off
  before=$(webkit_pids)
  AGENT_OFFICE_PERF="$1" AGENT_OFFICE_PERF_RUNS="$2" AGENT_OFFICE_DATA_DIR="$data" "$exe" >/dev/null 2>&1 &
  pid=$!
  for _ in $(seq 1 300); do grep -q '"ui-ready"' "$1" 2>/dev/null && break; sleep 0.2; done
  grep -q '"ui-ready"' "$1" || { echo "first screen never reported ($1):" >&2; cat "$1" >&2 2>/dev/null; exit 1; }
  sleep 2
  procs="$pid $(comm -13 <(echo "$before") <(webkit_pids) | tr '\n' ' ')"
}
stop() { kill "$pid"; wait "$pid" 2>/dev/null || true; sleep 4; }

ready_ms() { sed -n 's/.*"ms":\([0-9]*\),"name":"ui-ready".*/\1/p' "$1"; }

window() { # $1 = seconds -> "cpu_percent rss_mb peak_rss_mb"
  local c0 c1 peak=0 r
  c0=$(cpu_seconds $procs)
  for _ in $(seq 1 "$1"); do sleep 1; r=$(rss_mb $procs); [ "$r" -gt "$peak" ] && peak=$r; done
  c1=$(cpu_seconds $procs)
  echo "$(awk -v a="$c0" -v b="$c1" -v s="$1" 'BEGIN { printf "%.1f", (b - a) * 100 / s }') $(rss_mb $procs) $peak"
}

lag() { # worst p95, max, and reports taken while the window was hidden / all
  awk -F'"p95":' '/"name":"lag"/ { n++; if (/"hidden":true/) h++; split($2, a, ","); if (a[1] > p) p = a[1]; m = $0; sub(/.*"max":/, "", m); sub(/[,}].*/, "", m); if (m + 0 > mx) mx = m + 0 } END { printf "%d %d %d/%d", p, mx, h, n }' "$1"
}
lagjson() { set -- $1; echo "\"lagP95Ms\":$1,\"lagMaxMs\":$2,\"hiddenReports\":\"$3\""; }

log1="$work/idle.jsonl"; launch "$log1" off; nprocs=$(echo $procs | wc -w | tr -d ' ')
sleep 8; read -r idle_cpu idle_rss idle_peak < <(window "$idle"); idle_lag=$(lagjson "$(lag "$log1")"); stop
log2="$work/active.jsonl"; launch "$log2" on
read -r act_cpu act_rss act_peak < <(window "$active"); act_lag=$(lagjson "$(lag "$log2")"); stop
log3="$work/third.jsonl"; launch "$log3" off; stop

os=$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)
ver=$(sed -n 's/.*"version":"\([^"]*\)".*/\1/p' "$log1" | head -1)
cat > "perf-$os.json" <<JSON
{"os":"$os","version":"$ver","processes":$nprocs,
 "startupMs":[$(ready_ms "$log1"),$(ready_ms "$log2"),$(ready_ms "$log3")],
 "idle":{"seconds":$idle,"cpuPercent":$idle_cpu,"rssMb":$idle_rss,"peakRssMb":$idle_peak,$idle_lag},
 "active":{"seconds":$active,"cpuPercent":$act_cpu,"rssMb":$act_rss,"peakRssMb":$act_peak,$act_lag}}
JSON
cat "perf-$os.json"
