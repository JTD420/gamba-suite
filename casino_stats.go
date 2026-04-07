package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// casino_stats.go -- build computed casino statistics from GameHistoryEntry

type CasinoStats struct {
	GeneratedAt string               `json:"generatedAt"`
	Range       StatsRange           `json:"range"`
	Overall     CasinoStatsSummary   `json:"overall"`
	ByGame      map[string]GameStats `json:"byGame"`
	ByItem      []ItemStats          `json:"byItem"`
	ByPlayer    []PlayerStats        `json:"byPlayer"`
	Streaks     StreakStats          `json:"streaks"`
	Issues      IssueStats           `json:"issues"`
}

type StatsRange struct {
	Key     string `json:"key"`
	StartAt string `json:"startAt,omitempty"`
	EndAt   string `json:"endAt,omitempty"`
}

type CasinoStatsSummary struct {
	TotalRounds         int            `json:"totalRounds"`
	CompletedRounds     int            `json:"completedRounds"`
	IssueRounds         int            `json:"issueRounds"`
	PlayerWins          int            `json:"playerWins"`
	DealerWins          int            `json:"dealerWins"`
	Pushes              int            `json:"pushes"`
	PlayerWinRate       float64        `json:"playerWinRate"`
	DealerWinRate       float64        `json:"dealerWinRate"`
	IssueRate           float64        `json:"issueRate"`
	TotalBetItemsIn     int            `json:"totalBetItemsIn"`
	TotalPayoutItemsOut int            `json:"totalPayoutItemsOut"`
	NetItems            int            `json:"netItems"`
	RTPPercent          float64        `json:"rtpPercent"`
	ProfitMarginPercent float64        `json:"profitMarginPercent"`
	BetItemCounts       map[string]int `json:"betItemCounts"`
	PayoutItemCounts    map[string]int `json:"payoutItemCounts"`
	NetItemCounts       map[string]int `json:"netItemCounts"`
}

type GameStats struct {
	Game                string         `json:"game"`
	TotalRounds         int            `json:"totalRounds"`
	CompletedRounds     int            `json:"completedRounds"`
	IssueRounds         int            `json:"issueRounds"`
	PlayerWins          int            `json:"playerWins"`
	DealerWins          int            `json:"dealerWins"`
	Pushes              int            `json:"pushes"`
	PlayerWinRate       float64        `json:"playerWinRate"`
	DealerWinRate       float64        `json:"dealerWinRate"`
	TotalBetItemsIn     int            `json:"totalBetItemsIn"`
	TotalPayoutItemsOut int            `json:"totalPayoutItemsOut"`
	NetItems            int            `json:"netItems"`
	AverageBetSize      float64        `json:"averageBetSize"`
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
	ByGameBetIn     map[string]int `json:"byGameBetIn"`
	ByGamePayoutOut map[string]int `json:"byGamePayoutOut"`
	ByGameNet       map[string]int `json:"byGameNet"`
}

type PlayerStats struct {
	PlayerName       string         `json:"playerName"`
	TotalRounds      int            `json:"totalRounds"`
	PlayerWins       int            `json:"playerWins"`
	DealerWins       int            `json:"dealerWins"`
	IssueRounds      int            `json:"issueRounds"`
	BetItemsIn       int            `json:"betItemsIn"`
	PayoutItemsOut   int            `json:"payoutItemsOut"`
	NetAgainstCasino int            `json:"netAgainstCasino"`
	ByGameRounds     map[string]int `json:"byGameRounds"`
}

type StreakStats struct {
	CurrentDealerWinStreak int `json:"currentDealerWinStreak"`
	CurrentPlayerWinStreak int `json:"currentPlayerWinStreak"`
	LongestDealerWinStreak int `json:"longestDealerWinStreak"`
	LongestPlayerWinStreak int `json:"longestPlayerWinStreak"`
}

type IssueStats struct {
	TotalIssues          int            `json:"totalIssues"`
	ByReason             map[string]int `json:"byReason"`
	GameChoiceTimeouts   int            `json:"gameChoiceTimeouts"`
	PayoutTimeouts       int            `json:"payoutTimeouts"`
	PayoutCancelFlags    int            `json:"payoutCancelFlags"`
	TradeConfirmTimeouts int            `json:"tradeConfirmTimeouts"`
}

