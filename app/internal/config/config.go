package config

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	GeminiAPIKey string `env:"GEMINI_API_KEY"`
}

var instance *Config
var once sync.Once

func GetConfig() *Config {
	once.Do(func() {
		log.Print("gather config")

		instance = &Config{}

		rootPath := flag.String("root_path", "", "Root path")
		flag.Parse()

		envPaths := []string{
			*rootPath + ".env",
			filepath.Join("..", ".env"),
		}

		var envFilePath string
		for _, path := range envPaths {
			absPath, err := filepath.Abs(path)
			if err != nil {
				continue
			}
			// Check if file exists (basic check by trying to read it or stat it, but cleanenv handles read error)
			// We just want to find one that might work, or rely on cleanenv failures.
			// Let's try to read with cleanenv, if it succeeds (no error), we are good.
			// But cleanenv.ReadConfig might return error if file not found OR if content is bad.
			// Let's just pass the first one that exists technically.

			// Actually, let's just loop and try to read.
			if err := cleanenv.ReadConfig(absPath, instance); err == nil {
				envFilePath = absPath
				fmt.Printf("Loaded config from %s\n", absPath)
				break
			}
		}

		if envFilePath == "" {
			fmt.Println("Could not load .env file from paths, starting with default/env vars")
			// Try reading from env vars directly
			cleanenv.ReadEnv(instance)
		}
	})
	return instance
}
