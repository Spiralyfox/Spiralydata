@echo off
echo Compilation Spiraly Sync

echo [1/2] Compilation pour Windows...
go build -o Spiraly.exe
if %errorlevel% neq 0 (
    echo Erreur de compilation!
    pause
    exit /b 1
)
echo Spiraly.exe cree

echo.
echo [2/2] Compilation pour Linux...
set GOOS=linux
set GOARCH=amd64
go build -o Spiraly
if %errorlevel% neq 0 (
    echo Erreur de compilation Linux!
    pause
    exit /b 1
)
echo Spiraly (Linux) cree

echo.
echo  Compilation terminee avec succes! 
echo.
echo  Fichiers crees:
echo    - Spiraly.exe (Windows)
echo    - Spiraly (Linux)
echo.
echo  Double-cliquez sur Spiraly.exe pour lancer
echo.
pause