package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// casino_stats.go -- cleaned casino statistics built from GameHistoryEntry

type CasinoStats struct {
	GeneratedAt string               `json:"generatedAt"`
	Range       StatsRange           `json:"range"`
	Overall     CasinoStatsSummary   `json:"overall"`
	ByGame      map[string]GameStats `json:"byGame"`
	ByItem      []ItemStats          `json:"byItem"`
	ByPlayer    []PlayerStats        `json:"byPlayer"`
	Issues      IssueStats           `json:"issues"`
	Trends      TrendStats           `json:"trends"`
}

type StatsRange struct {
	Key     string `json:"key"`
	StartAt string `json:"startAt,omitempty"`
	EndAt   string `json:"endAt,omitempty"`
}

type CasinoStatsSummary struct {
	TotalRounds           int            `json:"totalRounds"`
	CompletedRounds       int            `json:"completedRounds"`
	IssueRounds           int            `json:"issueRounds"`
	PlayerWins            int            `json:"playerWins"`
	DealerWins            int            `json:"dealerWins"`
	PlayerWinRate         float64        `json:"playerWinRate"`
	DealerWinRate         float64        `json:"dealerWinRate"`
	IssueRate             float64        `json:"issueRate"`
	TotalBetItemsIn       int            `json:"totalBetItemsIn"`
	TotalPayoutItemsOut   int            `json:"totalPayoutItemsOut"`
	NetItems              int            `json:"netItems"`
	RTPPercent            float64        `json:"rtpPercent"`
	ProfitMarginPercent   float64        `json:"profitMarginPercent"`
	CasinoEdgePercent     float64        `json:"casinoEdgePercent"`
	AverageNetPerRound    float64        `json:"averageNetPerRound"`
	UniquePlayers         int            `json:"uniquePlayers"`
	BestGameByNet         string         `json:"bestGameByNet"`
	WorstGameByNet        string         `json:"worstGameByNet"`
	BestItemByNet         string         `json:"bestItemByNet"`
	WorstItemByNet        string         `json:"worstItemByNet"`
	MostProfitablePlayer  string         `json:"mostProfitablePlayer"`
	LeastProfitablePlayer string         `json:"leastProfitablePlayer"`
	BestSingleWin         int            `json:"bestSingleWin"`
	WorstSingleLoss       int            `json:"worstSingleLoss"`
	BetItemCounts         map[string]int `json:"betItemCounts"`
	PayoutItemCounts      map[string]int `json:"payoutItemCounts"`
	NetItemCounts         map[string]int `json:"netItemCounts"`
}

type GameStats struct {
	Game                string         `json:"game"`
	TotalRounds         int            `json:"totalRounds"`
	CompletedRounds     int            `json:"completedRounds"`
	IssueRounds         int            `json:"issueRounds"`
	PlayerWins          int            `json:"playerWins"`
	DealerWins          int            `json:"dealerWins"`
	PlayerWinRate       float64        `json:"playerWinRate"`
	DealerWinRate       float64        `json:"dealerWinRate"`
	TotalBetItemsIn     int            `json:"totalBetItemsIn"`
	TotalPayoutItemsOut int            `json:"totalPayoutItemsOut"`
	NetItems            int            `json:"netItems"`
	RTPPercent          float64        `json:"rtpPercent"`
	CasinoEdgePercent   float64        `json:"casinoEdgePercent"`
	AverageNetPerRound  float64        `json:"averageNetPerRound"`
	LargestBet          int            `json:"largestBet"`
	LargestPayout       int            `json:"largestPayout"`
	WorstCasinoLoss     int            `json:"worstCasinoLoss"`
	BestCasinoWin       int            `json:"bestCasinoWin"`
	BetItemCounts       map[string]int `json:"betItemCounts"`
	PayoutItemCounts    map[string]int `json:"payoutItemCounts"`
	NetItemCounts       map[string]int `json:"netItemCounts"`
}

type ItemStats struct {
	Name            string         `json:"name"`
	BetIn           int            `json:"betIn"`
	PayoutOut       int            `json:"payoutOut"`
	Net             int            `json:"net"`
	GamesPlayed     int            `json:"gamesPlayed"`
	CasinoWins      int            `json:"casinoWins"`
	CasinoLosses    int            `json:"casinoLosses"`
	CasinoWinRate   float64        `json:"casinoWinRate"`
	Status          string         `json:"status"`
	ByGameBetIn     map[string]int `json:"byGameBetIn"`
	ByGamePayoutOut map[string]int `json:"byGamePayoutOut"`
	ByGameNet       map[string]int `json:"byGameNet"`
}

