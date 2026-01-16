@echo off
echo.
echo =======================================
echo      Compilation Spiraly Sync
echo =======================================
echo.

echo [1/2] Compilation pour Windows...
go build -buildvcs=false -o spiralydata.exe
if %errorlevel% neq 0 (
    echo Erreur de compilation!
    pause
    exit /b 1
)
echo spiralydata.exe cree

echo.
echo [2/2] Compilation pour Linux...
set GOOS=linux
set GOARCH=amd64
go build -buildvcs=false -o spiralydata
if %errorlevel% neq 0 (
    echo Erreur de compilation Linux!
    pause
    exit /b 1
)
echo spiralydata (Linux) cree

echo.
echo =======================================
echo Compilation terminee avec succes! 
echo =======================================
echo.
echo Fichiers crees:
echo    - spiralydata.exe (Windows)
echo    - spiralydata (Linux)
echo.
echo Double-cliquez sur spiralydata.exe pour lancer
echo.
echo L'executable peut etre place n'importe ou.
echo Le dossier 'Spiralydata' sera cree au meme
echo emplacement que l'executable.
echo.
pause