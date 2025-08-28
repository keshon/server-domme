package main

import (
	"server-domme/internal/config"
	"server-domme/internal/storage"
)

func main() {
	cfg := config.New()
	_, err := storage.New(cfg.StoragePath)
	if err != nil {
		panic(err)
	}

}
