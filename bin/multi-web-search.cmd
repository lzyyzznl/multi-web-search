@echo off
rem multi-web-search Windows wrapper: auto-detect local proxy (use if found, else direct)
setlocal
set "ROOT=%~dp0"

rem Keep existing explicit proxy config
if defined HTTPS_PROXY goto :run
if defined https_proxy goto :run

rem Read system proxy from registry
for /f "tokens=3" %%a in ('reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable 2^>nul') do set "PE=%%a"
if /i not "%PE%"=="0x1" goto :run

for /f "tokens=3" %%a in ('reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyServer 2^>nul') do set "PS=%%a"
if not defined PS goto :run

rem Multi-protocol format "http=host:port;https=host:port" -> take https part, else http part
echo %PS% | findstr /c:"=" >nul
if errorlevel 1 goto :checkport
for /f "tokens=2 delims==" %%a in ('echo %PS%') do set "PS=%%a"

:checkport
rem Use proxy only if the port is actually listening
set "PORT=%PS:*:=%"
netstat -ano | findstr /r /c:":%PORT% .*LISTENING" >nul
if errorlevel 1 (
  echo multi-web-search: proxy %PS% configured but port not listening, using direct 1>&2
  goto :run
)

set "HTTPS_PROXY=http://%PS%"
set "HTTP_PROXY=http://%PS%"
echo multi-web-search: using local proxy http://%PS% 1>&2

:run
if exist "%ROOT%multi-web-search.exe" (
  "%ROOT%multi-web-search.exe" %*
  exit /b %errorlevel%
)
if defined MULTI_WEB_SEARCH_BIN (
  "%MULTI_WEB_SEARCH_BIN%" %*
  exit /b %errorlevel%
)
rem Fallback to bash launcher (git-bash environment); convert D:\x -> /d/x (lowercase drive)
set "BASH_ROOT=%ROOT:\=/%"
for %%d in (a b c d e f g h i j k l m n o p q r s t u v w x y z) do (
  if /i "%BASH_ROOT:~0,1%"=="%%d" set "BASH_ROOT=/%%d%BASH_ROOT:~1%"
)
set "BASH_ROOT=%BASH_ROOT::=%"
bash "%BASH_ROOT%multi-web-search" %*
exit /b %errorlevel%
