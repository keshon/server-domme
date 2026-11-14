@echo off
call "%~dp0build.cmd" test %*
exit /b %errorlevel%