// Helper functions
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

	// prepare containers
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

	// Sort entries by parsed time ascending for streak calculation
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

	// Track streaks
	longestPlayerStreak := 0
	longestDealerStreak := 0
	currentStreakType := 0 // 1 player, -1 dealer
	currentStreakCount := 0

	// We'll also collect financials and counts
	for _, entry := range entries {
		t := parseEntryTime(entry)
		if !start.IsZero() {
			if t.IsZero() || t.Before(start) || t.After(end) {
				continue
			}
		} else {
			// all_time: include entries with zero time as well
			if !t.IsZero() && t.After(end) {
				continue
			}
		}

		overall.TotalRounds++

		if entry.CompletedAt != "" {
			overall.CompletedRounds++
		}
		if entry.Issue {
			overall.IssueRounds++
			issues.TotalIssues++
			issues.ByReason[entry.IssueReason]++
			// quick heuristics
			lower := strings.ToLower(entry.IssueReason)
			if strings.Contains(lower, "choice") || strings.Contains(lower, "timeout") && strings.Contains(lower, "choice") {
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

		// count per-game rounds (all rounds for that game)
		gname := normalizeGameName(entry.Game)
		if gname != "" {
			gs := byGame[gname]
			if gs != nil {
				gs.TotalRounds++
				if entry.CompletedAt != "" {
					gs.CompletedRounds++
				}
				if entry.Issue {
					gs.IssueRounds++
				}
			}
		}

		// count wins for completed/resolved rounds
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

		// financials: only include rounds that meet the financial rules
		if isFinanciallyCountableRound(entry) {
			betTotal := totalTradeItemQuantity(entry.BetItems)
			payoutTotal := totalTradeItemQuantity(entry.PayoutItems)
			net := betTotal - payoutTotal

			overall.TotalBetItemsIn += betTotal
			overall.TotalPayoutItemsOut += payoutTotal
			overall.NetItems += net

			// accumulate item counts
			for k, v := range tradeItemsToCountMap(entry.BetItems) {
				overall.BetItemCounts[k] += v
				it := itemsMap[k]
				if it == nil {
					it = &ItemStats{Name: k, ByGameBetIn: map[string]int{}, ByGamePayoutOut: map[string]int{}, ByGameNet: map[string]int{}}
					itemsMap[k] = it
				}
				it.BetIn += v
				if gname != "" {
					it.ByGameBetIn[gname] += v
				}
			}
			for k, v := range tradeItemsToCountMap(entry.PayoutItems) {
				overall.PayoutItemCounts[k] += v
				it := itemsMap[k]
				if it == nil {
					it = &ItemStats{Name: k, ByGameBetIn: map[string]int{}, ByGamePayoutOut: map[string]int{}, ByGameNet: map[string]int{}}
					itemsMap[k] = it
				}
				it.PayoutOut += v
				if gname != "" {
					it.ByGamePayoutOut[gname] += v
				}
			}

			// per-game financials
			if gname != "" {
				gs := byGame[gname]
				if gs != nil {
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
					// item counts per game
					for k, v := range tradeItemsToCountMap(entry.BetItems) {
						gs.BetItemCounts[k] += v
					}
					for k, v := range tradeItemsToCountMap(entry.PayoutItems) {
						gs.PayoutItemCounts[k] += v
					}
				}
			}

			// per-player
			pname := strings.TrimSpace(entry.PlayerName)
			if pname == "" {
				pname = "Unknown"
			}
			ps := playersMap[pname]
			if ps == nil {
				ps = &PlayerStats{PlayerName: pname, ByGameRounds: map[string]int{}}
				playersMap[pname] = ps
			}
			ps.TotalRounds++
			if didPlayerWin(entry) {
				ps.PlayerWins++
			} else if didDealerWin(entry) {
				ps.DealerWins++
			}
			if entry.Issue {
				ps.IssueRounds++
			}
			ps.BetItemsIn += betTotal
			ps.PayoutItemsOut += payoutTotal
			ps.NetAgainstCasino += net
			if gname != "" {
				ps.ByGameRounds[gname]++
			}
		}
	}

	// finalize item maps and compute nets
	byItem := make([]ItemStats, 0, len(itemsMap))
	for name, it := range itemsMap {
		it.Net = it.BetIn - it.PayoutOut
		// compute per-game net
		it.ByGameNet = map[string]int{}
		for _, g := range []string{"Poker", "21", "13", "Tri"} {
			it.ByGameNet[g] = it.ByGameBetIn[g] - it.ByGamePayoutOut[g]
		}
		overall.NetItemCounts[name] = it.BetIn - it.PayoutOut
		byItem = append(byItem, *it)
		_ = name
	}

	// finalize player list
	byPlayer := make([]PlayerStats, 0, len(playersMap))
	for _, p := range playersMap {
		byPlayer = append(byPlayer, *p)
	}

	// finalize per-game stats into map[string]GameStats
	byGameFinal := map[string]GameStats{}
	for _, name := range games {
		gsPtr := byGame[name]
		if gsPtr == nil {
			continue
		}
		if gsPtr.CompletedRounds > 0 {
			gsPtr.PlayerWinRate = float64(gsPtr.PlayerWins) / float64(gsPtr.CompletedRounds) * 100
			gsPtr.DealerWinRate = float64(gsPtr.DealerWins) / float64(gsPtr.CompletedRounds) * 100
			gsPtr.AverageBetSize = float64(gsPtr.TotalBetItemsIn) / float64(gsPtr.CompletedRounds)
		}
		// copy maps
		if gsPtr.BetItemCounts == nil {
			gsPtr.BetItemCounts = map[string]int{}
		}
		if gsPtr.PayoutItemCounts == nil {
			gsPtr.PayoutItemCounts = map[string]int{}
		}
		gs := *gsPtr
		byGameFinal[name] = gs
	}

	// compute overall rates
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
	}

	// longest streaks (scan chronological list)
	currentStreakType = 0
	currentStreakCount = 0
	for _, entry := range entries {
		// only consider completed/non-issue rounds
		if entry.Issue || entry.CompletedAt == "" {
			continue
		}
		wt := 0
		if didPlayerWin(entry) {
			wt = 1
		} else if didDealerWin(entry) {
			wt = -1
		} else {
			wt = 0
		}
		if wt == 0 {
			currentStreakType = 0
			currentStreakCount = 0
			continue
		}
		if wt == currentStreakType {
			currentStreakCount++
		} else {
			currentStreakType = wt
			currentStreakCount = 1
		}
		if currentStreakType == 1 && currentStreakCount > longestPlayerStreak {
			longestPlayerStreak = currentStreakCount
		}
		if currentStreakType == -1 && currentStreakCount > longestDealerStreak {
			longestDealerStreak = currentStreakCount
		}
	}

	// current streak (walk newest->oldest)
	currentDealerStreak := 0
	currentPlayerStreak := 0
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Issue || e.CompletedAt == "" {
			continue
		}
		if didDealerWin(e) {
			if currentPlayerStreak > 0 {
				break
			}
			currentDealerStreak++
		} else if didPlayerWin(e) {
			if currentDealerStreak > 0 {
				break
			}
			currentPlayerStreak++
		} else {
			break
		}
	}

	// sort item list by net descending
	sort.Slice(byItem, func(i, j int) bool {
		return byItem[i].Net > byItem[j].Net
	})
	// sort players by rounds desc
	sort.Slice(byPlayer, func(i, j int) bool {
		return byPlayer[i].TotalRounds > byPlayer[j].TotalRounds
	})

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
		Streaks: StreakStats{
			CurrentDealerWinStreak: currentDealerStreak,
			CurrentPlayerWinStreak: currentPlayerStreak,
			LongestDealerWinStreak: longestDealerStreak,
			LongestPlayerWinStreak: longestPlayerStreak,
		},
		Issues: issues,
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
