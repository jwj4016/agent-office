# Windows version of scripts/perf.sh (same scenario and report).
#
#   pwsh scripts/perf.ps1 <agent-office.exe> [active-seconds] [idle-seconds]
#
# CPU is the change in total processor time of the app and its WebView2
# processes (its descendants); memory is the sum of their working sets.
param([Parameter(Mandatory)][string]$Exe, [int]$Active = 60, [int]$Idle = 30)
$ErrorActionPreference = 'Stop'
$work = Join-Path ([IO.Path]::GetTempPath()) ("perf-" + [guid]::NewGuid())
$data = Join-Path $work '성능 데이터'
New-Item -ItemType Directory $work | Out-Null
Start-Process $Exe -ArgumentList '--perf-seed', '-data-dir', "`"$data`"" -Wait

function Tree([int]$root) {
    $all = Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId
    $ids = @($root); $i = 0
    while ($i -lt $ids.Count) { $ids += @($all | Where-Object ParentProcessId -eq $ids[$i] | ForEach-Object ProcessId); $i++ }
    Get-Process -Id $ids -ErrorAction SilentlyContinue
}
function Launch([string]$log, [string]$runs) {
    $env:AGENT_OFFICE_PERF = $log; $env:AGENT_OFFICE_PERF_RUNS = $runs; $env:AGENT_OFFICE_DATA_DIR = $data
    $p = Start-Process $Exe -PassThru
    for ($i = 0; $i -lt 300 -and -not ((Test-Path $log) -and (Select-String -Quiet '"ui-ready"' $log)); $i++) { Start-Sleep -Milliseconds 200 }
    if (-not (Select-String -Quiet '"ui-ready"' $log)) { throw 'first screen never reported' }
    Start-Sleep 2
    $p
}
function Stop($p) { Tree $p.Id | Stop-Process -Force -ErrorAction SilentlyContinue; Start-Sleep 2 }
function Window($p, [int]$sec) {
    $c0 = (Tree $p.Id | Measure-Object -Sum { $_.TotalProcessorTime.TotalSeconds }).Sum
    $peak = 0
    for ($i = 0; $i -lt $sec; $i++) {
        Start-Sleep 1
        $peak = [math]::Max($peak, (Tree $p.Id | Measure-Object -Sum WorkingSet64).Sum / 1MB)
    }
    $procs = Tree $p.Id
    $c1 = ($procs | Measure-Object -Sum { $_.TotalProcessorTime.TotalSeconds }).Sum
    [ordered]@{ seconds = $sec; processes = $procs.Count; cpuPercent = [math]::Round(($c1 - $c0) * 100 / $sec, 1)
        workingSetMb = [math]::Round(($procs | Measure-Object -Sum WorkingSet64).Sum / 1MB); peakWorkingSetMb = [math]::Round($peak) }
}
function Marks([string]$log) { Get-Content $log | ForEach-Object { $_ | ConvertFrom-Json } }
function Lag($w, [string]$log) {
    $lag = @(Marks $log | Where-Object name -eq 'lag')
    $w.lagP95Ms = ($lag | ForEach-Object { $_.data.p95 } | Measure-Object -Maximum).Maximum
    $w.lagMaxMs = ($lag | ForEach-Object { $_.data.max } | Measure-Object -Maximum).Maximum
    $w.hiddenReports = "$(@($lag | Where-Object { $_.data.hidden }).Count)/$($lag.Count)"
    $w
}
function Ready([string]$log) { (Marks $log | Where-Object name -eq 'ui-ready' | Select-Object -First 1).ms }

$l1 = Join-Path $work 'idle.jsonl'; $p = Launch $l1 'off'; Start-Sleep 8; $idleW = Lag (Window $p $Idle) $l1; Stop $p
$l2 = Join-Path $work 'active.jsonl'; $p = Launch $l2 'on'; $actW = Lag (Window $p $Active) $l2; Stop $p
$l3 = Join-Path $work 'third.jsonl'; $p = Launch $l3 'off'; Stop $p
$ver = (Marks $l1 | Where-Object name -eq 'startup' | Select-Object -First 1).data.version
$res = [ordered]@{ os = "windows-$env:PROCESSOR_ARCHITECTURE".ToLower(); version = $ver; startupMs = @((Ready $l1), (Ready $l2), (Ready $l3)); idle = $idleW; active = $actW }
$json = $res | ConvertTo-Json -Depth 4
$json | Set-Content -Encoding utf8 "perf-windows.json"
$json
Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
