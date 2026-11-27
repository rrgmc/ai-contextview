@echo off
rem Finds the ai-contextview binary and runs it with the given arguments: the
rem Windows twin of the "ai-contextview" sh script beside it. Claude Code starts
rem the plugin's MCP server without Git Bash, and "sh" is often not on the PATH,
rem so on Windows it runs this file instead of the sh script. Hooks still run the
rem sh script through Git Bash. AI_CONTEXTVIEW_BIN overrides the search.
setlocal

if defined AI_CONTEXTVIEW_BIN if exist "%AI_CONTEXTVIEW_BIN%" (
	"%AI_CONTEXTVIEW_BIN%" %*
	exit /b
)
for %%I in (ai-contextview.exe) do if not "%%~$PATH:I"=="" (
	"%%~$PATH:I" %*
	exit /b
)
if defined GOBIN if exist "%GOBIN%\ai-contextview.exe" (
	"%GOBIN%\ai-contextview.exe" %*
	exit /b
)
rem GOPATH may list several folders, separated by ";": take the first.
if defined GOPATH for /f "tokens=1 delims=;" %%G in ("%GOPATH%") do if exist "%%G\bin\ai-contextview.exe" (
	"%%G\bin\ai-contextview.exe" %*
	exit /b
)
if exist "%USERPROFILE%\go\bin\ai-contextview.exe" (
	"%USERPROFILE%\go\bin\ai-contextview.exe" %*
	exit /b
)
for /f "delims=" %%G in ('go env GOPATH 2^>nul') do for /f "tokens=1 delims=;" %%H in ("%%G") do if exist "%%H\bin\ai-contextview.exe" (
	"%%H\bin\ai-contextview.exe" %*
	exit /b
)
echo ai-contextview: binary not found. Install it as https://github.com/rrgmc/ai-contextview#install describes (or set AI_CONTEXTVIEW_BIN) 1>&2
exit /b 1
