package app

import (
	"log"
	"os"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	_ "gitlab.yurtal.tech/company/pocketbase-app-template/artifacts/migrations"
	"gitlab.yurtal.tech/company/pocketbase-app-template/internal/bot"
	"gitlab.yurtal.tech/company/pocketbase-app-template/internal/config"
	"gitlab.yurtal.tech/company/pocketbase-app-template/internal/handler"
	"gitlab.yurtal.tech/company/pocketbase-app-template/internal/hook"
	"gitlab.yurtal.tech/company/pocketbase-app-template/internal/service"
)

func NewApp(config *config.Config) *pocketbase.PocketBase {
	app := pocketbase.New()

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		logger := app.Logger()

		services := service.NewService(app)

		handlers := handler.NewHandler(logger, services, config)
		hooks := hook.New(logger, services)

		handlers.Register(e.Router)
		hooks.Register(app)

		var telegramBot *bot.Bot
		if token := os.Getenv("BOT_TOKEN"); token != "" {
			b, err := bot.NewBot(app)
			if err != nil {
				log.Printf("Failed to init Telegram bot: %v", err)
			} else {
				telegramBot = b
				go func() {
					// small delay to avoid startup race with PB internals
					time.Sleep(250 * time.Millisecond)
					log.Println("🤖 Telegram bot starting...")
					if err := telegramBot.Start(); err != nil {
						log.Printf("Telegram bot stopped with error: %v", err)
					}
				}()
			}
		} else {
			log.Println("BOT_TOKEN not set; Telegram bot will not start")
		}

		return e.Next()
	})

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: true,
	})

	return app
}
