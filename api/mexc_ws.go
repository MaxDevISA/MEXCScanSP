package api

import (
	"encoding/json"
	"fmt"
	"log"
	"mexc-scanner/types"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// OrderBook представляет данные стакана
type OrderBook struct {
	Symbol    string
	Bids      [][]string
	Asks      [][]string
	Timestamp int64
}

// MexcWS представляет WebSocket клиент для MEXC
type MexcWS struct {
	wsURL       string
	pairs       []string
	minSpread   float64
	conn        *websocket.Conn
	spreadChan  chan []types.SpreadData
	mu          sync.RWMutex
	orderBooks  map[string]*OrderBook
	spreadsData map[string]types.SpreadData
	stopChan    chan struct{}
}

// NewMexcWS создает новый экземпляр MexcWS
func NewMexcWS(wsURL string, pairs []string, minSpread float64) (*MexcWS, error) {
	log.Printf("Создание нового WebSocket клиента для %d пар", len(pairs))
	return &MexcWS{
		wsURL:       wsURL,
		pairs:       pairs,
		minSpread:   minSpread,
		spreadChan:  make(chan []types.SpreadData, 100),
		stopChan:    make(chan struct{}),
		orderBooks:  make(map[string]*OrderBook),
		spreadsData: make(map[string]types.SpreadData),
	}, nil
}

// connect устанавливает WebSocket соединение
func (m *MexcWS) connect() error {
	log.Printf("Попытка подключения к WebSocket: %s", m.wsURL)
	conn, _, err := websocket.DefaultDialer.Dial(m.wsURL, nil)
	if err != nil {
		return fmt.Errorf("ошибка подключения к WebSocket: %v", err)
	}
	log.Println("WebSocket соединение установлено успешно")
	m.conn = conn
	return nil
}

// SubscribeToOrderBooks подписывается на обновления стакана для всех пар
func (m *MexcWS) SubscribeToOrderBooks() error {
	if m.conn == nil {
		log.Println("Соединение не установлено, пытаемся подключиться")
		if err := m.connect(); err != nil {
			return err
		}
	}

	log.Printf("Начинаем подписку на %d пар", len(m.pairs))
	for _, pair := range m.pairs {
		msg := fmt.Sprintf(`{"method":"SUBSCRIPTION","params":["spot@public.limit.depth.v3.api@%s@5"]}`, pair)
		log.Printf("Отправка подписки для пары %s", pair)
		if err := m.conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return fmt.Errorf("ошибка подписки на %s: %v", pair, err)
		}
	}
	log.Println("Подписка на все пары выполнена успешно")
	return nil
}

// calculateSpread вычисляет спред для пары
func (m *MexcWS) calculateSpread(symbol string, orderBook *OrderBook) types.SpreadData {
	if len(orderBook.Bids) == 0 || len(orderBook.Asks) == 0 {
		log.Printf("Пустой стакан для пары %s", symbol)
		return types.SpreadData{}
	}

	bestBid := parseFloat(orderBook.Bids[0][0])
	bestAsk := parseFloat(orderBook.Asks[0][0])

	if bestBid <= 0 || bestAsk <= 0 {
		log.Printf("Некорректные цены для пары %s: bid=%f, ask=%f", symbol, bestBid, bestAsk)
		return types.SpreadData{}
	}

	spreadPercent := ((bestAsk - bestBid) / bestBid) * 100
	absoluteDiff := bestAsk - bestBid

	return types.SpreadData{
		Symbol:        symbol,
		BestBid:       bestBid,
		BestAsk:       bestAsk,
		SpreadPercent: spreadPercent,
		AbsoluteDiff:  absoluteDiff,
	}
}

// ListenAndCalculateSpread слушает обновления и вычисляет спреды
func (m *MexcWS) ListenAndCalculateSpread() {
	log.Println("Запуск прослушивания WebSocket обновлений")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.mu.RLock()
			var spreads []types.SpreadData
			for _, spread := range m.spreadsData {
				if spread.SpreadPercent >= m.minSpread {
					spreads = append(spreads, spread)
				}
			}
			m.mu.RUnlock()

			if len(spreads) > 0 {
				select {
				case m.spreadChan <- spreads:
					log.Printf("Отправлены данные о %d спредах", len(spreads))
				default:
					log.Printf("Канал спредов переполнен")
				}
			}

		case <-m.stopChan:
			log.Println("Получен сигнал остановки")
			return
		default:
			if m.conn == nil {
				log.Println("Соединение потеряно, пытаемся переподключиться")
				if err := m.connect(); err != nil {
					log.Printf("Ошибка переподключения: %v", err)
					time.Sleep(5 * time.Second)
					continue
				}
				if err := m.SubscribeToOrderBooks(); err != nil {
					log.Printf("Ошибка повторной подписки: %v", err)
					m.conn.Close()
					m.conn = nil
					continue
				}
			}

			_, message, err := m.conn.ReadMessage()
			if err != nil {
				log.Printf("Ошибка чтения WebSocket: %v", err)
				m.conn.Close()
				m.conn = nil
				time.Sleep(5 * time.Second)
				continue
			}

			log.Printf("Получено сообщение: %s", string(message))

			var data struct {
				C string `json:"c"` // Channel
				D struct {
					Bids []struct {
						P string `json:"p"` // Price
						V string `json:"v"` // Volume
					} `json:"bids"`
					Asks []struct {
						P string `json:"p"` // Price
						V string `json:"v"` // Volume
					} `json:"asks"`
					E string `json:"e"` // EventType
					R string `json:"r"` // Version
				} `json:"d"`
				S string `json:"s"` // Symbol
				T int64  `json:"t"` // Timestamp
			}

			if err := json.Unmarshal(message, &data); err != nil {
				log.Printf("Ошибка парсинга сообщения: %v, сообщение: %s", err, string(message))
				continue
			}

			log.Printf("Распарсенные данные: %+v", data)

			symbol := data.S
			if symbol == "" {
				log.Printf("Пустой символ в сообщении: %s", string(message))
				continue
			}

			// Преобразуем структурированные данные в формат OrderBook
			bids := make([][]string, len(data.D.Bids))
			asks := make([][]string, len(data.D.Asks))

			for i, bid := range data.D.Bids {
				bids[i] = []string{bid.P, bid.V}
			}

			for i, ask := range data.D.Asks {
				asks[i] = []string{ask.P, ask.V}
			}

			orderBook := &OrderBook{
				Symbol:    symbol,
				Bids:      bids,
				Asks:      asks,
				Timestamp: data.T,
			}

			m.mu.Lock()
			m.orderBooks[symbol] = orderBook
			m.mu.Unlock()

			log.Printf("Обновлен стакан для %s: %d bids, %d asks", symbol, len(orderBook.Bids), len(orderBook.Asks))

			spread := m.calculateSpread(symbol, orderBook)
			if spread.SpreadPercent >= m.minSpread {
				m.mu.Lock()
				m.spreadsData[symbol] = spread
				m.mu.Unlock()
			} else {
				m.mu.Lock()
				delete(m.spreadsData, symbol)
				m.mu.Unlock()
			}
		}
	}
}

// GetSpreadChan возвращает канал с данными о спредах
func (m *MexcWS) GetSpreadChan() chan []types.SpreadData {
	return m.spreadChan
}

// Close закрывает соединение
func (m *MexcWS) Close() {
	log.Println("Закрытие WebSocket соединения")
	close(m.stopChan)
	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}
}

// parseFloat преобразует строку в float64
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
