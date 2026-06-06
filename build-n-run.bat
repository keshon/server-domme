@echo off
setlocal EnableExtensions EnableDelayedExpansion

rem Always run from the repo root (where this script lives).
cd /d "%~dp0"

set "ROOT=%CD%"
set "RCLONE_EXE=%ROOT%\dev\tools\rclone.exe"
set "RCLONE_CONF=%ROOT%\dev\rclone.conf"
set "MEDIA_DIR=%ROOT%\data\media-plain"
set "RCLONE_PID=%ROOT%\dev\rclone.pid"
set "RC_URL=http://127.0.0.1:5572"
set "BOT_EXE=%ROOT%\server-domme-discord.exe"

echo.
echo === server-domme dev launcher ===
echo.

call :ensure_rclone || exit /b 1
call :ensure_media_dir || exit /b 1
call :ensure_env_media || exit /b 1
call :start_rclone || exit /b 1
call :build_bot || (
    call :stop_rclone
    exit /b 1
)

echo.
echo === Starting bot (Ctrl+C stops bot + rclone) ===
echo.

"%BOT_EXE%"
set "BOT_EXIT=%ERRORLEVEL%"

call :stop_rclone
exit /b %BOT_EXIT%

rem ---------------------------------------------------------------------------
:ensure_rclone
if exist "%RCLONE_EXE%" exit /b 0

echo [dev] rclone not found — downloading portable build to dev\tools ...
if not exist "%ROOT%\dev\tools" mkdir "%ROOT%\dev\tools"

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$zip = Join-Path $env:TEMP 'rclone-dev.zip';" ^
  "$dest = '%ROOT:\=\\%\\dev\\tools';" ^
  "Invoke-WebRequest -Uri 'https://downloads.rclone.org/rclone-current-windows-amd64.zip' -OutFile $zip -UseBasicParsing;" ^
  "Expand-Archive -Path $zip -DestinationPath (Join-Path $env:TEMP 'rclone-dev-extract') -Force;" ^
  "$exe = Get-ChildItem -Path (Join-Path $env:TEMP 'rclone-dev-extract') -Recurse -Filter 'rclone.exe' | Select-Object -First 1;" ^
  "Copy-Item -Path $exe.FullName -Destination (Join-Path $dest 'rclone.exe') -Force"

if not exist "%RCLONE_EXE%" (
    echo [dev] ERROR: failed to download rclone. Install manually: winget install Rclone.Rclone
    exit /b 1
)
echo [dev] rclone ready: %RCLONE_EXE%
exit /b 0

rem ---------------------------------------------------------------------------
:ensure_media_dir
if not exist "%MEDIA_DIR%" (
    echo [dev] Creating media plain dir: %MEDIA_DIR%
    mkdir "%MEDIA_DIR%"
)
if not exist "%RCLONE_CONF%" (
    echo [dev] ERROR: missing %RCLONE_CONF%
    exit /b 1
)
exit /b 0

rem ---------------------------------------------------------------------------
:ensure_env_media
if not exist "%ROOT%\.env" (
    echo [dev] WARNING: .env not found — copy .env.example and set DISCORD_TOKEN
    exit /b 0
)

findstr /I /C:"MEDIA_RCLONE_RC_URL" "%ROOT%\.env" >nul 2>&1
if errorlevel 1 (
    echo [dev] Adding MEDIA_RCLONE_* entries to .env ...
    >>"%ROOT%\.env" echo.
    >>"%ROOT%\.env" echo # Media storage - rclone RC + crypt remote
    >>"%ROOT%\.env" echo MEDIA_RCLONE_RC_URL=%RC_URL%
    >>"%ROOT%\.env" echo MEDIA_RCLONE_REMOTE=crypt-media
)
exit /b 0

rem ---------------------------------------------------------------------------
:start_rclone
if exist "%RCLONE_PID%" (
    for /f "usebackq delims=" %%p in ("%RCLONE_PID%") do (
        tasklist /FI "PID eq %%p" 2>nul | find "rclone.exe" >nul && (
            echo [dev] rclone RC already running (PID %%p)
            exit /b 0
        )
    )
    del /f /q "%RCLONE_PID%" 2>nul
)

echo [dev] Starting rclone RC on %RC_URL% ...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$p = Start-Process -FilePath '%RCLONE_EXE%' -WorkingDirectory '%ROOT%' -ArgumentList @('rcd','--rc-addr','127.0.0.1:5572','--rc-no-auth','--config','%RCLONE_CONF:\=\\%') -PassThru -WindowStyle Hidden;" ^
  "$p.Id | Out-File -Encoding ascii -NoNewline '%RCLONE_PID:\=\\%';" ^
  "for ($i=0; $i -lt 30; $i++) { try { Invoke-RestMethod -Uri '%RC_URL%/core/version' -Method Post -Body '{}' -ContentType 'application/json' -TimeoutSec 2 | Out-Null; exit 0 } catch { Start-Sleep -Milliseconds 500 } };" ^
  "Write-Error 'rclone RC did not become ready'; exit 1"

if errorlevel 1 (
    echo [dev] ERROR: rclone RC failed to start. Check dev\rclone.conf and port 5572.
    exit /b 1
)
echo [dev] rclone RC ready.
exit /b 0

rem ---------------------------------------------------------------------------
:stop_rclone
if not exist "%RCLONE_PID%" exit /b 0

echo [dev] Stopping rclone RC ...
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "if (Test-Path '%RCLONE_PID:\=\\%') { $id = Get-Content '%RCLONE_PID:\=\\%'; Stop-Process -Id $id -Force -ErrorAction SilentlyContinue; Remove-Item '%RCLONE_PID:\=\\%' -Force -ErrorAction SilentlyContinue }"
exit /b 0

rem ---------------------------------------------------------------------------
:build_bot
echo.
echo === Building bot ===
echo.

for /f "tokens=3" %%i in ('go version 2^>nul') do set "GO_VERSION=%%i"
if not defined GO_VERSION set "GO_VERSION=unknown"

for /f "tokens=*" %%a in ('powershell -NoProfile -Command "Get-Date -Format o"') do set "BUILD_DATE=%%a"

go build -o "%BOT_EXE%" -ldflags "-X github.com/keshon/server-domme/internal/version.BuildDate=%BUILD_DATE% -X github.com/keshon/server-domme/internal/version.GoVersion=%GO_VERSION%" cmd\discord\main.go
if errorlevel 1 (
    echo [dev] ERROR: go build failed
    exit /b 1
)
echo [dev] Build OK: %BOT_EXE%
exit /b 0
