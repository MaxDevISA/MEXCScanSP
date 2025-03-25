package types

// SpreadData представляет данные о спреде
type SpreadData struct {
	Symbol        string  `json:"Symbol"`
	BestBid       float64 `json:"BestBid"`
	BestAsk       float64 `json:"BestAsk"`
	SpreadPercent float64 `json:"SpreadPercent"`
	AbsoluteDiff  float64 `json:"AbsoluteDiff"`
	Volume24h     float64 `json:"Volume24h"`
	LastUpdate    string  `json:"LastUpdate"`
}
