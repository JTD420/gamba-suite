package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	g "xabbo.b7c.io/goearth"
	gencoding "xabbo.b7c.io/goearth/encoding"
	"xabbo.b7c.io/goearth/shockwave/inventory"
	"xabbo.b7c.io/goearth/shockwave/in"
	"xabbo.b7c.io/goearth/shockwave/out"
	room "xabbo.b7c.io/goearth/shockwave/room"
)

// Global variables for dice management, rolling state, mutex, and wait group
var (
	diceList         []*Dice
	mutedDuration    int
	isMuted          bool
	currentSum       int
	commandList      string
	awaitingTradeOpen bool
	tradeOpenCount    int
	tradeCloseCount   int
	lastTradePartnerID int
	lastTradePartnerName string
	lastTradePartnerToken string
	tradeAutoFlowID    int
	tradeAutoAccepted  bool
	tradeAutoConfirmed bool
	tradeAutoAcceptPending bool
	tradeAutoConfirmPending bool
	tradeCompleted bool
	tradeCloseAnnounced bool
	lastTradeCoverageNotice string
	lastTradeBlockNotice string
	underfundedTradeMonitorID int
	underfundedTradeMonitorNotice string
	lastTradeOpenData string
	lastTradeOpen     string
	isPokerRolling   bool
	isTriRolling     bool
	isBJRolling      bool
	is13Rolling      bool
	is13Hitting      bool
	isHitting        bool
	isClosing        bool
	ChatIsDisabled   bool
	mutex            sync.Mutex
	resultsWaitGroup sync.WaitGroup
	rollDelay        = 550 * time.Millisecond
	tradeUserPattern = regexp.MustCompile(`\[(\d+)\]`)
	stripItemNameRe  = regexp.MustCompile(`(?:CF_\d+_[a-z][a-z_]*|[a-z][a-z0-9_]*_[a-z0-9_]+)(?:\*\d+)?`)
	allowedTradeItemAliases = map[string]string{
		"club_sofa":     "club_sofa",
		"hc_sofa":       "club_sofa",
		"chair_plasty": "chair_plasty",
		"plastic_chair": "chair_plasty",
		"plstic_chair":  "chair_plasty",
	}
	roomEntities     = map[int]room.Entity{}
	roomMu           sync.Mutex
	users28ByToken   = map[string]string{}
	users28Mu        sync.Mutex
	headerSniffUntil time.Time
	headerSniffSeen  = map[uint16]bool{}
	headerSniffMu    sync.Mutex
	currentTradeItems    []TradeItem
	currentOwnTradeItems []TradeItem
	tradeItemsMu         sync.Mutex
	lastAddItemWasOurs   bool
	addItemMu            sync.Mutex
	currentHandItems  []TradeItem
	handItemsMu       sync.Mutex
	stripScanMu       sync.Mutex
	stripScanActive   bool
	stripScanSeenItemIDs = map[int]struct{}{}
	stripScanCounts      = map[string]int{}
	stripScanRawByName   = map[string]string{}
	knownDiceIDs     = map[int]struct{}{}
	fakeDiceTestingMode bool
	dealerOpenHeartbeatID int
	dealerOpenHeartbeatActive bool
)

type TradeItem struct {
	Name     string
	Quantity int
	RawData  string // Store raw field for debugging
}

type tradeShortage struct {
	Name       string
	Required   int
	Have       int
	PayoutTotal int
}

type App struct {
	ext    *g.Ext
	assets embed.FS
	log    []string
	logMu  sync.Mutex
	chatLog   []string
	chatLogMu sync.Mutex
	ctx    context.Context
}

type PokerDisplayConfig struct {
	FiveOfAKind  string `json:"five_of_a_kind"`
	FourOfAKind  string `json:"four_of_a_kind"`
	FullHouse    string `json:"full_house"`
	HighStraight string `json:"high_straight"`
	LowStraight  string `json:"low_straight"`
	ThreeOfAKind string `json:"three_of_a_kind"`
	TwoPair      string `json:"two_pair"`
	OnePair      string `json:"one_pair"`
	Nothing      string `json:"nothing"`
}

func NewApp(ext *g.Ext, assets embed.FS) *App {
	return &App{
		ext:    ext,
		assets: assets,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.setupExt()
	go func() {
		a.runExt()
	}()
	go func() {
		time.Sleep(1500 * time.Millisecond)
		a.requestPlayerStrip()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			a.requestPlayerStrip()
		}
	}()
}

func (a *App) LoadConfig() *PokerDisplayConfig {
	configFilePath := getConfigFilePath()

	file, err := os.Open(configFilePath)
	if err != nil {
		a.AddLogMsg("Config file not found, loading default values")
		return &PokerDisplayConfig{
			FiveOfAKind:  "Five of a kind: %s",
			FourOfAKind:  "Four of a kind: %s",
			FullHouse:    "Full House: %s",
			HighStraight: "High Str8",
			LowStraight:  "Low Str8",
			ThreeOfAKind: "Three of a kind: %s",
			TwoPair:      "Two Pair: %ss",
			OnePair:      "One Pair: %ss",
			Nothing:      "Nothing",
		}
	}
	defer file.Close()

	var config PokerDisplayConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		a.AddLogMsg("Error decoding config file: " + err.Error())
		return nil
	}

	// Config file loaded successfully
	return &config
}

func (a *App) SaveConfig(config *PokerDisplayConfig) {
	configFilePath := getConfigFilePath()

	file, err := os.Create(configFilePath)
	if err != nil {
		a.AddLogMsg("Error creating config file: " + err.Error())
		return
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(config); err != nil {
		a.AddLogMsg("Error encoding config file: " + err.Error())
		return
	}

	a.AddLogMsg("Config file saved successfully")
}

func (a *App) dealerOpenMessage() string {
	return "Dealer Open, Trade Away"
}

func getConfigFilePath() string {
	configDir, _ := os.UserConfigDir()
	configPath := filepath.Join(configDir, "Gamba-Suite")
	os.MkdirAll(configPath, 0700)
	return filepath.Join(configPath, "poker_display_config.json")
}

func (a *App) setupExt() {
	registerCustomTradeHeaders(a)

	a.ext.Intercept(out.CHAT, out.SHOUT, out.WHISPER).With(a.onChatMessage)
	a.ext.Intercept(out.THROW_DICE).With(a.handleThrowDice)
	a.ext.Intercept(out.DICE_OFF).With(a.handleDiceOff)
	a.ext.Intercept(in.DICE_VALUE).With(a.handleDiceResult)
	a.ext.Intercept(in.CHAT, in.CHAT_2, in.CHAT_3).With(a.handleIncomingChat)
	a.ext.Intercept(in.ROOM_READY).With(a.handleRoomReady)
	a.ext.Intercept(in.USERS).With(a.handleRoomUsers)
	a.ext.Intercept(in.SPACENODEUSERS).With(a.handleRoomUsers)
	a.ext.Intercept(out.CHAT).With(a.handleTalk)
	a.ext.Intercept(out.SHOUT).With(a.handleTalk)
	a.ext.InterceptAll(func(e *g.Intercept) {
		handleMutePacket(e)
		handleTradePacket(a, e)
		handleUsers28Packet(a, e)
		handleIncomingHeaderSniff(a, e)
		handleStripPacket(a, e)
	})
}

func registerCustomTradeHeaders(a *App) {
	confirmID := g.Out.Id("TRADE_CONFIRM_ACCEPT")
	if _, ok := a.ext.Headers().TryGet(confirmID); !ok {
		a.ext.Headers().Add("TRADE_CONFIRM_ACCEPT", g.Header{Dir: g.Out, Value: 402})
		a.AddLogMsg("[TRADE_HEADERS] registered outgoing TRADE_CONFIRM_ACCEPT -> 402")
		log.Printf("[TRADE_HEADERS] registered outgoing TRADE_CONFIRM_ACCEPT -> 402")
	}
}

func (a *App) runExt() {
	defer os.Exit(0)
	a.ext.Run()
}

func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
}

func startMuteTimer(duration int) {
	for duration > 0 {
		log.Printf("Remaining mute time: %d seconds", duration)
		time.Sleep(1 * time.Second) // Sleep for 1 second
		duration--
	}

	// Mute duration finished
	handleMuteEnd()
}

func handleMuteEnd() {
	isMuted = false
	log.Println("Mute finished, sending queued messages...")

	// ToDo:
	// // Send all queued messages
	// for _, message := range messageQueue {
	// 	sendMessageWithDelay(message)
	// }

	// // Clear the message queue
	// messageQueue = []string{}
}

// Mute detection logic (called within InterceptAll)
func handleMutePacket(e *g.Intercept) {
	// Check for the "first muted" packet with header 4069
	if e.Packet.Header.Value == 4069 {
		mutedDuration = e.Packet.ReadInt() // Read the mute duration in seconds
		log.Printf("You are muted for %d seconds.", mutedDuration)
		isMuted = true
		go startMuteTimer(mutedDuration) // Start the mute timer
	}

	// Check for the "trying to chat while muted" packet with header 3285
	if e.Packet.Header.Value == 3285 {
		remainingMuteDuration := e.Packet.ReadInt() // Read the remaining mute duration
		log.Printf("Mute still active, remaining time: %d seconds.", remainingMuteDuration)
	}
}

