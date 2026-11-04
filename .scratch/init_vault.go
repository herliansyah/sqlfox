package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sqlfox/internal/vault"
	_ "sqlfox/internal/driver/sqlite"
)

func main() {
	cwd, _ := os.Getwd()
	prodDb := filepath.Join(cwd, ".scratch", "production_store.db")
	stagingDb := filepath.Join(cwd, ".scratch", "staging_store.db")
	configDb := filepath.Join(cwd, ".scratch", "config.db")

	os.Remove(configDb)

	v, err := vault.OpenVault(configDb, "sqlfox")
	if err != nil {
		panic(err)
	}
	defer v.Close()

	conns := []*vault.ConnectionRecord{
		{
			Name:         "Production Store DB",
			Environment:  "Production",
			Driver:       "sqlite",
			DatabaseName: prodDb,
			Host:         "127.0.0.1",
		},
		{
			Name:         "Staging Replica DB",
			Environment:  "Staging",
			Driver:       "sqlite",
			DatabaseName: stagingDb,
			Host:         "127.0.0.1",
		},
		{
			Name:         "Dev Analytics Sandbox",
			Environment:  "Development",
			Driver:       "sqlite",
			DatabaseName: stagingDb,
			Host:         "127.0.0.1",
		},
	}

	for _, c := range conns {
		if err := v.SaveConnection(c); err != nil {
			panic(err)
		}
		fmt.Printf("Added connection: %s (%s) -> %s\n", c.Name, c.Environment, c.ID)
	}
}
