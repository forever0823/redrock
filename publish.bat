@echo off
setlocal enabledelayedexpansion

cd /d "%~dp0"

if not exist bin\* (
    echo [ERROR] bin\ is empty, run build.bat first
    pause
    exit /b 1
)

set "PUBLISH=publish"
if exist "%PUBLISH%" rd /s /q "%PUBLISH%"

echo === Generating deploy packages ===

call :pkg controller linux   amd64  ""
call :pkg controller linux   arm64  ""
call :pkg controller darwin  amd64  ""
call :pkg controller darwin  arm64  ""
call :pkg controller windows amd64  .exe

call :pkg agent linux   amd64  ""
call :pkg agent linux   arm64  ""
call :pkg agent darwin  amd64  ""
call :pkg agent darwin  arm64  ""
call :pkg agent windows amd64  .exe

echo.
echo Done: %PUBLISH%\
dir %PUBLISH% /b
goto :eof

:pkg
set "APP=%~1"
set "GOOS=%~2"
set "GOARCH=%~3"
set "EXT=%~4"
set "DIR=%PUBLISH%\%APP%-%GOOS%-%GOARCH%"
set "BIN=bin\%APP%-%GOOS%-%GOARCH%%EXT%"

if not exist "%BIN%" (
    echo   [SKIP] %BIN%
    goto :eof
)

mkdir "%DIR%" >nul 2>&1
copy /y "%BIN%" "%DIR%\"      >nul 2>&1
copy /y config.env "%DIR%\"   >nul 2>&1

if "%~1"=="controller" (
    copy /y dashboard.html "%DIR%\" >nul 2>&1
    copy /y login.html "%DIR%\"      >nul 2>&1
    echo   [OK] %DIR%  ^(binary + dashboard + login + config^)
) else (
    echo   [OK] %DIR%  ^(binary + config^)
)
goto :eof