// Trade detection logic (called within InterceptAll)
func handleTradePacket(a *App, e *g.Intercept) {
	if e.Packet.Header.Dir == g.Out && (e.Packet.Header.Value == 69 || e.Packet.Header.Value == 402) {
		shortages := a.getTradeCoverageShortages()
		if len(shortages) > 0 {
			notice := formatTradeShortages(shortages)
			msg := fmt.Sprintf("Trade blocked: insufficient payout stock (%s)", notice)
			e.Block()
			a.AddLogMsg("[TRADE_GUARD] " + msg)
			log.Printf("[TRADE_GUARD] %s", msg)
			if notice != lastTradeBlockNotice {
				lastTradeBlockNotice = notice
				ext.Send(out.SHOUT, msg)
			}
			return
		}
	}

	// TRADE_ADDITEM outgoing 72 - we are adding an item; flag next TRADE_ITEMS as ours
	if e.Packet.Header.Dir == g.Out && e.Packet.Header.Value == 72 {
		addItemMu.Lock()
		lastAddItemWasOurs = true
		addItemMu.Unlock()
		a.AddLogMsg("[TRADE_ADDITEM #72] outgoing: next TRADE_ITEMS belongs to us")
		log.Printf("[TRADE_ADDITEM #72] outgoing: next TRADE_ITEMS belongs to us")
		return
	}

	// TRADE_ACCEPT incoming 109 - wait 2 seconds then send TRADE_ACCEPT (69)
	if e.Packet.Header.Value == 109 {
		scheduleAutoTradeAccept(a, string(e.Packet.Data))
		return
	}

	// TRADE_CONFIRM incoming 111 - wait 4 seconds then send TRADE_CONFIRM_ACCEPT (402)
	if e.Packet.Header.Value == 111 {
		scheduleAutoTradeConfirm(a, string(e.Packet.Data))
		return
	}

	// TRADE_ITEMS header 108 - incoming server echo of full trade state (always incoming)
	// Ownership: if we just sent TRADE_ADDITEM[72], the new item is ours; otherwise partner triggered.
	// Compute each side by diffing total items against the other side's last known list.
	if e.Packet.Header.Value == 108 {
		allItems := parseTradeItemsPacket(e.Packet.Data)

		addItemMu.Lock()
		wasOurs := lastAddItemWasOurs
		lastAddItemWasOurs = false
		addItemMu.Unlock()

		side := "partner"
		if wasOurs {
			side = "yours"
		}

		tradeItemsMu.Lock()
		if wasOurs {
			// We added an item: our offer = total minus known partner items
			currentOwnTradeItems = diffItems(allItems, currentTradeItems)
		} else {
			// Partner added an item: their offer = total minus our known items
			currentTradeItems = diffItems(allItems, currentOwnTradeItems)
		}
		computedPartner := len(currentTradeItems)
		computedOwn := len(currentOwnTradeItems)
		tradeItemsMu.Unlock()

		a.AddLogMsg(fmt.Sprintf("[TRADE_ITEMS #108] side=%s total=%d partner=%d own=%d", side, len(allItems), computedPartner, computedOwn))
		log.Printf("[TRADE_ITEMS #108] side=%s total=%d partner=%d own=%d", side, len(allItems), computedPartner, computedOwn)

		for i, item := range allItems {
			a.AddLogMsg(fmt.Sprintf("[TRADE_ITEMS #108] raw[%d] name=%q quantity=%d", i, item.Name, item.Quantity))
			log.Printf("[TRADE_ITEMS #108] raw[%d] name=%q quantity=%d", i, item.Name, item.Quantity)
		}

		a.emitTradeItemsUpdate(side)
		a.notifyTradeQuantityCoverage()
		return
	}
	
	// TRADE_COMPLETED header 112 - send chat message with the traded items
	if e.Packet.Header.Value == 112 {
		tradeCompleted = true
		tradeAutoConfirmed = true
		tradeAutoConfirmPending = false
		a.AddLogMsg("[TRADE_COMPLETED #112] trade completed, sending trade summary")
		log.Printf("[TRADE_COMPLETED #112] trade completed, sending trade summary")
		
		// Send the trade items summary to chat
		a.sendTradeCompletionMessage()
		go a.requestPlayerStrip()
		return
	}
	
	if e.Packet.Header.Value == 104 {
		stopUnderfundedTradeMonitor()
		lastTradeCoverageNotice = ""
		lastTradeBlockNotice = ""
		if !awaitingTradeOpen {
			return
		}
		stopDealerOpenHeartbeat()

		resetTradeAutoFlow()

		for _, decodeLine := range decodeTradeOpenPacket(e.Packet) {
			a.AddLogMsg("[TRADE_OPEN_DECODE] " + decodeLine)
			log.Printf("[TRADE_OPEN_DECODE] %s", decodeLine)
		}

		for _, candidateLine := range describeTradeRoomCandidates(a.ext) {
			a.AddLogMsg("[TRADE_ROOM] " + candidateLine)
			log.Printf("[TRADE_ROOM] %s", candidateLine)
		}

		if len(e.Packet.Data) >= 4 {
			tradeToken := string(e.Packet.Data[:4])
			lastTradePartnerToken = tradeToken
			if name, ok := lookupUsers28Token(tradeToken); ok {
				a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN] token %q matched user %q", tradeToken, name))
				log.Printf("[TRADE_OPEN] token %q matched user %q", tradeToken, name)
			} else {
				a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN] token %q not found in USERS[28] cache", tradeToken))
				log.Printf("[TRADE_OPEN] token %q not found in USERS[28] cache", tradeToken)
			}

			if idx, name, ok := resolveTradeTokenToRoomIndex(tradeToken); ok {
				lastTradePartnerID = idx
				lastTradePartnerName = name
				outPreview := string(ext.NewPacket(out.TRADE_OPEN, idx).Data)
				a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN] token %q resolved room index %d (%q) -> outgoing[%d] %q", tradeToken, idx, name, 71, outPreview))
				log.Printf("[TRADE_OPEN] token %q resolved room index %d (%q) -> outgoing[%d] %q", tradeToken, idx, name, 71, outPreview)
			}
		}

		tradePayload := strings.TrimSpace(string(e.Packet.Data))
		if tradePayload == "" {
			tradePayload = "(empty payload)"
		}

		tradeOpenCount++
		lastTradeOpenData = string(e.Packet.Data)
		lastTradeOpen = fmt.Sprintf("Incoming[%d] -> %s", e.Packet.Header.Value, tradePayload)
		if lastTradePartnerID > 0 {
			partnerID := lastTradePartnerID
			outPreview := string(ext.NewPacket(out.TRADE_OPEN, partnerID).Data)
			a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN #%d] derived outgoing[%d] -> %s (partner id %d)", tradeOpenCount, 71, outPreview, partnerID))
			log.Printf("[TRADE_OPEN #%d] derived outgoing[%d] -> %s (partner id %d)", tradeOpenCount, 71, outPreview, partnerID)
		} else if partnerID, ok := extractTradePartnerID(tradePayload); ok {
			lastTradePartnerID = partnerID
			lastTradePartnerName = "Unknown"
			outPreview := string(ext.NewPacket(out.TRADE_OPEN, partnerID).Data)
			a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN #%d] fallback candidate outgoing[%d] -> %s (partner id %d)", tradeOpenCount, 71, outPreview, partnerID))
			log.Printf("[TRADE_OPEN #%d] fallback candidate outgoing[%d] -> %s (partner id %d)", tradeOpenCount, 71, outPreview, partnerID)
		} else {
			lastTradePartnerID = 0
			a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN #%d] unresolved reopen target: waiting for room-user mapping", tradeOpenCount))
			log.Printf("[TRADE_OPEN #%d] unresolved reopen target: waiting for room-user mapping", tradeOpenCount)
		}
		awaitingTradeOpen = false
		log.Printf("[TRADE_OPEN #%d] %s", tradeOpenCount, lastTradeOpen)
		a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN #%d] %s", tradeOpenCount, lastTradeOpen))

		partnerName := strings.TrimSpace(lastTradePartnerName)
		if partnerName == "" {
			partnerName = "Unknown"
		}
		openMsg := fmt.Sprintf("Trade Opened: \"%s\"", partnerName)
		a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN] shouting: %q", openMsg))
		log.Printf("[TRADE_OPEN] shouting: %q", openMsg)
		ext.Send(out.SHOUT, openMsg)
		go a.requestPlayerStrip()
		return
	}

	// TRADE_CLOSE appears as incoming header 110 in your client logs.
	if e.Packet.Header.Value == 110 {
		stopUnderfundedTradeMonitor()
		wasCompleted := tradeCompleted
		tradeAutoConfirmed = true
		tradeAutoConfirmPending = false
		partnerName := strings.TrimSpace(lastTradePartnerName)
		if partnerName == "" {
			partnerName = "Unknown"
		}

		if !tradeCompleted && !tradeCloseAnnounced {
			closeMsg := fmt.Sprintf("Trade Closed: \"%s\"", partnerName)
			a.AddLogMsg(fmt.Sprintf("[TRADE_CLOSE] shouting: %q", closeMsg))
			log.Printf("[TRADE_CLOSE] shouting: %q", closeMsg)
			ext.Send(out.SHOUT, closeMsg)
			tradeCloseAnnounced = true
		}

		tradeClosePayload := strings.TrimSpace(string(e.Packet.Data))
		if tradeClosePayload == "" {
			tradeClosePayload = "(empty payload)"
		}

		tradeCloseCount++
		closeLog := fmt.Sprintf("Incoming[%d] -> %s", e.Packet.Header.Value, tradeClosePayload)
		log.Printf("[TRADE_CLOSE #%d] %s", tradeCloseCount, closeLog)
		a.AddLogMsg(fmt.Sprintf("[TRADE_CLOSE #%d] %s", tradeCloseCount, closeLog))
		resetTradeAutoFlow()
		lastTradePartnerToken = ""
		
		// Clear trade items when trade closes
		a.ClearTradeItems()

		if !wasCompleted {
			awaitingTradeOpen = true
			a.AddLogMsg("[TRADE_REOPEN] trade closed before completion, restarting dealer cycle")
			log.Printf("[TRADE_REOPEN] trade closed before completion, restarting dealer cycle")
			go sendMessageWithDelay(a.dealerOpenMessage())
			startDealerOpenHeartbeat(a)
		}

		partnerID := lastTradePartnerID
		if partnerID <= 0 {
			requestRoomUsers(a)
			a.AddLogMsg("[TRADE_REOPEN] not ready: no last trade target, requested room users")
			return
		}

		a.AddLogMsg(fmt.Sprintf("[TRADE_REOPEN] ready for manual reopen -> %s (%d)", lastTradePartnerName, partnerID))
		log.Printf("[TRADE_REOPEN] ready for manual reopen -> %s (%d)", lastTradePartnerName, partnerID)
	}
}

func resetTradeAutoFlow() {
	tradeAutoFlowID++
	tradeAutoAccepted = false
	tradeAutoConfirmed = false
	tradeAutoAcceptPending = false
	tradeAutoConfirmPending = false
	tradeCompleted = false
	tradeCloseAnnounced = false
}

func stopUnderfundedTradeMonitor() {
	underfundedTradeMonitorID++
	underfundedTradeMonitorNotice = ""
}

