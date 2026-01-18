@echo off
echo ========================================
echo Compilation Spiraly - Version Finale
echo ========================================
echo.

REM Nettoyer
if exist go.mod del go.mod
if exist go.sum del go.sum
if exist spiralydata.exe del spiralydata.exe

REM Init module
echo [1/3] Initialisation...
go mod init spiralydata

REM Telecharger dependances
echo [2/3] Telechargement dependances...
go get fyne.io/fyne/v2@latest
go get fyne.io/fyne/v2/app@latest
go get fyne.io/fyne/v2/canvas@latest
go get fyne.io/fyne/v2/container@latest
go get fyne.io/fyne/v2/layout@latest
go get fyne.io/fyne/v2/theme@latest
go get fyne.io/fyne/v2/widget@latest
go get github.com/fsnotify/fsnotify@latest
go get github.com/gorilla/websocket@latest
go mod tidy

REM Compiler version GUI SANS CONSOLE
echo [3/3] Compilation...
go build -v -ldflags="-H windowsgui -s -w" -o spiralydata.exe

echo.
echo ========================================
if exist spiralydata.exe (
    echo SUCCES! Executable cree: spiralydata.exe
    echo.
    echo L'application s'ouvrira SANS console Windows.
    echo Tous les logs seront dans l'interface graphique.
    echo.
) else (
    echo ERREUR: Compilation echouee
)
echo ========================================
echo.
pause