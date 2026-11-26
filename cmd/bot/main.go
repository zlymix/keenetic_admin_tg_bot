package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"keenetic_admin/internal/bot"
	"keenetic_admin/internal/config"
	"keenetic_admin/internal/keenetic"
	"keenetic_admin/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func main() {
	defaultConfig := "data/config.yaml"
	if _, err := os.Stat("data/config.yaml"); os.IsNotExist(err) {
		if _, err := os.Stat("config.yaml"); err == nil {
			defaultConfig = "config.yaml"
		}
	}

	configPath := flag.String("config", defaultConfig, "Path to config file")
	flag.Parse()

	log.Println("[MAIN] Starting Keenetic Admin Bot...")

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("[MAIN] Failed to load configuration from %s: %v", *configPath, err)
	}

	if cfg.Telegram.BotToken == "" || cfg.Telegram.BotToken == "YOUR_TELEGRAM_BOT_TOKEN_HERE" {
		log.Fatalf("[MAIN] Invalid or missing telegram.bot_token in %s. Please specify your Telegram Bot Token.", *configPath)
	}

	// 1. Initialize Keenetic HTTP Client
	keeneticClient := keenetic.NewClient(cfg.Router.URL, cfg.Router.Login, cfg.Router.Password)

	// Test login with router
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("[MAIN] Connecting to Keenetic router at %s (user: %s)...", cfg.Router.URL, cfg.Router.Login)
	if err := keeneticClient.Login(ctx); err != nil {
		log.Fatalf("[MAIN] Failed to connect to Keenetic router: %v", err)
	}
	log.Println("[MAIN] Successfully authenticated with Keenetic router.")

	// 2. Initialize Telegram Bot
	pref := tele.Settings{
		Token:  cfg.Telegram.BotToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	teleBot, err := tele.NewBot(pref)
	if err != nil {
		log.Fatalf("[MAIN] Failed to start Telegram bot: %v", err)
	}

	// 3. Register Auth Middleware
	teleBot.Use(bot.AuthMiddleware(cfg.Telegram.AllowedUsers))

	// 4. Initialize & Start Device Connect/Disconnect Monitor
	pollInterval := time.Duration(cfg.Router.PollIntervalSeconds) * time.Second
	deviceMonitor := bot.NewDeviceMonitor(keeneticClient, teleBot, cfg.Telegram.NotificationChatID, pollInterval)

	monitorCtx, stopMonitor := context.WithCancel(context.Background())
	defer stopMonitor()
	deviceMonitor.Start(monitorCtx)

	// 5. Initialize Favorites Storage
	favoritesStore, err := storage.NewFavoritesStorage(cfg.Storage.FavoritesFile)
	if err != nil {
		log.Fatalf("[MAIN] Failed to initialize favorites storage: %v", err)
	}
	log.Printf("[MAIN] Favorites storage initialized from %s", cfg.Storage.FavoritesFile)

	// 6. Setup Bot Handlers
	appBot := bot.New(teleBot, keeneticClient, deviceMonitor, favoritesStore)
	appBot.SetupRoutes()

	// 7. Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("[MAIN] Received termination signal. Shutting down...")
		stopMonitor()
		teleBot.Stop()
	}()

	log.Printf("[MAIN] Bot @%s is running. Press Ctrl+C to stop.", teleBot.Me.Username)
	teleBot.Start()
}