func startUnderfundedTradeMonitor(a *App, notice string) {
	underfundedTradeMonitorID++
	monitorID := underfundedTradeMonitorID
	underfundedTradeMonitorNotice = notice

	go func(id int) {
		warn := func(message string) bool {
			if id != underfundedTradeMonitorID {
				return false
			}
			if len(a.getTradeCoverageShortages()) == 0 {
				return false
			}
			a.AddLogMsg("[TRADE_COVERAGE] " + message)
			log.Printf("[TRADE_COVERAGE] %s", message)
			ext.Send(out.SHOUT, message)
			return true
		}

		if !warn("Closing trade in 20secs if offer is not reduced.") {
			return
		}
		time.Sleep(10 * time.Second)

		if !warn("Closing trade in 10secs if offer is not reduced.") {
			return
		}
		time.Sleep(5 * time.Second)

		if !warn("Closing trade in 5secs if offer is not reduced.") {
			return
		}
		time.Sleep(5 * time.Second)

		if id != underfundedTradeMonitorID || len(a.getTradeCoverageShortages()) == 0 {
			return
		}

		a.AddLogMsg("[TRADE_COVERAGE] closing underfunded trade now")
		log.Printf("[TRADE_COVERAGE] closing underfunded trade now")
		ext.Send(out.TRADE_CLOSE)
	}(monitorID)
}

func startDealerOpenHeartbeat(a *App) {
	dealerOpenHeartbeatID++
	heartbeatID := dealerOpenHeartbeatID
	dealerOpenHeartbeatActive = true

	go func(id int) {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if id != dealerOpenHeartbeatID {
				return
			}

			if !awaitingTradeOpen {
				dealerOpenHeartbeatActive = false
				return
			}

			a.AddLogMsg("[TRADE_REOPEN] no new trade yet, re-announcing dealer open")
			log.Printf("[TRADE_REOPEN] no new trade yet, re-announcing dealer open")
			sendMessageWithDelay(a.dealerOpenMessage())
		}
	}(heartbeatID)
}

func stopDealerOpenHeartbeat() {
	if !dealerOpenHeartbeatActive {
		return
	}
	dealerOpenHeartbeatID++
	dealerOpenHeartbeatActive = false
}

func scheduleAutoTradeAccept(a *App, payload string) {
	if strings.TrimSpace(lastTradePartnerToken) == "" {
		return
	}
	if tradeAutoAccepted || tradeAutoAcceptPending {
		return
	}
	if len(a.getTradeCoverageShortages()) > 0 {
		a.AddLogMsg("[TRADE_ACCEPT] skipped auto-accept due to insufficient payout stock")
		log.Printf("[TRADE_ACCEPT] skipped auto-accept due to insufficient payout stock")
		return
	}

	tradeAutoAcceptPending = true
	flowID := tradeAutoFlowID
	a.AddLogMsg(fmt.Sprintf("[TRADE_ACCEPT #109] detected (%q), auto-accept in 2s", payload))
	log.Printf("[TRADE_ACCEPT #109] detected (%q), auto-accept in 2s", payload)

	go func(flow int) {
		time.Sleep(2 * time.Second)

		if flow != tradeAutoFlowID || strings.TrimSpace(lastTradePartnerToken) == "" {
			tradeAutoAcceptPending = false
			return
		}

		if len(a.getTradeCoverageShortages()) > 0 {
			tradeAutoAcceptPending = false
			a.AddLogMsg("[TRADE_ACCEPT] canceled auto-accept due to insufficient payout stock")
			log.Printf("[TRADE_ACCEPT] canceled auto-accept due to insufficient payout stock")
			return
		}

		ext.Send(out.TRADE_ACCEPT)
		tradeAutoAcceptPending = false
		tradeAutoAccepted = true
		a.AddLogMsg("[TRADE_ACCEPT] sent outgoing[69]")
		log.Printf("[TRADE_ACCEPT] sent outgoing[69]")
	}(flowID)
}

func scheduleAutoTradeConfirm(a *App, payload string) {
	if tradeAutoConfirmed || tradeAutoConfirmPending {
		return
	}
	if len(a.getTradeCoverageShortages()) > 0 {
		a.AddLogMsg("[TRADE_CONFIRM_ACCEPT] skipped auto-confirm due to insufficient payout stock")
		log.Printf("[TRADE_CONFIRM_ACCEPT] skipped auto-confirm due to insufficient payout stock")
		return
	}

	tradeAutoConfirmPending = true
	flowID := tradeAutoFlowID
	a.AddLogMsg(fmt.Sprintf("[TRADE_CONFIRM #111] detected (%q), auto-confirm starts in 4s with up to 10 attempts", payload))
	log.Printf("[TRADE_CONFIRM #111] detected (%q), auto-confirm starts in 4s with up to 10 attempts", payload)

	go func(flow int) {
		defer func() {
			if r := recover(); r != nil {
				tradeAutoConfirmPending = false
				a.AddLogMsg(fmt.Sprintf("[TRADE_CONFIRM_ACCEPT] recovered from panic: %v", r))
				log.Printf("[TRADE_CONFIRM_ACCEPT] recovered from panic: %v", r)
			}
		}()

		for attempt := 1; attempt <= 10; attempt++ {
			time.Sleep(4 * time.Second)

			if flow != tradeAutoFlowID || tradeAutoConfirmed {
				tradeAutoConfirmPending = false
				return
			}

			if len(a.getTradeCoverageShortages()) > 0 {
				tradeAutoConfirmPending = false
				a.AddLogMsg("[TRADE_CONFIRM_ACCEPT] canceled auto-confirm due to insufficient payout stock")
				log.Printf("[TRADE_CONFIRM_ACCEPT] canceled auto-confirm due to insufficient payout stock")
				return
			}

			ext.Send(g.Out.Id("TRADE_CONFIRM_ACCEPT"))
			a.AddLogMsg(fmt.Sprintf("[TRADE_CONFIRM_ACCEPT] sent outgoing[402] attempt %d/10", attempt))
			log.Printf("[TRADE_CONFIRM_ACCEPT] sent outgoing[402] attempt %d/10", attempt)
		}

		tradeAutoConfirmPending = false
		if flow == tradeAutoFlowID && !tradeAutoConfirmed {
			handleTradeConfirmTimeout(a)
		}
	}(flowID)
}

func handleTradeConfirmTimeout(a *App) {
	partnerName := strings.TrimSpace(lastTradePartnerName)
	if partnerName == "" {
		partnerName = "Unknown"
	}

	a.AddLogMsg("[TRADE_CONFIRM_ACCEPT] max attempts reached; sending trade close")
	log.Printf("[TRADE_CONFIRM_ACCEPT] max attempts reached; sending trade close")
	ext.Send(out.TRADE_CLOSE)

	closeMsg := fmt.Sprintf("Trade Closed: \"%s\"", partnerName)
	if !isMuted {
		tradeCloseAnnounced = true
		sendMessageWithDelay(closeMsg)
		sendMessageWithDelay(a.dealerOpenMessage())
	} else {
		a.AddLogMsg("[TRADE_CONFIRM_ACCEPT] user muted; skipped timeout close announcement")
		log.Printf("[TRADE_CONFIRM_ACCEPT] user muted; skipped timeout close announcement")
	}

	awaitingTradeOpen = true
	tradeAutoConfirmed = true
}

func isTradeItemsFromPartner(payload string) bool {
	if strings.TrimSpace(lastTradePartnerToken) == "" {
		return false
	}
	return strings.Contains(payload, lastTradePartnerToken)
}

func extractTradePartnerID(payload string) (int, bool) {
	matches := tradeUserPattern.FindStringSubmatch(payload)
	if len(matches) < 2 {
		return 0, false
	}

	partnerID, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}

	return partnerID, true
}

func decodeTradeOpenPacket(pkt *g.Packet) []string {
	copyPacket := func() *g.Packet {
		return &g.Packet{
			Client: pkt.Client,
			Header: pkt.Header,
			Data:   append([]byte(nil), pkt.Data...),
			Pos:    0,
		}
	}

	lines := []string{
		fmt.Sprintf("raw=%q", string(pkt.Data)),
		fmt.Sprintf("hex=% X", pkt.Data),
		fmt.Sprintf("len=%d", len(pkt.Data)),
	}

	if v, pos, ok := tryReadInt(copyPacket()); ok {
		lines = append(lines, fmt.Sprintf("layout int -> id=%d (pos=%d)", v, pos))
	}

	if id, s, pos, ok := tryReadIntString(copyPacket()); ok {
		lines = append(lines, fmt.Sprintf("layout int,string -> id=%d text=%q (pos=%d)", id, s, pos))
	}

	if s, pos, ok := tryReadString(copyPacket()); ok {
		lines = append(lines, fmt.Sprintf("layout string -> text=%q (pos=%d)", s, pos))
	}

	if a, b, pos, ok := tryReadIntInt(copyPacket()); ok {
		lines = append(lines, fmt.Sprintf("layout int,int -> a=%d b=%d (pos=%d)", a, b, pos))
	}

	lines = append(lines, scanTradeOpenFields(pkt.Data)...)

	return lines
}

func scanTradeOpenFields(data []byte) []string {
	lines := []string{}

	for offset := 0; offset < len(data); offset++ {
		remaining := len(data) - offset

		if remaining >= 2 {
			chunk := data[offset : offset+2]
			lines = append(lines, fmt.Sprintf("scan b64_2 @%d -> %q = %d", offset, string(chunk), gencoding.B64Decode(chunk)))
		}

		if remaining >= 3 {
			chunk := data[offset : offset+3]
			lines = append(lines, fmt.Sprintf("scan b64_3 @%d -> %q = %d", offset, string(chunk), gencoding.B64Decode(chunk)))
		}

		vl64Len := gencoding.VL64DecodeLen(data[offset])
		if vl64Len > 0 && vl64Len <= 6 && remaining >= vl64Len {
			chunk := data[offset : offset+vl64Len]
			lines = append(lines, fmt.Sprintf("scan vl64 @%d len=%d -> %q = %d", offset, vl64Len, string(chunk), gencoding.VL64Decode(chunk)))
		}
	}

	return lines
}

