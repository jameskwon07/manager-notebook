# Windows 실행 파일 시험 (GitHub Actions windows-latest, pwsh).
#   pwsh scripts/smoke-windows.ps1 -Exe .\manager-notebook.exe -Expected 2.1.0
# 확인: 버전, 127.0.0.1 에서만 열림, 쓰기·읽기, 다시 켜도 기록이 남음, 두 번 실행하면 켜진 것을 씀,
#       기본 데이터 폴더(%LOCALAPPDATA%\manager-notebook), 체험 모드(메모리·예시 데이터), 화면 파일,
#       요청 접수함만(--only intake), 요청 창구(--share: 따로 연 포트에 요청 보내기 화면만).
param(
  [Parameter(Mandatory = $true)][string]$Exe,
  [string]$Expected = ""
)
$ErrorActionPreference = "Stop"
$Exe = (Resolve-Path $Exe).Path
$script:fail = 0
$work = Join-Path ([IO.Path]::GetTempPath()) ("nb-smoke-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $work | Out-Null

function Check([string]$name, [bool]$cond, [string]$detail = "") {
  if ($cond) { Write-Host "ok    $name" } else { Write-Host "FAIL  $name $detail"; $script:fail++ }
}
function Start-Nb([string[]]$argList, [string]$tag) {
  $out = Join-Path $work "$tag.out.txt"; $err = Join-Path $work "$tag.err.txt"
  return Start-Process -FilePath $Exe -ArgumentList $argList -PassThru -NoNewWindow -RedirectStandardOutput $out -RedirectStandardError $err
}
function Wait-Health([int]$port) {
  for ($i = 0; $i -lt 60; $i++) {
    try { $r = Invoke-RestMethod "http://127.0.0.1:$port/api/health" -TimeoutSec 2; if ($r.ok) { return $r } } catch {}
    Start-Sleep -Milliseconds 500
  }
  throw "port ${port} - health did not answer"
}
function Stop-Nb($p) { if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force; $p.WaitForExit(10000) | Out-Null } }
function Post-Json([string]$url, $obj) {
  $json = $obj | ConvertTo-Json -Depth 5 -Compress
  return Invoke-RestMethod -Method Post -Uri $url -ContentType "application/json; charset=utf-8" -Body ([Text.Encoding]::UTF8.GetBytes($json))
}

