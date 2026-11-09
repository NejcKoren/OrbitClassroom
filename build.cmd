@echo off
rem Builds Orbit Classroom on Windows. Requirements: Go 1.24+ (https://go.dev/dl/)
rem Optional for icons: go install github.com/tc-hib/go-winres@latest
setlocal
cd /d "%~dp0"
if "%VERSION%"=="" set VERSION=1.0.0
set LDF=-s -w -H windowsgui -X orbit/internal/proto.Version=%VERSION%
if not exist dist mkdir dist
go test ./... || goto :err
go build -trimpath -ldflags "%LDF%" -o dist\OrbitAgent.exe ./cmd/agent || goto :err
copy /y dist\OrbitAgent.exe cmd\master\embedded\OrbitAgent.exe >nul
go build -trimpath -ldflags "%LDF%" -o dist\OrbitMaster.exe ./cmd/master || goto :err
del cmd\master\embedded\OrbitAgent.exe
copy /y internal\master\web\docs\en.md dist\GUIDE_EN.txt >nul
copy /y internal\master\web\docs\sl.md dist\NAVODILA_SL.txt >nul
echo Done: dist\OrbitMaster.exe and dist\OrbitAgent.exe
exit /b 0
:err
echo Build failed.
exit /b 1
