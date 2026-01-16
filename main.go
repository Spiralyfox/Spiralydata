package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	// Si aucun argument, afficher le menu interactif
	if len(os.Args) < 2 {
		showInteractiveMenu()
		return
	}

	// Sinon, utiliser les arguments de ligne de commande (ancien mode)
	mode := os.Args[1]
	reader := bufio.NewReader(os.Stdin)

	if mode == "--host" {
		fmt.Print("Port (ex: 1234) : ")
		port, _ := reader.ReadString('\n')
		port = strings.TrimSpace(port)

		server := NewServer()
		server.Start(port)
	}

	if mode == "--user" {
		for {
			StartClient("")

			fmt.Print("\n💡 Tapez 'r' pour une nouvelle connexion ou 'x' pour quitter: ")
			choice, _ := reader.ReadString('\n')
			choice = strings.TrimSpace(strings.ToLower(choice))

			if choice == "x" {
				fmt.Println("🛑 Arrêt...")
				os.Exit(0)
			}
		}
	}
}

func showInteractiveMenu() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║            🌀 SPIRALY SYNC 🌀          ║")
	fmt.Println("║     Synchronisation de fichiers        ║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Choisissez un mode:")
	fmt.Println("1. 👤 Utilisateur (User)")
	fmt.Println("2. 🖥️  Hôte (Host)")
	fmt.Println("3. ❌ Quitter")
	fmt.Println()
	fmt.Print("Votre choix (1-3): ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		fmt.Println("\n🚀 Démarrage en mode Utilisateur...\n")
		for {
			StartClient("")

			fmt.Print("\n💡 Tapez 'r' pour une nouvelle connexion ou 'x' pour quitter: ")
			reconnect, _ := reader.ReadString('\n')
			reconnect = strings.TrimSpace(strings.ToLower(reconnect))

			if reconnect == "x" {
				fmt.Println("🛑 Arrêt...")
				os.Exit(0)
			}
		}

	case "2":
		fmt.Println("\n🚀 Démarrage en mode Hôte...\n")
		fmt.Print("Port (ex: 1234): ")
		port, _ := reader.ReadString('\n')
		port = strings.TrimSpace(port)

		server := NewServer()
		server.Start(port)

	case "3":
		fmt.Println("\n👋 Au revoir!")
		os.Exit(0)

	default:
		fmt.Println("\n❌ Choix invalide!")
		os.Exit(1)
	}
}