func handleUsers28Packet(a *App, e *g.Intercept) {
	if e.Packet.Header.Dir != g.In {
		return
	}

	if e.Packet.Header.Value != 28 {
		return
	}

	raw := string(e.Packet.Data)
	entries := extractUsers28Entries(raw)
	if len(entries) == 0 {
		a.AddLogMsg(fmt.Sprintf("[USERS28] received header %d but found no parseable entries raw=%q", e.Packet.Header.Value, raw))
		log.Printf("[USERS28] received header %d but found no parseable entries raw=%q", e.Packet.Header.Value, raw)
		return
	}

	users28Mu.Lock()
	for _, entry := range entries {
		users28ByToken[entry.Token] = entry.Name
	}
	users28Mu.Unlock()

	for _, entry := range entries {
		a.AddLogMsg(fmt.Sprintf("[USERS28] header=%d token=%q name=%q", e.Packet.Header.Value, entry.Token, entry.Name))
		log.Printf("[USERS28] header=%d token=%q name=%q", e.Packet.Header.Value, entry.Token, entry.Name)
	}
}

func lookupUsers28Token(token string) (string, bool) {
	users28Mu.Lock()
	defer users28Mu.Unlock()
	name, ok := users28ByToken[token]
	return name, ok
}

// parseTradeItemsPacket extracts trade items from TRADE_ITEMS packet (header 108)
// Items are separated by \x02 bytes and may contain item names and quantities
func parseTradeItemsPacket(data []byte) []TradeItem {
	counts := map[string]int{}
	rawByName := map[string]string{}

	// Split on \x02 separator byte.
	fields := bytes.Split(data, []byte{0x02})
	for _, field := range fields {
		if len(field) == 0 {
			continue
		}

		fieldStr := strings.TrimSpace(string(field))
		itemName, ok := extractTradeItemName(fieldStr)
		if !ok {
			continue
		}
		itemName, ok = canonicalAllowedTradeItem(itemName)
		if !ok {
			continue
		}

		counts[itemName]++
		if _, exists := rawByName[itemName]; !exists {
			rawByName[itemName] = fieldStr
		}
	}

	if len(counts) == 0 {
		return []TradeItem{}
	}

	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]TradeItem, 0, len(names))
	for _, name := range names {
		items = append(items, TradeItem{
			Name:     name,
			Quantity: counts[name],
			RawData:  rawByName[name],
		})
	}

	return items
}

func extractTradeItemName(field string) (string, bool) {
	// Legacy format example: "itkoHP|club_sofa"
	if strings.Contains(field, "|") {
		parts := strings.Split(field, "|")
		if len(parts) >= 2 {
			if name, ok := normalizeTradeItemName(parts[len(parts)-1]); ok {
				return name, true
			}
		}
	}

	// Current format example: "irbUAXb{chair_plasty*109"
	if strings.Contains(field, "{") {
		parts := strings.SplitN(field, "{", 2)
		if len(parts) == 2 {
			raw := parts[1]
			star := strings.Index(raw, "*")
			if star < 0 {
				return "", false
			}
			raw = raw[:star]
			if name, ok := normalizeTradeItemName(raw); ok {
				return name, true
			}
		}
	}

	return "", false
}

func normalizeTradeItemName(raw string) (string, bool) {
	name := strings.TrimSpace(strings.ToLower(raw))
	if name == "" || isCoordinatePattern(name) {
		return "", false
	}

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return "", false
	}

	return name, true
}

func canonicalAllowedTradeItem(name string) (string, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "", false
	}

	if canonical, ok := allowedTradeItemAliases[name]; ok {
		return canonical, true
	}

	if strings.HasPrefix(name, "chair_plasty") || strings.HasPrefix(name, "plastic_chair") || strings.HasPrefix(name, "plstic_chair") {
		return "chair_plasty", true
	}

	return "", false
}

// isCoordinatePattern checks if a string looks like coordinates (e.g., "0,0,0")
func isCoordinatePattern(s string) bool {
	if !strings.Contains(s, ",") {
		return false
	}
	
	parts := strings.Split(s, ",")
	for _, part := range parts {
		if _, err := strconv.Atoi(strings.TrimSpace(part)); err != nil {
			return false
		}
	}
	
	return len(parts) >= 2
}

func (a *App) OpenLastTrade() {
	if lastTradePartnerID <= 0 {
		a.AddLogMsg("Open Last Trade failed: no last trader cached yet")
		requestRoomUsers(a)
		return
	}

	ext.Send(out.TRADE_OPEN, lastTradePartnerID)
	outPreview := string(ext.NewPacket(out.TRADE_OPEN, lastTradePartnerID).Data)
	a.AddLogMsg(fmt.Sprintf("Open Last Trade sent -> %s (%d), Outgoing[71] %q", lastTradePartnerName, lastTradePartnerID, outPreview))
}

func (a *App) GetLastTradePartnerName() string {
	if lastTradePartnerID <= 0 {
		return "None"
	}
	if strings.TrimSpace(lastTradePartnerName) == "" {
		return "Unknown"
	}
	return lastTradePartnerName
}

// GetCurrentTradeItems returns the list of items currently in the trade
func (a *App) GetCurrentTradeItems() []TradeItem {
	tradeItemsMu.Lock()
	defer tradeItemsMu.Unlock()
	
	// Return a copy to prevent external modifications
	itemsCopy := make([]TradeItem, len(currentTradeItems))
	copy(itemsCopy, currentTradeItems)
	return itemsCopy
}

// GetTradeItemsJSON returns the current trade items as a JSON string for the frontend
func (a *App) GetTradeItemsJSON() string {
	items := a.GetCurrentTradeItems()
	jsonData, err := json.Marshal(items)
	if err != nil {
		a.AddLogMsg(fmt.Sprintf("[ERROR] failed to marshal trade items: %v", err))
		return "[]"
	}
	return string(jsonData)
}

// ClearTradeItems removes all current trade items
func (a *App) ClearTradeItems() {
	tradeItemsMu.Lock()
	currentTradeItems = []TradeItem{}
	currentOwnTradeItems = []TradeItem{}
	tradeItemsMu.Unlock()
	stopUnderfundedTradeMonitor()
	lastTradeCoverageNotice = ""
	a.AddLogMsg("[TRADE_ITEMS] cleared partner and own trade items")
	log.Printf("[TRADE_ITEMS] cleared partner and own trade items")
	a.emitTradeItemsUpdate("both")
}

// emitTradeItemsUpdate pushes the current trade items to the frontend via an event
func (a *App) emitTradeItemsUpdate(side string) {
	tradeItemsMu.Lock()
	partnerItems := make([]TradeItem, len(currentTradeItems))
	copy(partnerItems, currentTradeItems)
	ownItems := make([]TradeItem, len(currentOwnTradeItems))
	copy(ownItems, currentOwnTradeItems)
	tradeItemsMu.Unlock()

	if side == "partner" || side == "both" {
		jsonData, err := json.Marshal(partnerItems)
		if err == nil {
			runtime.EventsEmit(a.ctx, "tradeItemsUpdate", string(jsonData))
		}
	}

	if side == "yours" || side == "both" {
		jsonData, err := json.Marshal(ownItems)
		if err == nil {
			runtime.EventsEmit(a.ctx, "ownTradeItemsUpdate", string(jsonData))
		}
	}
}

// requestPlayerStrip sends GETSTRIP[65] to refresh the player's hand inventory.
func (a *App) requestPlayerStrip() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[STRIP] GETSTRIP request panicked: %v", r)
		}
	}()

	stripScanMu.Lock()
	stripScanActive = true
	stripScanSeenItemIDs = map[int]struct{}{}
	stripScanCounts = map[string]int{}
	stripScanRawByName = map[string]string{}
	stripScanMu.Unlock()

	a.ext.Send(out.GETSTRIP, "new")
	a.AddLogMsg("[STRIP] requested player hand scan (GETSTRIP new)")
	log.Printf("[STRIP] requested player hand scan (GETSTRIP new)")
}

// handleStripPacket parses STRIPINFO_2 [140] to track items in the player's hand.
func handleStripPacket(a *App, e *g.Intercept) {
	if e.Packet.Header.Dir != g.In || e.Packet.Header.Value != 140 {
		return
	}

	rawData := append([]byte(nil), e.Packet.Data...)

	var inv inventory.Inventory
	e.Packet.Read(&inv)

	stripScanMu.Lock()
	active := stripScanActive
	if !active {
		stripScanActive = true
		stripScanSeenItemIDs = map[int]struct{}{}
		stripScanCounts = map[string]int{}
		stripScanRawByName = map[string]string{}
		active = true
	}
	wrapped := accumulateStripScan(inv.Items)
	pageCount := len(inv.Items)
	lastPage := pageCount < 9

	if wrapped || lastPage {
		items := buildStripScanItems()
		stripScanActive = false
		stripScanMu.Unlock()

		rawItems := parseStripItemsPacketRaw(rawData)
		if len(items) == 0 && len(rawItems) > 0 {
			items = rawItems
			a.AddLogMsg("[STRIP] scan decode had no matches; raw fallback parser recovered hand items")
			log.Printf("[STRIP] scan decode had no matches; raw fallback parser recovered hand items")
		} else if len(items) > 0 && len(rawItems) > 0 {
			items = mergePreferHigherQuantity(items, rawItems)
		}

		handItemsMu.Lock()
		currentHandItems = items
		handItemsMu.Unlock()

		a.AddLogMsg(fmt.Sprintf("[STRIP] scan complete (%d page item(s)); hand item types=%d", pageCount, len(items)))
		log.Printf("[STRIP] scan complete (%d page item(s)); hand item types=%d", pageCount, len(items))
		for i, item := range items {
			a.AddLogMsg(fmt.Sprintf("[STRIP] hand[%d] name=%q qty=%d", i, item.Name, item.Quantity))
			log.Printf("[STRIP] hand[%d] name=%q qty=%d", i, item.Name, item.Quantity)
		}
		a.emitHandItemsUpdate()
		return
	}

	stripScanMu.Unlock()

	go func() {
		time.Sleep(550 * time.Millisecond)
		a.ext.Send(out.GETSTRIP, "next")
	}()

	a.AddLogMsg(fmt.Sprintf("[STRIP] continuing scan: page had %d item(s), requesting next", pageCount))
	log.Printf("[STRIP] continuing scan: page had %d item(s), requesting next", pageCount)
	return
}

