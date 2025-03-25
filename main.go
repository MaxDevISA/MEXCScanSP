package main

import (
	"flag"
	"log"
	"mexc-scanner/api"
	"mexc-scanner/web"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/viper"
)

func main() {
	// Загрузка конфигурации
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Ошибка чтения конфигурации: %v", err)
	}

	// Парсинг флагов командной строки
	minSpread := flag.Float64("min-spread", viper.GetFloat64("mexc.min_spread"), "Минимальный спред в процентах")
	flag.Parse()

	// Создание REST клиента
	restClient := api.NewMexcREST(viper.GetString("mexc.rest_url"))

	// Получение списка торговых пар
	pairs, err := restClient.GetTradingPairs()
	if err != nil {
		log.Fatalf("Ошибка получения списка пар: %v", err)
	}
	log.Printf("Получено %d торговых пар", len(pairs))

	// Создание WebSocket клиента
	wsClient, err := api.NewMexcWS(
		viper.GetString("mexc.ws_url"),
		pairs,
		*minSpread,
	)
	if err != nil {
		log.Fatalf("Ошибка создания WebSocket клиента: %v", err)
	}

	// Подписка на обновления стакана
	if err := wsClient.SubscribeToOrderBooks(); err != nil {
		log.Fatalf("Ошибка подписки на обновления: %v", err)
	}

	// Создание веб-сервера
	server := web.NewServer(wsClient.GetSpreadChan())

	// Запуск горутины для обработки WebSocket сообщений
	go wsClient.ListenAndCalculateSpread()

	// Запуск веб-сервера
	go func() {
		addr := viper.GetString("server.host") + ":" + viper.GetString("server.port")
		if err := server.Start(addr); err != nil {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	// Ожидание сигнала для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Завершение работы...")
	wsClient.Close()
}
