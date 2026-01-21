package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type Config struct {
	GeminiAPIKey string `env:"GEMINI_API_KEY"`
	BotToken     string `env:"BOT_TOKEN"`
}

var instance *Config
var once sync.Once

func GetConfig() *Config {
	once.Do(func() {
		log.Print("gather config")

		instance = &Config{}

		rootPath := flag.String("root_path", "", "Root path")
		if !flag.Parsed() {
			flag.Parse()
		}

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

			// Load into os.Environ first (so other packages using os.Getenv work)
			_ = godotenv.Overload(absPath)

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

		// Ensure BOT_TOKEN is available via os.Getenv for bot startup.
		if token := instance.BotToken; token != "" {
			_ = os.Setenv("BOT_TOKEN", instance.BotToken)
		}
	})
	return instance
}