func accumulateStripScan(invItems []inventory.Item) (wrapped bool) {
	for _, invItem := range invItems {
		if _, exists := stripScanSeenItemIDs[invItem.ItemId]; exists {
			return true
		}
		stripScanSeenItemIDs[invItem.ItemId] = struct{}{}

		if strings.TrimSpace(invItem.Class) == "" {
			continue
		}

		itemName, qty, ok := normalizeCatalogClassWithQuantity(invItem.Class)
		if !ok {
			continue
		}

		stripScanCounts[itemName] += qty
		if _, exists := stripScanRawByName[itemName]; !exists {
			stripScanRawByName[itemName] = invItem.String()
		}
	}

	return false
}

func buildStripScanItems() []TradeItem {
	if len(stripScanCounts) == 0 {
		return []TradeItem{}
	}

	names := make([]string, 0, len(stripScanCounts))
	for name := range stripScanCounts {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]TradeItem, 0, len(names))
	for _, name := range names {
		items = append(items, TradeItem{
			Name:     name,
			Quantity: stripScanCounts[name],
			RawData:  stripScanRawByName[name],
		})
	}

	return items
}

// parseStripItemsPacketRaw is a fallback parser for STRIPINFO_2 payloads that extracts
// class names from raw fields and counts occurrences.
func parseStripItemsPacketRaw(data []byte) []TradeItem {
	counts := map[string]int{}
	rawByName := map[string]string{}

	fields := bytes.Split(data, []byte{0x02})
	for i, field := range fields {
		if len(field) == 0 {
			continue
		}

		fieldStr := string(field)
		itemName, qty, ok := extractStripItemAndQuantity(fieldStr)
		if !ok {
			continue
		}

		if qty <= 1 && i > 0 {
			if inferred, ok := inferStackCountFromMetaField(string(fields[i-1])); ok {
				qty = inferred
			}
		}

		counts[itemName] += qty
		if _, exists := rawByName[itemName]; !exists {
			rawByName[itemName] = fieldStr
		}
	}

	if len(counts) == 0 {
		return []TradeItem{}
	}

	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]TradeItem, 0, len(names))
	for _, name := range names {
		items = append(items, TradeItem{
			Name:     name,
			Quantity: counts[name],
			RawData:  rawByName[name],
		})
	}

	return items
}

// diffItems returns the items in `all` that exceed the quantities in `subtract`.
// Used to compute one trader's items from the combined TRADE_ITEMS packet.
func diffItems(all []TradeItem, subtract []TradeItem) []TradeItem {
	subtractQty := make(map[string]int, len(subtract))
	for _, item := range subtract {
		subtractQty[item.Name] += item.Quantity
	}

	allQty := make(map[string]int, len(all))
	rawByName := make(map[string]string, len(all))
	names := make([]string, 0, len(all))
	for _, item := range all {
		if allQty[item.Name] == 0 {
			names = append(names, item.Name)
		}
		allQty[item.Name] += item.Quantity
		if rawByName[item.Name] == "" {
			rawByName[item.Name] = item.RawData
		}
	}
	sort.Strings(names)

	result := make([]TradeItem, 0)
	for _, name := range names {
		remaining := allQty[name] - subtractQty[name]
		if remaining > 0 {
			result = append(result, TradeItem{
				Name:     name,
				Quantity: remaining,
				RawData:  rawByName[name],
			})
		}
	}
	return result
}

func mergePreferHigherQuantity(base []TradeItem, candidate []TradeItem) []TradeItem {
	byName := make(map[string]TradeItem, len(base))
	for _, item := range base {
		byName[item.Name] = item
	}

	for _, item := range candidate {
		existing, ok := byName[item.Name]
		if !ok || item.Quantity > existing.Quantity {
			byName[item.Name] = item
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	merged := make([]TradeItem, 0, len(names))
	for _, name := range names {
		merged = append(merged, byName[name])
	}

	return merged
}

func inferStackCountFromMetaField(meta string) (int, bool) {
	// In observed STRIPINFO_2 metadata, stacked furni count correlates with
	// repeated "bUA" segments in the metadata field directly before class name.
	// Example:
	//   1x -> "nxbUAHJS"           (1 occurrence)
	//   2x -> "nxbUAIntbUAJS"      (2 occurrences)
	//   3x -> "nxbUAJntbUAmrbUAJS" (3 occurrences)
	if strings.Contains(meta, "bUA") {
		c := strings.Count(meta, "bUA")
		if c >= 1 && c <= 50 {
			return c, true
		}
	}

	// Avoid VL64-based guessing for non-bUA metadata because it can overcount
	// when unrelated items are present on the same page.
	return 0, false
}

func normalizeCatalogClassWithQuantity(raw string) (name string, qty int, ok bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", 0, false
	}

	qty = 1
	if star := strings.LastIndex(raw, "*"); star > 0 && star < len(raw)-1 {
		suffix := raw[star+1:]
		if n, err := strconv.Atoi(suffix); err == nil {
			// In strip payloads a small suffix can represent a stack amount; larger values are typically ids.
			if n >= 2 && n <= 50 {
				qty = n
			}
			raw = raw[:star]
		}
	}

	name, ok = normalizeTradeItemName(raw)
	if !ok {
		return "", 0, false
	}

	name, ok = canonicalAllowedTradeItem(name)
	if !ok {
		return "", 0, false
	}

	return name, qty, true
}

// extractStripItemName extracts the furniture class name from a STRIPINFO_2 field.
func extractStripItemName(field string) (string, bool) {
	name, _, ok := extractStripItemAndQuantity(field)
	return name, ok
}

func extractStripItemAndQuantity(field string) (string, int, bool) {
	// Try existing trade formats first (handles | and { delimiters).
	if name, ok := extractTradeItemName(field); ok {
		name, ok = canonicalAllowedTradeItem(name)
		if !ok {
			return "", 0, false
		}
		return name, 1, true
	}

	matches := stripItemNameRe.FindAllString(field, -1)
	if len(matches) == 0 {
		return "", 0, false
	}

	best := ""
	for _, m := range matches {
		if len(m) > len(best) {
			best = m
		}
	}

	if star := strings.LastIndex(best, "*"); star > 0 {
		// Keep suffix handling in one place.
	}

	name, qty, ok := normalizeCatalogClassWithQuantity(best)
	if !ok {
		return "", 0, false
	}

	return name, qty, true
}

// emitHandItemsUpdate pushes the player's current hand items to the frontend.
func (a *App) emitHandItemsUpdate() {
	handItemsMu.Lock()
	items := make([]TradeItem, len(currentHandItems))
	copy(items, currentHandItems)
	handItemsMu.Unlock()

	jsonData, err := json.Marshal(items)
	if err != nil {
		return
	}
	runtime.EventsEmit(a.ctx, "handItemsUpdate", string(jsonData))
}

func (a *App) notifyTradeQuantityCoverage() {
	shortages := a.getTradeCoverageShortages()
	if len(shortages) == 0 {
		stopUnderfundedTradeMonitor()
		lastTradeCoverageNotice = ""
		lastTradeBlockNotice = ""
		a.AddLogMsg("[TRADE_COVERAGE] hand has enough stock to pay double (bet + match)")
		log.Printf("[TRADE_COVERAGE] hand has enough stock to pay double (bet + match)")
		return
	}

	notice := formatTradeShortages(shortages)
	if notice == lastTradeCoverageNotice {
		return
	}
	lastTradeCoverageNotice = notice

	primary := shortages[0]
	msg := fmt.Sprintf("Total \"%s\" available \"%d\": please offer less.", formatTradeItemName(primary.Name), primary.Have)
	a.AddLogMsg("[TRADE_COVERAGE] " + msg)
	log.Printf("[TRADE_COVERAGE] %s", msg)
	ext.Send(out.SHOUT, msg)

	if notice != underfundedTradeMonitorNotice {
		startUnderfundedTradeMonitor(a, notice)
	}
}

func (a *App) getTradeCoverageShortages() []tradeShortage {
	tradeItemsMu.Lock()
	partnerItems := make([]TradeItem, len(currentTradeItems))
	copy(partnerItems, currentTradeItems)
	tradeItemsMu.Unlock()

	handItemsMu.Lock()
	handItems := make([]TradeItem, len(currentHandItems))
	copy(handItems, currentHandItems)
	handItemsMu.Unlock()

	if len(partnerItems) == 0 {
		return nil
	}

	haveByName := map[string]int{}
	for _, item := range handItems {
		haveByName[item.Name] += item.Quantity
	}

	shortages := make([]tradeShortage, 0)
	for _, item := range partnerItems {
		required := item.Quantity
		payoutTotal := item.Quantity * 2
		have := haveByName[item.Name]
		if have < required {
			shortages = append(shortages, tradeShortage{
				Name: item.Name,
				Required: required,
				Have: have,
				PayoutTotal: payoutTotal,
			})
		}
	}
	return shortages
}

func formatTradeShortages(shortages []tradeShortage) string {
	parts := make([]string, 0, len(shortages))
	for _, shortage := range shortages {
		parts = append(parts, fmt.Sprintf("%s available %d need %d (payout %d)", formatTradeItemName(shortage.Name), shortage.Have, shortage.Required, shortage.PayoutTotal))
	}
	return strings.Join(parts, ", ")
}

// sendTradeCompletionMessage sends the post-trade game prompt sequence.
func (a *App) sendTradeCompletionMessage() {
	items := a.GetCurrentTradeItems()
	
	if len(items) == 0 {
		a.AddLogMsg("[TRADE_MESSAGE] no items in trade, skipping message")
		log.Printf("[TRADE_MESSAGE] no items in trade, skipping message")
		return
	}

	partnerName := strings.TrimSpace(lastTradePartnerName)
	if partnerName == "" {
		partnerName = "Player"
	}

	first := fmt.Sprintf("%s what game do you want to play?", partnerName)
	second := "Say Poker, 21, 13"

	a.AddLogMsg(fmt.Sprintf("[TRADE_MESSAGE] shouting: %q", first))
	log.Printf("[TRADE_MESSAGE] shouting: %q", first)
	ext.Send(out.SHOUT, first)

	go func(msg string) {
		time.Sleep(700 * time.Millisecond)
		a.AddLogMsg(fmt.Sprintf("[TRADE_MESSAGE] shouting: %q", msg))
		log.Printf("[TRADE_MESSAGE] shouting: %q", msg)
		ext.Send(out.SHOUT, msg)
	}(second)
}

func formatTradeItemName(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

type user28Entry struct {
	Token string
	Name  string
}

func extractUsers28Entries(raw string) []user28Entry {
	parts := strings.Split(raw, "\x02")
	entries := make([]user28Entry, 0)

	for _, part := range parts {
		name, token, ok := parseUsers28Head(part)
		if !ok {
			continue
		}
		entries = append(entries, user28Entry{Token: token, Name: name})
	}

	return entries
}

func parseUsers28Head(part string) (name string, token string, ok bool) {
	if len(part) < 7 {
		return "", "", false
	}

	nameStart := len(part)
	for nameStart > 0 && isLikelyNameChar(part[nameStart-1]) {
		nameStart--
	}

	if nameStart < 5 || nameStart >= len(part) {
		return "", "", false
	}

	name = part[nameStart:]
	if len(name) < 2 {
		return "", "", false
	}

	token = part[nameStart-4 : nameStart]
	if !isLikelyToken(token) {
		return "", "", false
	}

	return name, token, true
}

func isLikelyNameChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

func isLikelyToken(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] > 126 {
			return false
		}
	}
	return true
}

