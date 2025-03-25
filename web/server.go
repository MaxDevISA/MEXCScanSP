package web

import (
	"log"
	"mexc-scanner/types"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Структура для хранения данных о спреде
type SpreadData struct {
	Symbol        string  `json:"symbol"`
	BestBid       float64 `json:"best_bid"`
	BestAsk       float64 `json:"best_ask"`
	SpreadPercent float64 `json:"spread_percent"`
	AbsoluteDiff  float64 `json:"absolute_diff"`
	LastUpdate    int64   `json:"last_update"`
}

// Структура сервера
type Server struct {
	router      *gin.Engine
	spreadChan  chan []types.SpreadData
	clients     map[*websocket.Conn]bool
	spreadsData map[string]*SpreadData // Хранение актуальных спредов
	mu          sync.RWMutex
}

// Конструктор сервера
func NewServer(spreadChan chan []types.SpreadData) *Server {
	s := &Server{
		router:      gin.Default(),
		spreadChan:  spreadChan,
		clients:     make(map[*websocket.Conn]bool),
		spreadsData: make(map[string]*SpreadData),
	}

	// Запускаем очистку устаревших данных
	go s.cleanupOldData()
	return s
}

// Очистка устаревших данных (старше 5 минут)
func (s *Server) cleanupOldData() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().Unix()
		s.mu.Lock()
		for symbol, data := range s.spreadsData {
			if now-data.LastUpdate > 300 { // 5 минут
				delete(s.spreadsData, symbol)
				log.Printf("Удалены устаревшие данные для %s", symbol)
			}
		}
		s.mu.Unlock()
	}
}

// Запуск сервера
func (s *Server) Start(addr string) error {
	// Настройка маршрутов
	s.router.Static("/static", "./static")
	s.router.GET("/", s.handleIndex)
	s.router.GET("/ws", s.handleWebSocket)

	// Настройка доверенных прокси
	s.router.SetTrustedProxies([]string{"127.0.0.1", "::1"})

	// Запуск горутины для рассылки данных
	go s.broadcastSpreads()

	return s.router.Run(addr)
}

// Обработчик главной страницы
func (s *Server) handleIndex(c *gin.Context) {
	c.File("./static/index.html")
}

// Обработчик WebSocket соединений
func (s *Server) handleWebSocket(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // В продакшене здесь должна быть проверка origin
		},
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Ошибка обновления соединения: %v", err)
		return
	}

	// Проверяем, нет ли уже активного соединения с этого клиента
	s.mu.Lock()
	if _, exists := s.clients[conn]; exists {
		s.mu.Unlock()
		conn.Close()
		log.Printf("Попытка повторного подключения от клиента")
		return
	}
	s.clients[conn] = true
	s.mu.Unlock()

	// Очистка при закрытии соединения
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
		log.Printf("Клиент отключился")
	}()

	// Ожидание закрытия соединения
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Ошибка чтения: %v", err)
			}
			break
		}
	}
}

// Рассылка данных всем подключенным клиентам
func (s *Server) broadcastSpreads() {
	for spreads := range s.spreadChan {
		now := time.Now().Unix()

		// Обновляем данные
		s.mu.Lock()
		for _, spread := range spreads {
			s.spreadsData[spread.Symbol] = &SpreadData{
				Symbol:        spread.Symbol,
				BestBid:       spread.BestBid,
				BestAsk:       spread.BestAsk,
				SpreadPercent: spread.SpreadPercent,
				AbsoluteDiff:  spread.AbsoluteDiff,
				LastUpdate:    now,
			}
		}

		// Создаем отсортированный список всех актуальных спредов
		var allSpreads []SpreadData
		for _, spread := range s.spreadsData {
			allSpreads = append(allSpreads, *spread)
		}

		// Сортируем по убыванию спреда
		sort.Slice(allSpreads, func(i, j int) bool {
			return allSpreads[i].SpreadPercent > allSpreads[j].SpreadPercent
		})
		s.mu.Unlock()

		// Отправляем данные клиентам
		s.mu.RLock()
		for client := range s.clients {
			err := client.WriteJSON(allSpreads)
			if err != nil {
				log.Printf("Ошибка отправки данных: %v", err)
				client.Close()
				s.mu.RUnlock()
				s.mu.Lock()
				delete(s.clients, client)
				s.mu.Unlock()
				s.mu.RLock()
			}
		}
		s.mu.RUnlock()
	}
}
