package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type Config struct {
	ServerAddr string `json:"server_addr"`
	HostID     string `json:"host_id"`
}

const configFile = "spiraly_config.json"

func ConfigExists() bool {
	_, err := os.Stat(configFile)
	return err == nil
}

func SaveConfig(addr, hostID string) error {
	config := Config{
		ServerAddr: addr,
		HostID:     hostID,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(configFile, data, 0644)
}

func LoadConfig() (*Config, error) {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func DeleteConfig() error {
	return os.Remove(configFile)
}

func UpdateHostID(newHostID string) error {
	config, err := LoadConfig()
	if err != nil {
		return err
	}

	config.HostID = newHostID
	return SaveConfig(config.ServerAddr, config.HostID)
}

func ShowConfigMenu() (string, string, bool) {
	for {
		fmt.Println("\n╔════════════════════════════════════════╗")
		fmt.Println("║        CONFIGURATION SPIRALY           ║")
		fmt.Println("╚════════════════════════════════════════╝")
		fmt.Println("1. 📂 Charger la configuration existante")
		fmt.Println("2. ➕ Créer une nouvelle configuration")
		fmt.Println("3. 🔑 Changer l'ID hôte de la configuration")
		fmt.Println("4. 🗑️  Supprimer la configuration")
		fmt.Println("5. 🚀 Connexion sans configuration")
		fmt.Print("\nChoix (1-5): ")

		var choice string
		fmt.Scanln(&choice)

		switch choice {
		case "1":
			config, err := LoadConfig()
			if err != nil {
				fmt.Println("❌ Erreur de chargement:", err)
				continue
			}
			fmt.Println("\n✅ Configuration chargée:")
			fmt.Println("   📡 Serveur:", config.ServerAddr)
			fmt.Println("   🔑 Host ID:", config.HostID)
			return config.ServerAddr, config.HostID, false

		case "2":
			fmt.Print("\n📡 Adresse serveur (IP:PORT): ")
			var addr string
			fmt.Scanln(&addr)

			fmt.Print("🔑 ID du host: ")
			var hostID string
			fmt.Scanln(&hostID)

			if err := SaveConfig(addr, hostID); err != nil {
				fmt.Println("❌ Erreur de sauvegarde:", err)
				continue
			}

			fmt.Println("✅ Configuration sauvegardée!")
			return addr, hostID, false

		case "3":
			fmt.Print("\n🔑 Nouvel ID du host: ")
			var newHostID string
			fmt.Scanln(&newHostID)

			if err := UpdateHostID(newHostID); err != nil {
				fmt.Println("❌ Erreur de mise à jour:", err)
				continue
			}

			fmt.Println("✅ ID du host mis à jour!")
			config, _ := LoadConfig()
			return config.ServerAddr, config.HostID, false

		case "4":
			fmt.Print("\n⚠️  Êtes-vous sûr de vouloir supprimer la configuration? (y/n): ")
			var confirm string
			fmt.Scanln(&confirm)

			if confirm == "y" || confirm == "Y" {
				if err := DeleteConfig(); err != nil {
					fmt.Println("❌ Erreur de suppression:", err)
				} else {
					fmt.Println("✅ Configuration supprimée!")
				}
			}
			continue

		case "5":
			return "", "", true

		default:
			fmt.Println("❌ Choix invalide!")
			continue
		}
	}
}