func (a *App) handleRoomReady(e *g.Intercept) {
	roomMu.Lock()
	defer roomMu.Unlock()
	clear(roomEntities)
	a.AddLogMsg("[ROOM_USERS] cleared cached room users")
	log.Printf("[ROOM_USERS] cleared cached room users")
	go requestRoomUsers(a)
}

func (a *App) handleRoomUsers(e *g.Intercept) {
	defer func() {
		if recover() != nil {
			a.AddLogMsg(fmt.Sprintf("[ROOM_USERS] failed to parse packet %d", e.Packet.Header.Value))
			log.Printf("[ROOM_USERS] failed to parse packet %d", e.Packet.Header.Value)
		}
	}()

	a.AddLogMsg(fmt.Sprintf("[ROOM_USERS] received packet %d len=%d", e.Packet.Header.Value, len(e.Packet.Data)))
	log.Printf("[ROOM_USERS] received packet %d len=%d", e.Packet.Header.Value, len(e.Packet.Data))

	count := e.Packet.ReadInt()

	roomMu.Lock()

	for range count {
		var entity room.Entity
		e.Packet.Read(&entity)
		if entity.Type == room.User {
			roomEntities[entity.Index] = entity
		}
	}
	roomMu.Unlock()

	for _, line := range summarizeRoomUsers() {
		a.AddLogMsg("[ROOM_USERS] " + line)
		log.Printf("[ROOM_USERS] %s", line)
	}
}

func requestRoomUsers(a *App) {
	defer func() {
		if recover() != nil {
			a.AddLogMsg("[ROOM_USERS] request failed")
			log.Printf("[ROOM_USERS] request failed")
		}
	}()

	// G_USRS is the packet this client uses to request the in-room USERS list (header 61).
	a.ext.Send(out.G_USRS)
	// Keep legacy request as a secondary path in case the server expects both in some sessions.
	a.ext.Send(out.GETSPACENODEUSERS)
	startIncomingHeaderSniff(8 * time.Second)
	a.AddLogMsg("[ROOM_USERS] requested current room users via G_USRS + GETSPACENODEUSERS")
	log.Printf("[ROOM_USERS] requested current room users via G_USRS + GETSPACENODEUSERS")
}

func startIncomingHeaderSniff(duration time.Duration) {
	headerSniffMu.Lock()
	defer headerSniffMu.Unlock()
	headerSniffUntil = time.Now().Add(duration)
	headerSniffSeen = map[uint16]bool{}
}

func handleIncomingHeaderSniff(a *App, e *g.Intercept) {
	if e.Packet.Header.Dir != g.In {
		return
	}

	headerSniffMu.Lock()
	active := time.Now().Before(headerSniffUntil)
	if !active {
		headerSniffMu.Unlock()
		return
	}

	header := e.Packet.Header.Value
	if headerSniffSeen[header] {
		headerSniffMu.Unlock()
		return
	}
	headerSniffSeen[header] = true
	headerSniffMu.Unlock()

	preview := string(e.Packet.Data)
	if len(preview) > 32 {
		preview = preview[:32]
	}
	name := ext.Headers().Name(e.Packet.Header)
	a.AddLogMsg(fmt.Sprintf("[HEADER_SNIFF] incoming[%d:%s] len=%d preview=%q", header, name, len(e.Packet.Data), preview))
	log.Printf("[HEADER_SNIFF] incoming[%d:%s] len=%d preview=%q", header, name, len(e.Packet.Data), preview)
}

func summarizeRoomUsers() []string {
	roomMu.Lock()
	defer roomMu.Unlock()

	if len(roomEntities) == 0 {
		return []string{"no cached room users"}
	}

	indices := make([]int, 0, len(roomEntities))
	for index := range roomEntities {
		indices = append(indices, index)
	}
	sort.Ints(indices)

	entries := make([]string, 0, len(indices))
	for _, index := range indices {
		entity := roomEntities[index]
		_, cleanedName, ok := splitTokenAndName(entity.Name)
		if ok {
			entries = append(entries, fmt.Sprintf("%s(%d)", cleanedName, entity.Index))
		} else {
			entries = append(entries, fmt.Sprintf("%s(%d)", entity.Name, entity.Index))
		}
	}

	return []string{fmt.Sprintf("cached %d room user(s): %s", len(entries), strings.Join(entries, ", "))}
}

func describeTradeRoomCandidates(ext *g.Ext) []string {
	roomMu.Lock()
	defer roomMu.Unlock()

	if len(roomEntities) == 0 {
		return []string{"no cached room users"}
	}

	indices := make([]int, 0, len(roomEntities))
	for index := range roomEntities {
		indices = append(indices, index)
	}
	sort.Ints(indices)

	lines := make([]string, 0, len(indices))
	for _, index := range indices {
		entity := roomEntities[index]
		token, cleanName, hasToken := splitTokenAndName(entity.Name)
		displayName := entity.Name
		if hasToken {
			displayName = cleanName
			users28Mu.Lock()
			users28ByToken[token] = cleanName
			users28Mu.Unlock()
		}
		libraryPayload := string(ext.NewPacket(out.TRADE_OPEN, index).Data)
		lines = append(lines, fmt.Sprintf(
			"name=%q index=%d candidates{library_int=%q vl64=%q b64_2=%q b64_3=%q}",
			displayName,
			entity.Index,
			libraryPayload,
			encodeVL64(entity.Index),
			encodeB64(entity.Index, 2),
			encodeB64(entity.Index, 3),
		))
	}

	return lines
}

func splitTokenAndName(s string) (token string, name string, ok bool) {
	if len(s) < 6 {
		return "", "", false
	}
	token = s[:4]
	name = s[4:]
	if !isLikelyToken(token) || len(name) < 2 {
		return "", "", false
	}
	for i := 0; i < len(name); i++ {
		if !isLikelyNameChar(name[i]) && name[i] != ' ' {
			return "", "", false
		}
	}
	return token, name, true
}

func resolveTradeTokenToRoomIndex(token string) (index int, name string, ok bool) {
	roomMu.Lock()
	defer roomMu.Unlock()

	for _, entity := range roomEntities {
		entityToken, cleanName, hasToken := splitTokenAndName(entity.Name)
		if hasToken && entityToken == token {
			return entity.Index, cleanName, true
		}
	}

	return 0, "", false
}

func encodeVL64(value int) string {
	buf := make([]byte, gencoding.VL64EncodeLen(value))
	gencoding.VL64Encode(buf, value)
	return string(buf)
}

func encodeB64(value int, length int) string {
	buf := make([]byte, length)
	gencoding.B64Encode(buf, value)
	return string(buf)
}

func tryReadInt(pkt *g.Packet) (value int, pos int, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	value = pkt.ReadInt()
	pos = pkt.Pos
	return value, pos, true
}

func tryReadString(pkt *g.Packet) (value string, pos int, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	value = pkt.ReadString()
	pos = pkt.Pos
	return value, pos, true
}

func tryReadIntString(pkt *g.Packet) (id int, text string, pos int, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	id = pkt.ReadInt()
	text = pkt.ReadString()
	pos = pkt.Pos
	return id, text, pos, true
}

func tryReadIntInt(pkt *g.Packet) (a int, b int, pos int, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	a = pkt.ReadInt()
	b = pkt.ReadInt()
	pos = pkt.Pos
	return a, b, pos, true
}

func (a *App) onChatMessage(e *g.Intercept) {
	msg := e.Packet.ReadString()
	a.AddChatLog("[OUT] " + msg)

	// Process commands based on the message prefix and suffix
	if strings.HasPrefix(msg, ":") {
		// Check if already rolling or closing
		if isPokerRolling || isTriRolling || isBJRolling || is13Rolling || isHitting || is13Hitting || isClosing {
			log.Println("Already rolling or closing...")
			e.Block()
			return
		}

		command := strings.TrimPrefix(msg, ":")
		switch {
		case strings.HasSuffix(command, "reset"):
			e.Block()
			resetDiceState()
		case strings.HasSuffix(command, "roll"):
			e.Block()
			a.startPokerRoll()
		case strings.HasSuffix(command, "tri"):
			e.Block()
			isTriRolling = true
			logRollResult := fmt.Sprint("Tri Roll:\n")
			a.AddLogMsg(logRollResult)
			go a.rollTriDice()
		case strings.HasSuffix(command, "close"):
			e.Block()
			go a.closeAllDice()
		case strings.HasSuffix(command, "21"):
			e.Block()
			isBJRolling = true
			logRollResult := fmt.Sprintf("21 Roll:\n")
			a.AddLogMsg(logRollResult)
			go a.rollBjDice()
		case strings.HasSuffix(command, "13"):
			e.Block()
			is13Rolling = true
			logRollResult := fmt.Sprintf("13 Roll:\n")
			a.AddLogMsg(logRollResult)
			go a.roll13Dice()
		case strings.HasPrefix(command, "@"):
			e.Block()
			extra := strings.TrimSpace(strings.TrimPrefix(command, "@"))
			go a.evalAt(extra)
		case strings.HasSuffix(command, "verify"):
			e.Block()
			go verifyResult()
		case strings.HasSuffix(command, "commands"):
			e.Block()
			go a.ShowCommands()
		case strings.HasSuffix(command, "chaton"):
			e.Block()
			ChatIsDisabled = false
		case strings.HasSuffix(command, "chatoff"):
			e.Block()
			ChatIsDisabled = true
		}
	}
}

