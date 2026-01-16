package main

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Lit un fichier avec retry pour Windows
func readFileWithRetry(path string) ([]byte, error) {
	var data []byte
	var err error

	for i := 0; i < 5; i++ {
		data, err = os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if os.IsNotExist(err) {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil, err
}

// NOUVEAU: Suppression robuste de dossier avec gestion des permissions Windows
func forceRemoveAll(path string) error {
	// Première tentative normale
	err := os.RemoveAll(path)
	if err == nil {
		return nil
	}

	// Si échec, essayer de forcer les permissions
	if runtime.GOOS == "windows" {
		// Changer récursivement les permissions
		filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Continuer même en cas d'erreur
			}
			os.Chmod(filePath, 0777)
			return nil
		})
		
		// Attendre un peu pour que Windows libère les handles
		time.Sleep(200 * time.Millisecond)
		
		// Réessayer la suppression
		err = os.RemoveAll(path)
		if err == nil {
			return nil
		}
		
		// Dernière tentative : supprimer fichier par fichier en partant du bas
		return forceRemoveRecursive(path)
	}

	// Pour Linux/Mac, réessayer avec chmod
	filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		os.Chmod(filePath, 0777)
		return nil
	})
	
	time.Sleep(100 * time.Millisecond)
	return os.RemoveAll(path)
}

// Suppression récursive forcée (dernier recours)
func forceRemoveRecursive(path string) error {
	// Lire le contenu du dossier
	entries, err := os.ReadDir(path)
	if err != nil {
		// Si on ne peut pas lire, essayer de supprimer directement
		os.Chmod(path, 0777)
		time.Sleep(50 * time.Millisecond)
		return os.Remove(path)
	}

	// Supprimer tous les enfants d'abord
	for _, entry := range entries {
		childPath := filepath.Join(path, entry.Name())
		
		if entry.IsDir() {
			// Récursion pour les sous-dossiers
			forceRemoveRecursive(childPath)
		} else {
			// Supprimer le fichier
			os.Chmod(childPath, 0777)
			time.Sleep(20 * time.Millisecond)
			os.Remove(childPath)
		}
	}

	// Enfin supprimer le dossier lui-même
	os.Chmod(path, 0777)
	time.Sleep(50 * time.Millisecond)
	
	// Plusieurs tentatives
	for i := 0; i < 3; i++ {
		err = os.Remove(path)
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	return err
}