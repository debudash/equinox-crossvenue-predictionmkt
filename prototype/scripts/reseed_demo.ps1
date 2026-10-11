# Rebuild the SYNTHETIC demo database with the core's own seed script (real fetchers on fixtures ->
# matcher -> router, prices stamped now), then swap it in under the demo feed.
# SQLite cannot be written through the Docker Desktop mount while serve holds it open, so the seed
# writes inside the container, the demo feed (serve on :8081) is stopped, the file swapped, serve restarted.
$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\..\.."
Set-Location $root

$out = docker run --rm -v "${root}:/src" -v equinox-gomod:/go/pkg/mod -w /src golang:1.26 `
  sh -c "go run ./scripts/seed --config config/equinox.yaml --db /tmp/demo.db && cp /tmp/demo.db data/demo-fresh.db" 2>&1
$out | Where-Object { $_ -notmatch "^go: downloading" } | ForEach-Object { "$_" }
if (-not (Test-Path "data\demo-fresh.db")) { throw "seed produced no database" }

Get-CimInstance Win32_Process -Filter "Name='equinox.exe'" |
  Where-Object { $_.CommandLine -like "*serve*demo.yaml*" } |
  ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
Start-Sleep -Milliseconds 800
Remove-Item "data\demo.db-wal", "data\demo.db-shm" -ErrorAction SilentlyContinue
Move-Item "data\demo-fresh.db" "data\demo.db" -Force

# Started through WMI so the feed holds none of this script's handles; otherwise the caller waits on it forever.
$cmdline = "cmd /c `"`"$root\equinox.exe`" serve --addr 127.0.0.1:8081 --config config/demo.yaml > data\serve-demo.log 2> data\serve-demo.err`""
$null = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{ CommandLine = $cmdline; CurrentDirectory = "$root" }
Start-Sleep -Seconds 2
"demo feed restarted on http://127.0.0.1:8081"