func (a *App) evalAt(msg string) {
	mutex.Lock()
	at := "@" + msg
	ext.Send(out.SHOUT, at)
	a.AddLogMsg(at)
	mutex.Unlock()
}

func (a *App) startPokerRoll() {
	isPokerRolling = true
	logRollResult := fmt.Sprintf("Poker Roll:\n")
	a.AddLogMsg(logRollResult)
	go a.rollPokerDice()
}

// Reset all saved dice states
func resetDiceState() {
	mutex.Lock()
	defer mutex.Unlock()
	resultsWaitGroup.Wait() // Ensure all dice roll results are processed
	diceList = []*Dice{}
	awaitingTradeOpen = false
	fakeDiceTestingMode = false
	isPokerRolling, isTriRolling, isBJRolling, is13Rolling, isHitting, is13Hitting, isClosing = false, false, false, false, false, false, false
}

func rememberDiceID(diceID int) {
	if diceID <= 0 {
		return
	}
	knownDiceIDs[diceID] = struct{}{}
}

func (a *App) SkipDiceSetupForTesting() {
	mutex.Lock()
	defer mutex.Unlock()

	diceList = make([]*Dice, 0, 5)
	for i := 1; i <= 5; i++ {
		diceList = append(diceList, &Dice{ID: 100000 + i, Value: rand.Intn(6) + 1, IsRolling: false, IsClosed: false})
	}
	fakeDiceTestingMode = true
	awaitingTradeOpen = true
	a.AddLogMsg("Dice setup bypass enabled for testing. Using 5 fake dice values.")
}

func (a *App) handleThrowDice(e *g.Intercept) {
	packet := e.Packet
	rawData := string(packet.Data)
	logrus.WithFields(logrus.Fields{"raw_data": rawData}).Debug("Raw packet data")

	diceData := strings.Fields(rawData)
	diceIDStr := diceData[0]
	diceID, err := strconv.Atoi(diceIDStr)
	if err != nil {
		logrus.WithFields(logrus.Fields{"dice_id_str": diceIDStr, "error": err}).Warn("Failed to parse dice ID")
		return
	}
	rememberDiceID(diceID)

	mutex.Lock()
	defer mutex.Unlock()

	// Search for a dice with the given ID in the list
	var existingDice *Dice
	for _, dice := range diceList {
		if dice != nil && dice.ID == diceID {
			existingDice = dice
			break
		}
	}

	// If not found and the list has fewer than 5 dice, create and add a new one
	if existingDice == nil && len(diceList) < 5 {
		newDice := &Dice{ID: diceID, IsRolling: true, IsClosed: false}
		diceList = append(diceList, newDice)
		log.Printf("Dice %d added\n", diceID)

		if len(diceList) == 5 {
			message := "Dice setup sucessful! Run :roll to confirm"
			a.AddLogMsg(message)
			go requestRoomUsers(a)
			awaitingTradeOpen = true
			if !isMuted {
				go sendMessageWithDelay(a.dealerOpenMessage())
			} else {
				log.Printf("User is muted. Skipping dealer open prompt message.")
			}
		}
	}
}

// handle the turning off of a dice
func (a *App) handleDiceOff(e *g.Intercept) {
	packet := e.Packet
	diceIDStr := string(packet.Data)

	diceID, err := strconv.Atoi(diceIDStr)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"dice_id_str": diceIDStr,
			"error":       err,
		}).Warn("Failed to parse dice ID")
		return
	}
	rememberDiceID(diceID)

	mutex.Lock()
	defer mutex.Unlock()

	// Search for a dice with the given ID in the list
	var existingDice *Dice
	for _, dice := range diceList {
		if dice != nil && dice.ID == diceID {
			existingDice = dice
			break
		}
	}

	// If not found and the list has fewer than 5 dice, create and add a new one
	if existingDice == nil && len(diceList) < 5 {
		newDice := &Dice{ID: diceID, IsRolling: false, IsClosed: true}
		diceList = append(diceList, newDice)
		log.Printf("Dice %d added\n", diceID)
	}
}

// Handle the result of a dice roll
func (a *App) handleDiceResult(e *g.Intercept) {
	packet := e.Packet
	rawData := string(packet.Data)
	logrus.WithFields(logrus.Fields{"raw_data": rawData}).Debug("Raw packet data")

	diceData := strings.Fields(rawData)
	if len(diceData) < 2 {
		return
	}

	diceIDStr := diceData[0]
	diceID, err := strconv.Atoi(diceIDStr)
	if err != nil {
		logrus.WithFields(logrus.Fields{"dice_id_str": diceIDStr, "error": err}).Warn("Failed to parse dice ID")
		return
	}
	rememberDiceID(diceID)

	diceValueStr := diceData[1]
	diceValue, err := strconv.Atoi(diceValueStr)
	if err != nil {
		logrus.WithFields(logrus.Fields{"dice_value_str": diceValueStr, "error": err}).Warn("Failed to parse dice value")
		return
	}
	adjustedDiceValue := diceValue - (diceID * 38)

	mutex.Lock()
	for i, dice := range diceList {
		if dice.ID == diceID {
			if dice.IsRolling && (isPokerRolling || isTriRolling || isBJRolling || is13Rolling || is13Hitting || isHitting) {
				dice.IsRolling = false
				resultsWaitGroup.Done()
			}
			diceList[i].Value = adjustedDiceValue
			diceList[i].IsClosed = diceList[i].Value == 0

			if isPokerRolling || isTriRolling || isBJRolling || is13Rolling || is13Hitting || isHitting {
				log.Printf("Dice %d rolled: %d\n", diceID, adjustedDiceValue)
				logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceID, adjustedDiceValue)
				a.AddLogMsg(logRollResult)
			}
			break
		}
	}
	mutex.Unlock()
}

// Close the dice and send the packets to the game server
func (a *App) closeAllDice() {
	if fakeDiceTestingMode {
		mutex.Lock()
		isClosing = true
		for _, dice := range diceList {
			dice.IsClosed = true
			dice.Value = 0
		}
		isClosing = false
		mutex.Unlock()
		return
	}

	mutex.Lock()
	isClosing = true
	mutex.Unlock()

	for _, dice := range diceList {
		dice.Close()

		// random delay between 550 and 600ms
		time.Sleep(rollDelay + time.Duration(rand.Intn(50))*time.Millisecond)
	}
	mutex.Lock()
	isClosing = false
	mutex.Unlock()
}

// Roll the poker dice by sending packets and waiting for results
func (a *App) rollPokerDice() {
	if fakeDiceTestingMode {
		mutex.Lock()
		if len(diceList) < 5 {
			mutex.Unlock()
			log.Println("Not enough dice to roll")
			isPokerRolling = false
			return
		}
		for i := range diceList {
			diceList[i].Value = rand.Intn(6) + 1
			diceList[i].IsClosed = false
			logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceList[i].ID, diceList[i].Value)
			a.AddLogMsg(logRollResult)
		}
		mutex.Unlock()
		a.evaluatePokerHand()
		isPokerRolling = false
		return
	}

	mutex.Lock()

	if len(diceList) < 5 {
		mutex.Unlock()
		log.Println("Not enough dice to roll")
		isPokerRolling = false
		return
	}

	resultsWaitGroup.Add(len(diceList))
	mutex.Unlock()

	for _, dice := range diceList {
		dice.Roll()

		// random delay between 550 and 600ms
		time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
	}

	time.Sleep(1000 * time.Millisecond)
	resultsWaitGroup.Wait()
	a.evaluatePokerHand()
	isPokerRolling = false
}

// Evaluate the poker hand and send the result to the chat
func (a *App) rollTriDice() {
	if fakeDiceTestingMode {
		mutex.Lock()
		if len(diceList) < 5 {
			mutex.Unlock()
			log.Println("Not enough dice to roll")
			isTriRolling = false
			return
		}
		for _, index := range []int{0, 2, 4} {
			diceList[index].Value = rand.Intn(6) + 1
			diceList[index].IsClosed = false
			logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceList[index].ID, diceList[index].Value)
			a.AddLogMsg(logRollResult)
		}
		mutex.Unlock()
		a.evaluateTriHand()
		isTriRolling = false
		return
	}

	mutex.Lock()

	if len(diceList) < 5 {
		mutex.Unlock()
		log.Println("Not enough dice to roll")
		isTriRolling = false
		return
	}

	resultsWaitGroup.Add(3)
	mutex.Unlock()

	for _, index := range []int{0, 2, 4} {
		diceList[index].Roll()
		time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
	}

	time.Sleep(1000 * time.Millisecond)
	resultsWaitGroup.Wait()

	a.evaluateTriHand()
	isTriRolling = false
}

// Roll dice for blackjack-style game
func (a *App) rollBjDice() {
	if fakeDiceTestingMode {
		mutex.Lock()
		if len(diceList) < 5 {
			mutex.Unlock()
			log.Println("Not enough dice to roll")
			isBJRolling = false
			return
		}
		currentSum = 0
		for _, index := range []int{0, 1, 2} {
			diceList[index].Value = rand.Intn(6) + 1
			diceList[index].IsClosed = false
			currentSum += diceList[index].Value
			logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceList[index].ID, diceList[index].Value)
			a.AddLogMsg(logRollResult)
		}
		mutex.Unlock()
		a.evaluateBlackjackHand()
		isBJRolling = false
		return
	}

	go a.closeAllDice()
	time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
	mutex.Lock()

	if len(diceList) < 5 {
		mutex.Unlock()
		log.Println("Not enough dice to roll")
		isBJRolling = false
		return
	}

	currentSum = 0 // Reset sum before starting
	resultsWaitGroup.Add(3)
	mutex.Unlock()

	// Roll the first three dice in order
	for _, index := range []int{0, 1, 2} {
		diceList[index].Roll()
		time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
	}

	time.Sleep(1000 * time.Millisecond)
	resultsWaitGroup.Wait()

	mutex.Lock()
	for _, index := range []int{0, 1, 2} {
		currentSum += diceList[index].Value
	}
	mutex.Unlock()

	a.evaluateBlackjackHand()
	isBJRolling = false
}