type PlayerStats struct {
	PlayerName        string             `json:"playerName"`
	TotalRounds       int                `json:"totalRounds"`
	PlayerWins        int                `json:"playerWins"`
	DealerWins        int                `json:"dealerWins"`
	PlayerWinRate     float64            `json:"playerWinRate"`
	BetItemsIn        int                `json:"betItemsIn"`
	PayoutItemsOut    int                `json:"payoutItemsOut"`
	NetAgainstCasino  int                `json:"netAgainstCasino"`
	AverageNetPerGame float64            `json:"averageNetPerGame"`
	IsProfitable      bool               `json:"isProfitable"`
	ByGameRounds      map[string]int     `json:"byGameRounds"`
	ByGameWins        map[string]int     `json:"byGameWins"`
	ByGameLosses      map[string]int     `json:"byGameLosses"`
	ByGameWinRate     map[string]float64 `json:"byGameWinRate"`
}

type DailyStatsPoint struct {
	Day            string  `json:"day"`
	TotalRounds    int     `json:"totalRounds"`
	NetItems       int     `json:"netItems"`
	IssueRounds    int     `json:"issueRounds"`
	BetItemsIn     int     `json:"betItemsIn"`
	PayoutItemsOut int     `json:"payoutItemsOut"`
	RTPPercent     float64 `json:"rtpPercent"`
}

type TrendStats struct {
	Daily []DailyStatsPoint `json:"daily"`
}

type IssueStats struct {
	TotalIssues          int            `json:"totalIssues"`
	ByReason             map[string]int `json:"byReason"`
	GameChoiceTimeouts   int            `json:"gameChoiceTimeouts"`
	PayoutTimeouts       int            `json:"payoutTimeouts"`
	PayoutCancelFlags    int            `json:"payoutCancelFlags"`
	TradeConfirmTimeouts int            `json:"tradeConfirmTimeouts"`
}

func totalTradeItemQuantity(items []TradeItem) int {
	total := 0
	for _, it := range items {
		total += it.Quantity
	}
	return total
}

func tradeItemsToCountMap(items []TradeItem) map[string]int {
	m := map[string]int{}
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		m[name] += it.Quantity
	}
	return m
}

func normalizeGameName(game string) string {
	g := strings.TrimSpace(strings.ToLower(game))
	switch g {
	case "poker":
		return "Poker"
	case "21", "blackjack", "black jack":
		return "21"
	case "13", "thirteen":
		return "13"
	case "tri", "tri (high/low)", "tri (high)", "tri (low)":
		return "Tri"
	default:
		return ""
	}
}

func isFinanciallyCountableRound(entry GameHistoryEntry) bool {
	if totalTradeItemQuantity(entry.BetItems) == 0 {
		return false
	}
	if normalizeGameName(entry.Game) == "" {
		return false
	}
	if entry.Issue {
		return false
	}
	if entry.CompletedAt != "" || strings.TrimSpace(entry.Winner) != "" || strings.ToLower(strings.TrimSpace(entry.Status)) == "completed" {
		return true
	}
	return false
}

func didPlayerWin(entry GameHistoryEntry) bool {
	if strings.TrimSpace(entry.Winner) == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(entry.Winner), strings.TrimSpace(entry.PlayerName))
}

func didDealerWin(entry GameHistoryEntry) bool {
	return strings.EqualFold(strings.TrimSpace(entry.Winner), "dealer")
}

