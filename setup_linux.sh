#!/bin/bash

echo ""
echo "     Compilation Spiraly Sync"
echo ""
echo ""

echo "[1/2] Compilation pour Linux..."
go build -o Spiraly
if [ $? -ne 0 ]; then
    echo " Erreur de compilation!"
    exit 1
fi
chmod +x Spiraly
echo " Spiraly cree"

echo ""
echo "[2/2] Compilation pour Windows..."
GOOS=windows GOARCH=amd64 go build -o Spiraly.exe
if [ $? -ne 0 ]; then
    echo " Erreur de compilation Windows!"
    exit 1
fi
echo " Spiraly.exe cree"

echo ""
echo ""
echo " Compilation terminee avec succes! "
echo ""
echo ""
echo " Fichiers crees:"
echo "   - Spiraly (Linux/Mac)"
echo "   - Spiraly.exe (Windows)"
echo ""
echo " Lancez avec: ./Spiraly"
echo ""