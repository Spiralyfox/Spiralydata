#!/bin/bash

echo ""
echo "======================================="
echo "     Compilation Spiraly Sync"
echo "======================================="
echo ""

echo "[1/2] Compilation pour Linux..."
go build -buildvcs=false -o spiralydata
if [ $? -ne 0 ]; then
    echo "Erreur de compilation!"
    exit 1
fi
chmod +x spiralydata
echo "spiralydata cree"

echo ""
echo "[2/2] Compilation pour Windows..."
GOOS=windows GOARCH=amd64 go build -buildvcs=false -o spiralydata.exe
if [ $? -ne 0 ]; then
    echo "Erreur de compilation Windows!"
    exit 1
fi
echo "spiralydata.exe cree"

echo ""
echo "======================================="
echo "Compilation terminee avec succes! "
echo "======================================="
echo ""
echo "Fichiers crees:"
echo "   - spiralydata (Linux/Mac)"
echo "   - spiralydata.exe (Windows)"
echo ""
echo "Lancez avec: ./spiralydata"
echo ""
echo "L'executable peut etre place n'importe ou."
echo "Le dossier 'Spiralydata' sera cree au meme"
echo "emplacement que l'executable."
echo ""