$data = Join-Path $work "data"
$p1 = $null; $p3 = $null; $p4 = $null; $p5 = $null; $p6 = $null
try {
  # 1) 데이터 폴더와 포트를 정해서 실행
  $p1 = Start-Nb @("--no-browser", "--port", "18080", "--data", $data) "p1"
  $h = Wait-Health 18080
  Check "health ok" ($h.ok -eq $true)
  if ($Expected) { Check "version $Expected" ($h.version -eq $Expected) "got $($h.version)" }
  $listen = Get-NetTCPConnection -LocalPort 18080 -State Listen -ErrorAction SilentlyContinue
  Check "127.0.0.1 에서만 들음" (($listen | Where-Object { $_.LocalAddress -ne "127.0.0.1" }).Count -eq 0 -and $listen.Count -ge 1) ($listen | Out-String)

  $req = Post-Json "http://127.0.0.1:18080/api/docs/requests" @{ what = "시험 요청"; status = "접수"; size = "S" }
  Check "요청 만들기" ([bool]$req.id)
  $st = Invoke-RestMethod "http://127.0.0.1:18080/api/state"
  Check "state 에 요청 1건" ($st.requests.Count -eq 1 -and $st.requests[0].what -eq "시험 요청") ($st.requests | ConvertTo-Json -Compress)
  Check "체험 모드 아님" ($st.demo -eq $false)
  Check "notebook.db 생김" (Test-Path (Join-Path $data "notebook.db"))
  $page = Invoke-WebRequest "http://127.0.0.1:18080/" -UseBasicParsing
  Check "화면 index.html" ($page.StatusCode -eq 200 -and $page.Content -match "관리 수첩")
  $core = Invoke-WebRequest "http://127.0.0.1:18080/core.js" -UseBasicParsing
  Check "core.js" ($core.StatusCode -eq 200 -and $core.Headers["Content-Type"] -match "javascript")

  # 2) 같은 포트로 한 번 더 실행하면 새로 띄우지 않고 끝남(종료 코드 0)
  $p2 = Start-Nb @("--no-browser", "--port", "18080", "--data", $data) "p2"
  $exited = $p2.WaitForExit(20000)
  Check "두 번째 실행은 바로 끝남" $exited
  if ($exited) { Check "두 번째 실행 종료 코드 0" ($p2.ExitCode -eq 0) "exit $($p2.ExitCode)" }
  $o2 = Get-Content (Join-Path $work "p2.out.txt") -Raw -Encoding UTF8
  Check "이미 켜져 있다고 알림" ($o2 -match "이미 켜져 있는") $o2

  # 3) 끄고 다시 켜도 기록이 남음
  Stop-Nb $p1; $p1 = $null
  $p3 = Start-Nb @("--no-browser", "--port", "18080", "--data", $data) "p3"
  Wait-Health 18080 | Out-Null
  $st = Invoke-RestMethod "http://127.0.0.1:18080/api/state"
  Check "다시 켜도 요청 1건" ($st.requests.Count -eq 1)
  Stop-Nb $p3; $p3 = $null

  # 4) 데이터 폴더를 안 주면 %LOCALAPPDATA%\manager-notebook
  $defDb = Join-Path $env:LOCALAPPDATA "manager-notebook\notebook.db"
  $p4 = Start-Nb @("--no-browser", "--port", "18081") "p4"
  Wait-Health 18081 | Out-Null
  Check "기본 데이터 폴더" (Test-Path $defDb) $defDb
  $o4 = Get-Content (Join-Path $work "p4.out.txt") -Raw -Encoding UTF8
  Check "안내에 주소와 데이터 위치" ($o4 -match "http://localhost:18081/" -and $o4 -match "notebook.db") $o4
  Stop-Nb $p4; $p4 = $null

  # 5) 체험 모드: 메모리, 예시 데이터, 데이터 폴더를 건드리지 않음
  $before = (Get-Item $defDb).LastWriteTimeUtc
  $p5 = Start-Nb @("--demo", "--no-browser", "--port", "18090") "p5"
  Wait-Health 18090 | Out-Null
  $st = Invoke-RestMethod "http://127.0.0.1:18090/api/state"
  Check "체험 모드 표시" ($st.demo -eq $true)
  Check "예시 프로그램 7개" ($st.programs.Count -eq 7) "$($st.programs.Count)"
  Check "예시 요청 8건" ($st.requests.Count -eq 8) "$($st.requests.Count)"
  Stop-Nb $p5; $p5 = $null
  Check "체험 모드는 기본 데이터를 건드리지 않음" ((Get-Item $defDb).LastWriteTimeUtc -eq $before)

  # 6) 요청 접수함만(--only intake) + 요청 창구(--share): 첫 화면이 요청 접수함, 창구는 따로 연 포트
  $p6 = Start-Nb @("--only", "intake", "--demo", "--share", "--share-port", "18191", "--no-browser", "--port", "18091") "p6"
  Wait-Health 18091 | Out-Null
  $home6 = Invoke-WebRequest "http://127.0.0.1:18091/" -UseBasicParsing
  Check "요청 접수함 첫 화면" ($home6.Content -match "요청 접수함")
  $st6 = Invoke-RestMethod "http://127.0.0.1:18091/api/state"
  Check "state.only = intake" ($st6.only -eq "intake")
  Check "state.ask 열림(18191)" ($st6.ask.on -eq $true -and $st6.ask.port -eq 18191) ($st6.ask | ConvertTo-Json -Compress)
  $door = Invoke-WebRequest "http://127.0.0.1:18191/" -UseBasicParsing
  Check "창구 첫 화면은 요청 보내기" ($door.Content -match "요청 보내기")
  $blocked = $true
  foreach ($path in @("/index.html", "/api/state", "/api/export")) {
    try { Invoke-WebRequest "http://127.0.0.1:18191$path" -UseBasicParsing | Out-Null; $blocked = $false } catch { if ($_.Exception.Response.StatusCode.value__ -ne 404) { $blocked = $false } }
  }
  Check "창구 포트로는 수첩 화면·API가 안 열림" $blocked
  $sent = Post-Json "http://127.0.0.1:18191/api/ask" @{ what = "창구 시험 요청"; from = "시험" }
  Check "창구로 요청 보냄" ($sent.code -match '^[a-z0-9]{10}$') ($sent | ConvertTo-Json -Compress)
  $st6 = Invoke-RestMethod "http://127.0.0.1:18091/api/state"
  Check "보낸 요청이 접수함에 들어옴" (@($st6.requests | Where-Object { $_.askCode -eq $sent.code -and $_.via -eq "창구" }).Count -eq 1)
  $wd = Post-Json "http://127.0.0.1:18191/api/ask/$($sent.code)/withdraw" @{ text = "시험 취소" }
  Check "창구에서 요청 취소(상태는 담당자 것 그대로)" ([bool]$wd.withdrawnAt -and $wd.status -eq "접수" -and $wd.canWithdraw -eq $false) ($wd | ConvertTo-Json -Compress)
  $mainL = @(Get-NetTCPConnection -LocalPort 18091 -State Listen -ErrorAction SilentlyContinue | ForEach-Object { $_.LocalAddress })
  $doorL = @(Get-NetTCPConnection -LocalPort 18191 -State Listen -ErrorAction SilentlyContinue | ForEach-Object { $_.LocalAddress })
  Check "수첩은 127.0.0.1 에서만" (($mainL.Count -gt 0) -and -not ($mainL | Where-Object { $_ -ne "127.0.0.1" })) ($mainL -join ",")
  Check "창구는 모든 주소에서" (($doorL | Where-Object { $_ -eq "0.0.0.0" -or $_ -eq "::" }).Count -gt 0) ($doorL -join ",")
  Stop-Nb $p6; $p6 = $null

  # 7) 버전 명령
  $v = & $Exe version
  if ($Expected) { Check "version 명령" ($v.Trim() -eq $Expected) $v }
}
catch {
  Write-Host "FAIL  예외: $_"
  $script:fail++
}
finally {
  Stop-Nb $p1; Stop-Nb $p3; Stop-Nb $p4; Stop-Nb $p5; Stop-Nb $p6
  Get-ChildItem $work -Filter *.txt -ErrorAction SilentlyContinue | ForEach-Object {
    if ($script:fail -gt 0) { Write-Host "---- $($_.Name)"; Get-Content $_.FullName -Encoding UTF8 | Select-Object -First 40 }
  }
}
Write-Host ""
if ($script:fail -gt 0) { Write-Host "결과: 실패 $script:fail"; exit 1 }
Write-Host "결과: 모두 통과"
exit 0
