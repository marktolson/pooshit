@echo off
echo Building Pooshit...
go build -o pooshit.exe
if %errorlevel% neq 0 (
    echo Build failed!
    exit /b %errorlevel%
)
echo Build successful! Run with: pooshit.exe
