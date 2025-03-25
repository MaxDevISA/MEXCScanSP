package main

import (
	"flag"
	"fmt"
	"log"
	"mexc-scanner/api"
	"mexc-scanner/web"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/viper"
)

type SpreadData struct {
	Symbol        string  `json:"Symbol"`
	BestBid       float64 `json:"BestBid"`
	BestAsk       float64 `json:"BestAsk"`
	SpreadPercent float64 `json:"SpreadPercent"`
	AbsoluteDiff  float64 `json:"AbsoluteDiff"`
	Volume24h     float64 `json:"Volume24h"`
	LastUpdate    string  `json:"LastUpdate"`
}

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

func get24hVolume(client *api.MexcREST, symbol string) (float64, error) {
	stats, err := client.Get24HStats(symbol)
	if err != nil {
		return 0, fmt.Errorf("ошибка при получении 24h статистики: %v", err)
	}

	volume, err := strconv.ParseFloat(stats.Volume, 64)
	if err != nil {
		return 0, fmt.Errorf("ошибка при конвертации объема '%s' в число: %v", stats.Volume, err)
	}

	return volume, nil
}
