@echo off
setlocal enabledelayedexpansion

cd /d "%~dp0"
if not exist bin mkdir bin

set "CGO_ENABLED=0"

echo === build controller ===
call :do controller linux   amd64
call :do controller linux   arm64
call :do controller darwin  amd64
call :do controller darwin  arm64
call :do controller windows amd64 .exe

echo === build agent ===
call :do agent linux   amd64
call :do agent linux   arm64
call :do agent darwin  amd64
call :do agent darwin  arm64
call :do agent windows amd64 .exe

echo.
echo Done. Files in bin\:
dir bin /b
goto :eof

:do
set "GOOS=%2"
set "GOARCH=%3"
set "EXT=%4"
set "OUT=%~n1-%GOOS%-%GOARCH%%EXT%"
echo   -^> %GOOS%/%GOARCH%
go build -C %1 -o ..\bin\!OUT! .
if errorlevel 1 echo   FAILED
goto :eof
