package main

import (
	"log"
	"os"

	"github.com/hidetoshing/armarium/internal/server"
)

func main() {
	configPath := os.Getenv("ARMARIUM_CONFIG")
	if configPath == "" {
		configPath = "config.json"
	}
	app, err := server.NewFromFile(configPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("armarium listening on %s", app.Address())
	log.Fatal(app.ListenAndServe())
}
