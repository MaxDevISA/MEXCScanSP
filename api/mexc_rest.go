package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// TradingPair представляет информацию о торговой паре
type TradingPair struct {
	Symbol               string `json:"symbol"`
	Status               string `json:"status"`
	IsSpotTradingAllowed bool   `json:"isSpotTradingAllowed"`
}

// MexcREST представляет REST клиент для MEXC
type MexcREST struct {
	baseURL    string
	httpClient *http.Client
}

// NewMexcREST создает новый экземпляр MexcREST
func NewMexcREST(baseURL string) *MexcREST {
	return &MexcREST{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Second * 10,
		},
	}
}

// GetTradingPairs получает список всех доступных спот-пар
func (m *MexcREST) GetTradingPairs() ([]string, error) {
	url := fmt.Sprintf("%s/api/v3/exchangeInfo", m.baseURL)
	log.Printf("Запрос списка пар: %s", url)

	resp, err := m.httpClient.Get(url)
	if err != nil {
		log.Printf("Ошибка запроса к API: %v", err)
		return nil, fmt.Errorf("ошибка запроса к API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Ошибка чтения ответа: %v", err)
		return nil, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	log.Printf("Получен ответ от API: %s", string(body))

	var data struct {
		Symbols []TradingPair `json:"symbols"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("Ошибка парсинга JSON: %v, тело: %s", err, string(body))
		return nil, fmt.Errorf("ошибка парсинга JSON: %v, тело: %s", err, string(body))
	}

	var pairs []string
	for _, symbol := range data.Symbols {
		if symbol.Status == "1" && symbol.IsSpotTradingAllowed {
			pairs = append(pairs, symbol.Symbol)
		}
	}

	if len(pairs) == 0 {
		log.Printf("Не найдено активных торговых пар")
		return nil, fmt.Errorf("не найдено активных торговых пар")
	}

	log.Printf("Найдено %d активных торговых пар", len(pairs))
	return pairs, nil
}

// GetOrderBook получает данные стакана для конкретной пары
func (m *MexcREST) GetOrderBook(symbol string, limit int) (*OrderBook, error) {
	url := fmt.Sprintf("%s/api/v3/depth?symbol=%s&limit=%d", m.baseURL, symbol, limit)

	resp, err := m.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	var data OrderBook
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %v", err)
	}

	return &data, nil
}

// Stats24H представляет 24-часовую статистику
type Stats24H struct {
	Volume string `json:"volume"`
}

// Get24HStats получает 24-часовую статистику для пары
func (m *MexcREST) Get24HStats(symbol string) (*Stats24H, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", m.baseURL, symbol)

	resp, err := m.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	var data Stats24H
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %v", err)
	}

	return &data, nil
}
