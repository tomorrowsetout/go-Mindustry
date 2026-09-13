@echo off
REM mdt-server 编译入口（cmd / PowerShell 通用）
REM 转发所有参数给 scripts\build.ps1
setlocal
set SCRIPT_DIR=%~dp0
powershell -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%build.ps1" %*
exit /b %ERRORLEVEL%