func parseEntryTime(entry GameHistoryEntry) time.Time {
	var s string
	if entry.CompletedAt != "" {
		s = entry.CompletedAt
	} else if entry.UpdatedAt != "" {
		s = entry.UpdatedAt
	} else if entry.StartedAt != "" {
		s = entry.StartedAt
	}
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itemStatusFromNet(net int) string {
	if net > 0 {
		return "Up"
	}
	if net < 0 {
		return "Down"
	}
	return "Even"
}

func unionItemNames(a map[string]int, b map[string]int) []string {
	namesMap := map[string]struct{}{}
	for k := range a {
		namesMap[k] = struct{}{}
	}
	for k := range b {
		namesMap[k] = struct{}{}
	}
	names := make([]string, 0, len(namesMap))
	for k := range namesMap {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// BuildCasinoStats computes statistics for the provided time range key.
// Supported rangeKey: all_time, today, last_7_days, last_30_days
func (a *App) BuildCasinoStats(rangeKey string) CasinoStats {
	a.gameHistoryMu.Lock()
	entries := make([]GameHistoryEntry, len(a.gameHistory))
	copy(entries, a.gameHistory)
	a.gameHistoryMu.Unlock()

	now := time.Now()
	var start time.Time
	end := now
	switch rangeKey {
	case "today":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "last_7_days":
		start = now.Add(-7 * 24 * time.Hour)
	case "last_30_days":
		start = now.Add(-30 * 24 * time.Hour)
	case "all_time":
		start = time.Time{}
	default:
		start = time.Time{}
	}

	overall := CasinoStatsSummary{
		BetItemCounts:    map[string]int{},
		PayoutItemCounts: map[string]int{},
		NetItemCounts:    map[string]int{},
	}

	byGame := map[string]*GameStats{}
	games := []string{"Poker", "21", "13", "Tri"}
	for _, g := range games {
		byGame[g] = &GameStats{
			Game:             g,
			BetItemCounts:    map[string]int{},
			PayoutItemCounts: map[string]int{},
			NetItemCounts:    map[string]int{},
		}
	}

	itemsMap := map[string]*ItemStats{}
	playersMap := map[string]*PlayerStats{}
	issues := IssueStats{ByReason: map[string]int{}}
	dailyPoints := map[string]*DailyStatsPoint{}
	earliestEntryTime := time.Time{}
	hasSingle := false
	bestSingleWin := 0
	worstSingleLoss := 0
	financiallyCountableRounds := 0

	sort.Slice(entries, func(i, j int) bool {
		ti := parseEntryTime(entries[i])
		tj := parseEntryTime(entries[j])
		if ti.IsZero() && tj.IsZero() {
			return i < j
		}
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.Before(tj)
	})

	for _, entry := range entries {
		t := parseEntryTime(entry)
		if !start.IsZero() {
			if t.IsZero() || t.Before(start) || t.After(end) {
				continue
			}
		} else if !t.IsZero() && t.After(end) {
			continue
		}

		if !t.IsZero() && (earliestEntryTime.IsZero() || t.Before(earliestEntryTime)) {
			earliestEntryTime = t
		}

		dayKey := "unknown"
		if !t.IsZero() {
			dayKey = t.Format("2006-01-02")
		}
		dp := dailyPoints[dayKey]
		if dp == nil {
			dp = &DailyStatsPoint{Day: dayKey}
			dailyPoints[dayKey] = dp
		}
		dp.TotalRounds++
		overall.TotalRounds++

		if entry.CompletedAt != "" {
			overall.CompletedRounds++
		}

		if entry.Issue {
			overall.IssueRounds++
			dp.IssueRounds++
			issues.TotalIssues++
			issues.ByReason[entry.IssueReason]++
			lower := strings.ToLower(entry.IssueReason)
			if strings.Contains(lower, "choice") || (strings.Contains(lower, "timeout") && strings.Contains(lower, "choice")) {
				issues.GameChoiceTimeouts++
			}
			if strings.Contains(lower, "payout") || strings.Contains(lower, "underfund") {
				issues.PayoutTimeouts++
			}
			if strings.Contains(lower, "cancel") {
				issues.PayoutCancelFlags++
			}
			if strings.Contains(lower, "confirm") {
				issues.TradeConfirmTimeouts++
			}
		}

		gname := normalizeGameName(entry.Game)
		if gname != "" {
			gs := byGame[gname]
			gs.TotalRounds++
			if entry.CompletedAt != "" {
				gs.CompletedRounds++
			}
			if entry.Issue {
				gs.IssueRounds++
			}
		}

		if !entry.Issue && (entry.CompletedAt != "" || strings.TrimSpace(entry.Winner) != "") {
			if didPlayerWin(entry) {
				overall.PlayerWins++
				if gname != "" {
					byGame[gname].PlayerWins++
				}
			} else if didDealerWin(entry) {
				overall.DealerWins++
				if gname != "" {
					byGame[gname].DealerWins++
				}
			}
		}

		if !isFinanciallyCountableRound(entry) {
			continue
		}

		financiallyCountableRounds++

		betMap := tradeItemsToCountMap(entry.BetItems)
		payoutMap := tradeItemsToCountMap(entry.PayoutItems)
		betTotal := totalTradeItemQuantity(entry.BetItems)
		payoutTotal := totalTradeItemQuantity(entry.PayoutItems)
		net := betTotal - payoutTotal

		overall.TotalBetItemsIn += betTotal
		overall.TotalPayoutItemsOut += payoutTotal
		overall.NetItems += net

		dp.BetItemsIn += betTotal
		dp.PayoutItemsOut += payoutTotal
		dp.NetItems += net

		if !hasSingle {
			bestSingleWin = net
			worstSingleLoss = net
			hasSingle = true
		}
		if net > bestSingleWin {
			bestSingleWin = net
		}
		if net < worstSingleLoss {
			worstSingleLoss = net
		}

		for name, qty := range betMap {
			overall.BetItemCounts[name] += qty
			item := itemsMap[name]
			if item == nil {
				item = &ItemStats{
					Name:            name,
					ByGameBetIn:     map[string]int{},
					ByGamePayoutOut: map[string]int{},
					ByGameNet:       map[string]int{},
				}
				itemsMap[name] = item
			}
			item.BetIn += qty
			if gname != "" {
				item.ByGameBetIn[gname] += qty
			}
		}

		for name, qty := range payoutMap {
			overall.PayoutItemCounts[name] += qty
			item := itemsMap[name]
			if item == nil {
				item = &ItemStats{
					Name:            name,
					ByGameBetIn:     map[string]int{},
					ByGamePayoutOut: map[string]int{},
					ByGameNet:       map[string]int{},
				}
				itemsMap[name] = item
			}
			item.PayoutOut += qty
			if gname != "" {
				item.ByGamePayoutOut[gname] += qty
			}
		}

		for _, itemName := range unionItemNames(betMap, payoutMap) {
			item := itemsMap[itemName]
			if item == nil {
				continue
			}
			itemNet := betMap[itemName] - payoutMap[itemName]
			item.GamesPlayed++
			if itemNet > 0 {
				item.CasinoWins++
			} else if itemNet < 0 {
				item.CasinoLosses++
			}
		}

		if gname != "" {
			gs := byGame[gname]
			gs.TotalBetItemsIn += betTotal
			gs.TotalPayoutItemsOut += payoutTotal
			gs.NetItems += net
			if betTotal > gs.LargestBet {
				gs.LargestBet = betTotal
			}
			if payoutTotal > gs.LargestPayout {
				gs.LargestPayout = payoutTotal
			}
			if net > gs.BestCasinoWin {
				gs.BestCasinoWin = net
			}
			if net < gs.WorstCasinoLoss || gs.WorstCasinoLoss == 0 {
				gs.WorstCasinoLoss = net
			}
			for name, qty := range betMap {
				gs.BetItemCounts[name] += qty
				gs.NetItemCounts[name] += qty
			}
			for name, qty := range payoutMap {
				gs.PayoutItemCounts[name] += qty
				gs.NetItemCounts[name] -= qty
			}
		}

		pname := strings.TrimSpace(entry.PlayerName)
		if pname == "" {
			pname = "Unknown"
		}
		ps := playersMap[pname]
		if ps == nil {
			ps = &PlayerStats{
				PlayerName:    pname,
				ByGameRounds:  map[string]int{},
				ByGameWins:    map[string]int{},
				ByGameLosses:  map[string]int{},
				ByGameWinRate: map[string]float64{},
			}
			playersMap[pname] = ps
		}
		ps.TotalRounds++
		ps.BetItemsIn += betTotal
		ps.PayoutItemsOut += payoutTotal
		ps.NetAgainstCasino += net
		if didPlayerWin(entry) {
			ps.PlayerWins++
		} else if didDealerWin(entry) {
			ps.DealerWins++
		}
		if gname != "" {
			ps.ByGameRounds[gname]++
			if didPlayerWin(entry) {
				ps.ByGameWins[gname]++
			} else if didDealerWin(entry) {
				ps.ByGameLosses[gname]++
			}
		}
	}

	byItem := make([]ItemStats, 0, len(itemsMap))
	for name, it := range itemsMap {
		it.Net = it.BetIn - it.PayoutOut
		it.Status = itemStatusFromNet(it.Net)
		it.ByGameNet = map[string]int{}
		for _, g := range games {
			it.ByGameNet[g] = it.ByGameBetIn[g] - it.ByGamePayoutOut[g]
		}
		if it.GamesPlayed > 0 {
			it.CasinoWinRate = float64(it.CasinoWins) / float64(it.GamesPlayed) * 100
		}
		overall.NetItemCounts[name] = it.Net
		byItem = append(byItem, *it)
	}

	byPlayer := make([]PlayerStats, 0, len(playersMap))
	for _, p := range playersMap {
		if p.TotalRounds > 0 {
			p.PlayerWinRate = float64(p.PlayerWins) / float64(p.TotalRounds) * 100
			p.AverageNetPerGame = float64(p.NetAgainstCasino) / float64(p.TotalRounds)
		}
		p.IsProfitable = p.NetAgainstCasino < 0
		for _, g := range games {
			if p.ByGameRounds[g] > 0 {
				p.ByGameWinRate[g] = float64(p.ByGameWins[g]) / float64(p.ByGameRounds[g]) * 100
			}
		}
		byPlayer = append(byPlayer, *p)
	}

	byGameFinal := map[string]GameStats{}
	for _, name := range games {
		gs := byGame[name]
		if gs == nil {
			continue
		}
		if gs.CompletedRounds > 0 {
			gs.PlayerWinRate = float64(gs.PlayerWins) / float64(gs.CompletedRounds) * 100
			gs.DealerWinRate = float64(gs.DealerWins) / float64(gs.CompletedRounds) * 100
			gs.AverageNetPerRound = float64(gs.NetItems) / float64(gs.CompletedRounds)
		}
		if gs.TotalBetItemsIn > 0 {
			gs.RTPPercent = float64(gs.TotalPayoutItemsOut) / float64(gs.TotalBetItemsIn) * 100
			gs.CasinoEdgePercent = float64(gs.NetItems) / float64(gs.TotalBetItemsIn) * 100
		}
		byGameFinal[name] = *gs
	}

	if overall.CompletedRounds > 0 {
		overall.PlayerWinRate = float64(overall.PlayerWins) / float64(overall.CompletedRounds) * 100
		overall.DealerWinRate = float64(overall.DealerWins) / float64(overall.CompletedRounds) * 100
	}
	if overall.TotalRounds > 0 {
		overall.IssueRate = float64(overall.IssueRounds) / float64(overall.TotalRounds) * 100
	}
	if overall.TotalBetItemsIn > 0 {
		overall.RTPPercent = float64(overall.TotalPayoutItemsOut) / float64(overall.TotalBetItemsIn) * 100
		overall.ProfitMarginPercent = float64(overall.NetItems) / float64(overall.TotalBetItemsIn) * 100
		overall.CasinoEdgePercent = overall.ProfitMarginPercent
	}
	if financiallyCountableRounds > 0 {
		overall.AverageNetPerRound = float64(overall.NetItems) / float64(financiallyCountableRounds)
	}

	overall.UniquePlayers = len(playersMap)
	overall.BestSingleWin = bestSingleWin
	overall.WorstSingleLoss = worstSingleLoss

	bestGame := ""
	worstGame := ""
	bestGameNet := 0
	worstGameNet := 0
	for name, gs := range byGameFinal {
		if bestGame == "" || gs.NetItems > bestGameNet {
			bestGame = name
			bestGameNet = gs.NetItems
		}
		if worstGame == "" || gs.NetItems < worstGameNet {
			worstGame = name
			worstGameNet = gs.NetItems
		}
	}
	overall.BestGameByNet = bestGame
	overall.WorstGameByNet = worstGame

	if len(byItem) > 0 {
		sort.Slice(byItem, func(i, j int) bool { return byItem[i].Net > byItem[j].Net })
		overall.BestItemByNet = byItem[0].Name
		overall.WorstItemByNet = byItem[len(byItem)-1].Name
	}

	if len(byPlayer) > 0 {
		sort.Slice(byPlayer, func(i, j int) bool { return byPlayer[i].NetAgainstCasino > byPlayer[j].NetAgainstCasino })
		overall.MostProfitablePlayer = byPlayer[0].PlayerName
		overall.LeastProfitablePlayer = byPlayer[len(byPlayer)-1].PlayerName
		sort.Slice(byPlayer, func(i, j int) bool { return byPlayer[i].TotalRounds > byPlayer[j].TotalRounds })
	}

	dailySlice := make([]DailyStatsPoint, 0, len(dailyPoints))
	for _, dp := range dailyPoints {
		if dp.BetItemsIn > 0 {
			dp.RTPPercent = float64(dp.PayoutItemsOut) / float64(dp.BetItemsIn) * 100
		}
		dailySlice = append(dailySlice, *dp)
	}
	sort.Slice(dailySlice, func(i, j int) bool { return dailySlice[i].Day < dailySlice[j].Day })

	_ = earliestEntryTime

	stats := CasinoStats{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Range: StatsRange{
			Key: rangeKey,
			StartAt: func() string {
				if start.IsZero() {
					return ""
				}
				return start.Format(time.RFC3339)
			}(),
			EndAt: end.Format(time.RFC3339),
		},
		Overall:  overall,
		ByGame:   map[string]GameStats{},
		ByItem:   byItem,
		ByPlayer: byPlayer,
		Issues:   issues,
		Trends:   TrendStats{Daily: dailySlice},
	}

	for k, v := range byGameFinal {
		stats.ByGame[k] = v
	}

	return stats
}

func (a *App) GetCasinoStatsJSON(rangeKey string) string {
	stats := a.BuildCasinoStats(rangeKey)
	b, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