func (a *App) hitBjDice() {
	if fakeDiceTestingMode {
		mutex.Lock()
		if len(diceList) < 5 {
			mutex.Unlock()
			log.Println("Not enough dice to roll")
			isBJRolling = false
			isHitting = false
			return
		}

		rolled := false
		for i := 3; i < 5; i++ {
			if diceList[i].Value == 0 {
				diceList[i].Value = rand.Intn(6) + 1
				diceList[i].IsClosed = false
				currentSum += diceList[i].Value
				rolled = true
				logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceList[i].ID, diceList[i].Value)
				a.AddLogMsg(logRollResult)
				break
			}
		}

		if !rolled {
			diceList[4].Value = rand.Intn(6) + 1
			diceList[4].IsClosed = false
			currentSum += diceList[4].Value
			logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceList[4].ID, diceList[4].Value)
			a.AddLogMsg(logRollResult)
		}
		mutex.Unlock()

		a.evaluateBlackjackHand()
		isHitting = false
		isBJRolling = false
		return
	}

	mutex.Lock()

	if len(diceList) < 5 {
		mutex.Unlock()
		log.Println("Not enough dice to roll")
		isBJRolling = false
		isHitting = false
		return
	}

	resultsWaitGroup.Add(1)
	mutex.Unlock()

	for i := 3; i < 5; i++ { // Start from index 3 to roll the next available dice
		if diceList[i].Value == 0 {
			diceList[i].Roll()
			time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
			resultsWaitGroup.Wait()

			mutex.Lock()
			currentSum += diceList[i].Value // Add value to current sum
			mutex.Unlock()

			// Re-evaluate the hand after hitting
			a.evaluateBlackjackHand()

			isBJRolling = false
			isHitting = false
			return
		}
	}

	// If all dice have been rolled, re-roll the last one
	oldValue := diceList[4].Value
	// sleep random between 1000 and 1500ms
	time.Sleep(time.Duration(rand.Intn(1000)+500) * time.Millisecond)
	diceList[4].Roll()

	time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
	resultsWaitGroup.Wait()
	newValue := diceList[4].Value

	mutex.Lock()
	currentSum = currentSum + newValue // Adjust current sum
	mutex.Unlock()

	// Log the value of the dice rolled
	log.Printf("Hit: Re-rolled dice %d = %d (old value was %d)\n", diceList[4].ID, newValue, oldValue)

	// Re-evaluate the hand with the updated sum
	a.evaluateBlackjackHand()

	isHitting = false
	isBJRolling = false
}

// Roll dice for blackjack-style game
func (a *App) roll13Dice() {
	if fakeDiceTestingMode {
		mutex.Lock()
		if len(diceList) < 5 {
			mutex.Unlock()
			log.Println("Not enough dice to roll")
			is13Rolling = false
			return
		}
		currentSum = 0
		for _, index := range []int{0, 1} {
			diceList[index].Value = rand.Intn(6) + 1
			diceList[index].IsClosed = false
			currentSum += diceList[index].Value
			logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceList[index].ID, diceList[index].Value)
			a.AddLogMsg(logRollResult)
		}
		mutex.Unlock()
		a.evaluate13Hand()
		is13Rolling = false
		return
	}

	go a.closeAllDice()
	time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
	mutex.Lock()

	if len(diceList) < 5 {
		mutex.Unlock()
		log.Println("Not enough dice to roll")
		is13Rolling = false
		return
	}

	currentSum = 0 // Reset sum before starting
	resultsWaitGroup.Add(2)
	mutex.Unlock()

	// Roll the first three dice in order
	for _, index := range []int{0, 1} {
		diceList[index].Roll()
		time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
	}

	time.Sleep(1000 * time.Millisecond)
	resultsWaitGroup.Wait()

	mutex.Lock()
	for _, index := range []int{0, 1} {
		currentSum += diceList[index].Value
	}
	mutex.Unlock()

	a.evaluate13Hand()
	is13Rolling = false
}

func (a *App) hit13Dice() {
	if fakeDiceTestingMode {
		mutex.Lock()
		if len(diceList) < 5 {
			mutex.Unlock()
			log.Println("Not enough dice to roll")
			is13Rolling = false
			is13Hitting = false
			return
		}

		rolled := false
		for i := 2; i < 5; i++ {
			if diceList[i].Value == 0 {
				diceList[i].Value = rand.Intn(6) + 1
				diceList[i].IsClosed = false
				currentSum += diceList[i].Value
				rolled = true
				logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceList[i].ID, diceList[i].Value)
				a.AddLogMsg(logRollResult)
				break
			}
		}

		if !rolled {
			diceList[4].Value = rand.Intn(6) + 1
			diceList[4].IsClosed = false
			currentSum += diceList[4].Value
			logRollResult := fmt.Sprintf("Dice %d rolled: %d\n", diceList[4].ID, diceList[4].Value)
			a.AddLogMsg(logRollResult)
		}
		mutex.Unlock()

		a.evaluate13Hand()
		is13Hitting = false
		is13Rolling = false
		return
	}

	mutex.Lock()

	if len(diceList) < 5 {
		mutex.Unlock()
		log.Println("Not enough dice to roll")
		is13Rolling = false
		is13Hitting = false
		return
	}

	resultsWaitGroup.Add(1)
	mutex.Unlock()

	for i := 2; i < 5; i++ { // Start from index 2 to roll the next available dice
		if diceList[i].Value == 0 {
			diceList[i].Roll()
			time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
			resultsWaitGroup.Wait()

			mutex.Lock()
			currentSum += diceList[i].Value // Add value to current sum
			mutex.Unlock()

			// Re-evaluate the hand after hitting
			a.evaluate13Hand()

			is13Rolling = false
			is13Hitting = false
			return
		}
	}

	// If all dice have been rolled, re-roll the last one
	oldValue := diceList[4].Value
	// sleep random between 1000 and 1500ms
	time.Sleep(time.Duration(rand.Intn(1000)+500) * time.Millisecond)
	diceList[4].Roll()

	time.Sleep(rollDelay + time.Duration(rand.Intn(100))*time.Millisecond)
	resultsWaitGroup.Wait()
	newValue := diceList[4].Value

	mutex.Lock()
	currentSum = currentSum + newValue // Adjust current sum
	mutex.Unlock()

	// Log the value of the dice rolled
	log.Printf("Hit: Re-rolled dice %d = %d (old value was %d)\n", diceList[4].ID, newValue, oldValue)

	// Re-evaluate the hand with the updated sum
	a.evaluate13Hand()

	is13Hitting = false
	is13Rolling = false
}

func verifyResult() {
	// Convert the currentSum to a string
	sumStr := strconv.Itoa(currentSum)
	mutex.Lock()
	ext.Send(out.SHOUT, sumStr)
	mutex.Unlock()
}

func (a *App) ShowCommands() {
	commandList :=
		"Thanks for using my plugin!\nBelow is it's list of commands. \n" +
			"------------------------------------\n" +
			":reset \n" +
			"Forgets dice list for when you\nchange booth.\n" +
			"------------------------------------\n" +
			":roll \n" +
			"Rolls 5 dice and if chat is enabled \nsays the results in chat. \n" +
			"------------------------------------\n" +
			":close\n" +
			"Closes any of your open dice. \n" +
			"------------------------------------\n" +
			":21 \n" +
			"Auto rolls and if chat is enabled \nsays the sum in chat when > 15. \n" +
			"------------------------------------\n" +
			":13 \n" +
			"Auto rolls and if chat is enabled \nsays the sum in chat when > 8. \n" +
			"------------------------------------\n" +
			":tri \n" +
			"Auto rolls 3 dice in Tri Formation \nif chat is enabled says the \nresults in chat. \n" +
			"------------------------------------\n" +
			":verify \n" +
			"Will say the previous result in\nchat. Use if you were muted and\ndont know the results of 21/13.\n" +
			"------------------------------------\n" +
			":chaton \n" +
			"Enables chat announcement \nof game results. \n" +
			"------------------------------------\n" +
			":chatoff \n" +
			"Disables chat announcement \nof game results. \n" +
			"------------------------------------\n" +
			":@ <amount> \n" +
			"Stores @ amount in roll log \nwith the result and will announce \nit in chat. \n" +
			"------------------------------------\n" +
			":commands - This help screen :)"

	time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
	ext.Send(in.SYSTEM_BROADCAST, commandList)
}

// QDave's Logging function for frontend
func (a *App) AddLogMsg(msg string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	// Get the current time and format it as a timestamp
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	// Prepend the timestamp to the message
	timestampedMsg := fmt.Sprintf("[%s] %s", timestamp, msg)

	a.log = append(a.log, timestampedMsg)
	if len(a.log) > 100 {
		a.log = a.log[1:]
	}
	runtime.EventsEmit(a.ctx, "logUpdate", strings.Join(a.log, "\n"))
}

func (a *App) AddChatLog(msg string) {
	a.chatLogMu.Lock()
	defer a.chatLogMu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	timestampedMsg := fmt.Sprintf("[%s] %s", timestamp, msg)

	a.chatLog = append(a.chatLog, timestampedMsg)
	if len(a.chatLog) > 100 {
		a.chatLog = a.chatLog[1:]
	}
	runtime.EventsEmit(a.ctx, "chatLogUpdate", strings.Join(a.chatLog, "\n"))
}

// Thanks QDave <3
func (a *App) handleTalk(e *g.Intercept) {
	msg := e.Packet.ReadString()
	if msg == "#gsuite" {
		runtime.WindowShow(a.ctx)
		e.Block()
	}
}

func (a *App) handleIncomingChat(e *g.Intercept) {
	index := e.Packet.ReadInt()
	msg := e.Packet.ReadString()

	chatType := "CHAT"
	if e.Is(in.CHAT_2) {
		chatType = "WHISPER"
	} else if e.Is(in.CHAT_3) {
		chatType = "SHOUT"
	}

	log.Printf("[INCOMING %s] %d -> %s", chatType, index, msg)
	a.AddChatLog(fmt.Sprintf("[IN %s] %d -> %s", chatType, index, msg))
}
