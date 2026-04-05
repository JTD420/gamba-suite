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
	"xabbo.b7c.io/goearth/shockwave/in"
	"xabbo.b7c.io/goearth/shockwave/out"
	room "xabbo.b7c.io/goearth/shockwave/room"
)

// Global variables for dice management, rolling state, mutex, and wait group
var (
	diceList                           []*Dice
	mutedDuration                      int
	isMuted                            bool
	currentSum                         int
	commandList                        string
	awaitingTradeOpen                  bool
	dealerTradeWindowOpen              bool
	tradeOpenCount                     int
	tradeCloseCount                    int
	lastTradePartnerID                 int
	lastTradePartnerName               string
	lastTradePartnerToken              string
	tradeAutoFlowID                    int
	tradeAutoAccepted                  bool
	tradeAutoConfirmed                 bool
	tradeAutoAcceptPending             bool
	tradeAutoConfirmPending            bool
	tradeCompleted                     bool
	tradeCloseAnnounced                bool
	suppressNextTradeCloseAnnouncement bool
	lastTradeCoverageNotice            string
	lastTradeBlockNotice               string
	awaitingGameChoice                 bool
	awaitingGameChoicePartnerID        int
	awaitingGameChoicePartnerName      string
	pokerSequenceStage                 int
	pokerSequencePlayerName            string
	pokerSequencePlayerResult          PokerHandResult
	pokerSequencePlayerHand            string
	pokerPayoutMode                    bool
	pokerPayoutTradeActive             bool
	pokerPayoutTargetID                int
	pokerPayoutTargetName              string
	pokerPayoutAttempts                int
	pokerPayoutSessionID               int
	pokerPayoutTradeSent               bool
	payoutExpectedAddCount             int
	payoutActualAddCount               int
	underfundedTradeMonitorID          int
	underfundedTradeMonitorNotice      string
	lastTradeOpenData                  string
	lastTradeOpen                      string
	isPokerRolling                     bool
	isTriRolling                       bool
	isBJRolling                        bool
	is13Rolling                        bool
	is13Hitting                        bool
	isHitting                          bool
	isClosing                          bool
	ChatIsDisabled                     bool
	mutex                              sync.Mutex
	resultsWaitGroup                   sync.WaitGroup
	rollDelay                          = 550 * time.Millisecond
	stripNextDelay                     = 2250 * time.Millisecond
	stripGetNewPayload                 = "new"
	stripGetNextPayload                = "next"
	tradeUserPattern                   = regexp.MustCompile(`\[(\d+)\]`)
	stripItemNameRe                    = regexp.MustCompile(`(?:CF_\d+_[a-z][a-z_]*|[a-z][a-z0-9_]*_[a-z0-9_]+)(?:\*\d+)?`)
	gameChoiceCleanupRe                = regexp.MustCompile(`[^a-z0-9]+`)
	roomEntities                       = map[int]room.Entity{}
	roomMu                             sync.Mutex
	lastRoomUsersRequestAt             time.Time
	roomUsersReqMu                     sync.Mutex
	users28ByToken                     = map[string]string{}
	users28ByIndex                     = map[int]string{} // roomIndex -> name
	users28ByShortToken                = map[string]string{}
	roomIdentityByShortToken           = map[string]RoomIdentityEntry{}
	users28Mu                          sync.Mutex
	headerSniffUntil                   time.Time
	headerSniffSeen                    = map[uint16]bool{}
	headerSniffMu                      sync.Mutex
	currentTradeItems                  []TradeItem
	currentOwnTradeItems               []TradeItem
	tradeItemsMu                       sync.Mutex
	lastAddItemWasOurs                 bool
	addItemMu                          sync.Mutex
	currentHandItems                   []TradeItem
	currentHandItemIDs                 map[string][]int
	handItemsMu                        sync.Mutex
	pokerGameBetItems                  []TradeItem
	stripScanMu                        sync.Mutex
	stripScanActive                    bool
	stripScanSessionID                 = 0
	stripScanPageCount                 = 0
	stripScanSeenItemIDs               = map[int]struct{}{}
	stripScanCounts                    = map[string]int{}
	stripScanItemIDs                   = map[string][]int{}
	knownDiceIDs                       = map[int]struct{}{}
	fakeDiceTestingMode                bool
	dealerOpenHeartbeatID              int
	dealerOpenHeartbeatActive          bool
	dealerResyncInProgress             bool
	lastOutgoingTradeOpenID            int
	lastOutgoingTradeOpenAt            time.Time
	tradeOpenStateMu                   sync.Mutex
)

type TradeItem struct {
	Name     string
	Quantity int
	RawData  string // Store raw field for debugging
}

type RoomIdentityEntry struct {
	Name      string `json:"name"`
	Token     string `json:"token"`
	Short     string `json:"short"`
	ChatIndex int    `json:"chatIndex"`
	RoomIndex int    `json:"roomIndex"`
}

type tradeShortage struct {
	Name        string
	Required    int
	Have        int
	PayoutTotal int
}

type App struct {
	ext       *g.Ext
	assets    embed.FS
	log       []string
	logMu     sync.Mutex
	chatLog   []string
	chatLogMu sync.Mutex
	ctx       context.Context
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
		time.Sleep(1200 * time.Millisecond)
		requestRoomUsers(a)

		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			requestRoomUsers(a)
		}
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

func dealerGameActive() bool {
	return awaitingGameChoice || pokerSequenceStage > 0 || isPokerRolling || isTriRolling || isBJRolling || is13Rolling || isHitting || is13Hitting || isClosing
}

func dealerReadyForNewTrade() bool {
	return awaitingTradeOpen && dealerTradeWindowOpen && !dealerGameActive() && !dealerResyncInProgress
}

func dealerDiceReadyLocked() bool {
	return fakeDiceTestingMode || len(diceList) >= 5
}

func dealerDiceReady() bool {
	mutex.Lock()
	defer mutex.Unlock()
	return dealerDiceReadyLocked()
}

func canAnnounceDealerOpenLocked() bool {
	return !isMuted && dealerDiceReadyLocked()
}

func canAnnounceDealerOpen() bool {
	return !isMuted && dealerDiceReady()
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
	a.ext.Intercept(out.GETSTRIP).With(a.handleOutgoingGetStrip)
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
	defer func() {
		if r := recover(); r != nil {
			a.AddLogMsg(fmt.Sprintf("[TRADE] recovered while handling header %d: %v", e.Packet.Header.Value, r))
			log.Printf("[TRADE] recovered while handling header %d: %v", e.Packet.Header.Value, r)
		}
	}()

	if e.Packet.Header.Dir == g.Out && (e.Packet.Header.Value == 69 || e.Packet.Header.Value == 402) && !pokerPayoutTradeActive {
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

	// TRADE_OPEN outgoing 71 - remember recent target so matching incoming 104 isn't blocked by dealer guard.
	if e.Packet.Header.Dir == g.Out && e.Packet.Header.Value == 71 {
		if targetID, ok := decodeLeadingVL64(e.Packet.Data); ok {
			rememberOutgoingTradeOpenTarget(targetID)
			a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN #71] remembered outgoing target id %d", targetID))
			log.Printf("[TRADE_OPEN #71] remembered outgoing target id %d", targetID)
		} else {
			a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN #71] outgoing payload decode failed: %q", string(e.Packet.Data)))
			log.Printf("[TRADE_OPEN #71] outgoing payload decode failed: %q", string(e.Packet.Data))
		}
	}

	// TRADE_ADDITEM outgoing 72 - we are adding an item; flag next TRADE_ITEMS as ours
	if e.Packet.Header.Dir == g.Out && e.Packet.Header.Value == 72 {
		addItemMu.Lock()
		lastAddItemWasOurs = true
		addItemMu.Unlock()
		if pokerPayoutTradeActive {
			payoutActualAddCount++
			a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] observed outgoing TRADE_ADDITEM count %d/%d", payoutActualAddCount, payoutExpectedAddCount))
			log.Printf("[PAYOUT_DEBUG] observed outgoing TRADE_ADDITEM count %d/%d", payoutActualAddCount, payoutExpectedAddCount)
		}
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
		allItems := a.parseTradeItemsPacket(e.Packet.Data)

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
		if pokerPayoutTradeActive {
			a.AddLogMsg("[TRADE_COVERAGE] skipped shortage enforcement during payout trade")
			log.Printf("[TRADE_COVERAGE] skipped shortage enforcement during payout trade")
		} else {
			a.notifyTradeQuantityCoverage()
		}
		return
	}

	// TRADE_COMPLETED header 112 - send chat message with the traded items
	if e.Packet.Header.Value == 112 {
		tradeCompleted = true
		tradeAutoConfirmed = true
		tradeAutoConfirmPending = false
		if pokerPayoutTradeActive {
			partnerName := strings.TrimSpace(lastTradePartnerName)
			if partnerName == "" || partnerName == "Unknown" {
				partnerName = strings.TrimSpace(pokerPayoutTargetName)
			}
			if partnerName == "" {
				partnerName = "Unknown"
			}

			a.AddLogMsg("[TRADE_COMPLETED #112] payout trade completed")
			log.Printf("[TRADE_COMPLETED #112] payout trade completed")

			completeMsg := fmt.Sprintf("Trade Completed: \"%s\"", partnerName)
			a.AddLogMsg(fmt.Sprintf("[TRADE_COMPLETED] shouting: %q", completeMsg))
			log.Printf("[TRADE_COMPLETED] shouting: %q", completeMsg)
			ext.Send(out.SHOUT, completeMsg)
		} else {
			a.AddLogMsg("[TRADE_COMPLETED #112] trade completed, sending trade summary")
			log.Printf("[TRADE_COMPLETED #112] trade completed, sending trade summary")

			// Send the trade items summary to chat
			a.sendTradeCompletionMessage()
			go a.requestPlayerStrip()
		}
		return
	}

	if e.Packet.Header.Value == 104 {
		a.ShowWindow()
		stopUnderfundedTradeMonitor()
		lastTradeCoverageNotice = ""
		lastTradeBlockNotice = ""

		incomingTraderID := 0
		if id, ok := decodeLeadingVL64(e.Packet.Data); ok {
			incomingTraderID = id
		}
		recentTargetID, matchedRecentOutgoing := matchesRecentOutgoingTradeOpen(e.Packet.Data, incomingTraderID)

		// During payout mode, someone else opened a trade with us — close it and let the payout loop retry
		isPayoutTradeOpen := false
		if pokerPayoutMode {
			if pokerPayoutTradeSent {
				// Our outgoing TRADE_OPEN was accepted — this is the payout trade opening successfully
				savedPayoutTargetID := pokerPayoutTargetID
				savedPayoutTargetName := pokerPayoutTargetName
				stopPokerPayout() // kills retry goroutine
				pokerPayoutTradeActive = true
				pokerPayoutTargetID = savedPayoutTargetID
				pokerPayoutTargetName = savedPayoutTargetName
				isPayoutTradeOpen = true
				a.AddLogMsg(fmt.Sprintf("[PAYOUT] trade opened successfully with %s, proceeding", savedPayoutTargetName))
				log.Printf("[PAYOUT] trade opened successfully with %s, proceeding", savedPayoutTargetName)
				// Fall through to normal trade-open handling below
				go a.autoAddPayoutItems()
			} else {
				// Someone else opened a trade with us during payout — block it
				a.AddLogMsg(fmt.Sprintf("[PAYOUT] incoming trade blocked during payout to %s, closing", pokerPayoutTargetName))
				log.Printf("[PAYOUT] incoming trade blocked during payout to %s, closing", pokerPayoutTargetName)
				suppressNextTradeCloseAnnouncement = true
				e.Block()
				ext.Send(out.TRADE_CLOSE)
				return
			}
		}

		if !isPayoutTradeOpen && !dealerReadyForNewTrade() && !matchedRecentOutgoing {
			reason := "dealer not open"
			if dealerGameActive() {
				reason = "dealer busy in active game"
			} else if dealerResyncInProgress {
				reason = "dealer syncing hand"
			} else if !awaitingTradeOpen {
				reason = "dealer not accepting trades"
			} else if !dealerTradeWindowOpen {
				reason = "dealer open announcement not active"
			}

			a.AddLogMsg(fmt.Sprintf("[TRADE_GUARD] blocking incoming trade open: %s", reason))
			log.Printf("[TRADE_GUARD] blocking incoming trade open: %s", reason)
			suppressNextTradeCloseAnnouncement = true
			e.Block()
			ext.Send(out.TRADE_CLOSE)
			return
		}

		if matchedRecentOutgoing {
			a.AddLogMsg(fmt.Sprintf("[TRADE_GUARD] allowing incoming trade open because it matches recent outgoing target %d", recentTargetID))
			log.Printf("[TRADE_GUARD] allowing incoming trade open because it matches recent outgoing target %d", recentTargetID)
		}

		awaitingGameChoice = false
		awaitingGameChoicePartnerID = 0
		awaitingGameChoicePartnerName = ""
		pokerSequenceStage = 0
		pokerSequencePlayerName = ""
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

		if len(e.Packet.Data) >= 1 {
			// Decode the trader's room index from the VL64 at the start of the trade packet.
			vlen := gencoding.VL64DecodeLen(e.Packet.Data[0])
			if vlen > 0 && vlen <= len(e.Packet.Data) {
				traderRoomIndex := gencoding.VL64Decode(e.Packet.Data[:vlen])
				if traderRoomIndex > 0 {
					lastTradePartnerID = traderRoomIndex
					a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN] decoded trader room index %d from VL64", traderRoomIndex))
					log.Printf("[TRADE_OPEN] decoded trader room index %d from VL64", traderRoomIndex)

					// Refresh users every trade-open so we can map room index -> username reliably.
					go requestRoomUsers(a)

					// Look up name by room index from USERS28 cache.
					indexName, indexOk := lookupUsers28Index(traderRoomIndex)
					if !indexOk {
						indexName, indexOk = waitForUsers28IndexName(traderRoomIndex, 900*time.Millisecond)
					}
					if indexOk {
						lastTradePartnerName = indexName
						a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN] resolved name %q from room index %d", indexName, traderRoomIndex))
						log.Printf("[TRADE_OPEN] resolved name %q from room index %d", indexName, traderRoomIndex)
					} else {
						a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN] room index %d not in USERS28 index cache, name unknown", traderRoomIndex))
						log.Printf("[TRADE_OPEN] room index %d not in USERS28 index cache, name unknown", traderRoomIndex)
					}
				}
			}

			// Keep token-based fallback for when index decode fails.
			if len(e.Packet.Data) >= 4 {
				tradeToken := string(e.Packet.Data[:4])
				lastTradePartnerToken = tradeToken
				if lastTradePartnerName == "" || lastTradePartnerName == "Unknown" {
					if name, ok := lookupUsers28Token(tradeToken); ok {
						lastTradePartnerName = name
						a.AddLogMsg(fmt.Sprintf("[TRADE_OPEN] fallback token %q matched name %q", tradeToken, name))
						log.Printf("[TRADE_OPEN] fallback token %q matched name %q", tradeToken, name)
					}
				}
				if lastTradePartnerID <= 0 {
					if idx, name, ok := resolveTradeTokenToRoomIndex(tradeToken); ok {
						lastTradePartnerID = idx
						if lastTradePartnerName == "" || lastTradePartnerName == "Unknown" {
							lastTradePartnerName = name
						}
					}
				}
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
		dealerTradeWindowOpen = false
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

		suppressCloseAnnouncement := suppressNextTradeCloseAnnouncement
		suppressNextTradeCloseAnnouncement = false

		if !tradeCompleted && !tradeCloseAnnounced && !suppressCloseAnnouncement {
			closeMsg := fmt.Sprintf("Trade Closed: \"%s\"", partnerName)
			a.AddLogMsg(fmt.Sprintf("[TRADE_CLOSE] shouting: %q", closeMsg))
			log.Printf("[TRADE_CLOSE] shouting: %q", closeMsg)
			ext.Send(out.SHOUT, closeMsg)
			tradeCloseAnnounced = true
		} else if suppressCloseAnnouncement {
			a.AddLogMsg("[TRADE_GUARD] suppressed trade closed announcement for forced guard-close")
			log.Printf("[TRADE_GUARD] suppressed trade closed announcement for forced guard-close")
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
			if pokerPayoutTradeActive {
				// Payout trade was cancelled by player — retry the payout
				retryTargetID := pokerPayoutTargetID
				retryTargetName := pokerPayoutTargetName
				pokerPayoutTradeActive = false
				a.AddLogMsg(fmt.Sprintf("[PAYOUT] payout trade cancelled by %s, retrying", retryTargetName))
				log.Printf("[PAYOUT] payout trade cancelled by %s, retrying", retryTargetName)
				startPokerPayout(a, retryTargetID, retryTargetName)
			} else {
				awaitingTradeOpen = true
				a.AddLogMsg("[TRADE_REOPEN] trade closed before completion, restarting dealer cycle")
				log.Printf("[TRADE_REOPEN] trade closed before completion, restarting dealer cycle")
				if canAnnounceDealerOpen() {
					dealerTradeWindowOpen = true
					go sendMessageWithDelay(a.dealerOpenMessage())
				} else {
					dealerTradeWindowOpen = false
					log.Printf("User is muted. Dealer open announcement skipped; incoming trades will be blocked.")
				}
				startDealerOpenHeartbeat(a)
			}
		} else if pokerPayoutTradeActive {
			// Payout trade completed normally — clear active flag
			pokerPayoutTradeActive = false
			a.AddLogMsg("[PAYOUT] payout trade completed successfully")
			log.Printf("[PAYOUT] payout trade completed successfully")

			// Resync hand before reopening dealer trades.
			go a.resyncHandThenOpenDealer()
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

func resetPokerSequence() {
	pokerSequenceStage = 0
	pokerSequencePlayerName = ""
	pokerSequencePlayerResult = PokerHandResult{}
	pokerSequencePlayerHand = ""
}

func stopPokerPayout() {
	pokerPayoutMode = false
	pokerPayoutTradeActive = false
	pokerPayoutTargetID = 0
	pokerPayoutTargetName = ""
	pokerPayoutAttempts = 0
	pokerPayoutSessionID++
	pokerPayoutTradeSent = false
	payoutExpectedAddCount = 0
	payoutActualAddCount = 0
}

func startPokerPayout(a *App, targetID int, targetName string) {
	stopPokerPayout()
	pokerPayoutMode = true
	pokerPayoutTargetID = targetID
	pokerPayoutTargetName = targetName
	pokerPayoutSessionID++
	sessionID := pokerPayoutSessionID

	go func() {
		// Small delay so the winner shout clears Habbo's rate limiter first
		time.Sleep(1200 * time.Millisecond)

		for attempt := 1; attempt <= 5; attempt++ {
			if sessionID != pokerPayoutSessionID {
				return
			}
			pokerPayoutAttempts = attempt

			if strings.TrimSpace(targetName) != "" {
				go requestRoomUsers(a)
				if resolvedID, ok := waitForRoomEntityIndexByName(targetName, 900*time.Millisecond); ok && resolvedID > 0 && resolvedID != targetID {
					a.AddLogMsg(fmt.Sprintf("[PAYOUT] refreshed %s target from ROOM_USERS index %d -> %d", targetName, targetID, resolvedID))
					log.Printf("[PAYOUT] refreshed %s target from ROOM_USERS index %d -> %d", targetName, targetID, resolvedID)
					targetID = resolvedID
					pokerPayoutTargetID = resolvedID
				} else if resolvedID, ok := waitForUsers28NameIndex(targetName, 700*time.Millisecond); ok && resolvedID > 0 && resolvedID != targetID {
					a.AddLogMsg(fmt.Sprintf("[PAYOUT] refreshed %s target from USERS28 index %d -> %d", targetName, targetID, resolvedID))
					log.Printf("[PAYOUT] refreshed %s target from USERS28 index %d -> %d", targetName, targetID, resolvedID)
					targetID = resolvedID
					pokerPayoutTargetID = resolvedID
				}
			}

			a.AddLogMsg(fmt.Sprintf("[PAYOUT] opening trade with %s (%d), attempt %d/5", targetName, targetID, attempt))
			log.Printf("[PAYOUT] opening trade with %s (%d), attempt %d/5", targetName, targetID, attempt)
			pokerPayoutTradeSent = true
			rememberOutgoingTradeOpenTarget(targetID)
			outPreview := string(ext.NewPacket(out.TRADE_OPEN, targetID).Data)
			a.AddLogMsg(fmt.Sprintf("[PAYOUT] outgoing[71] payload=%q", outPreview))
			log.Printf("[PAYOUT] outgoing[71] payload=%q", outPreview)
			ext.Send(out.TRADE_OPEN, targetID)

			// Fallback: send the raw VL64 payload form as well. Some sessions are picky about payload composition.
			rawPayload := encodeVL64(targetID)
			a.AddLogMsg(fmt.Sprintf("[PAYOUT] outgoing[71] raw payload fallback=%q", rawPayload))
			log.Printf("[PAYOUT] outgoing[71] raw payload fallback=%q", rawPayload)
			ext.Send(g.Out.Id("TRADE_OPEN"), []byte(rawPayload))

			if attempt > 1 {
				msg := fmt.Sprintf("Tried to open trade %d times", attempt)
				time.Sleep(800 * time.Millisecond)
				if sessionID != pokerPayoutSessionID {
					return
				}
				sendMessageWithDelay(msg)
			}

			// Wait up to 5 seconds for the trade to open (header 104 will call stopPokerPayout)
			for i := 0; i < 50; i++ {
				time.Sleep(100 * time.Millisecond)
				if sessionID != pokerPayoutSessionID {
					// Trade opened (or externally cancelled) — done
					return
				}
			}

			// Trade didn't open after 5s, loop for next attempt
		}

		// All 5 attempts exhausted
		if sessionID == pokerPayoutSessionID {
			msg := "Recorded game history and flagged"
			a.AddLogMsg(fmt.Sprintf("[PAYOUT] all attempts exhausted, shouting: %q", msg))
			log.Printf("[PAYOUT] all attempts exhausted, shouting: %q", msg)
			sendMessageWithDelay(msg)
			stopPokerPayout()
			// Resume normal dealer-open cycle
			awaitingTradeOpen = true
			if canAnnounceDealerOpen() {
				dealerTradeWindowOpen = true
				go sendMessageWithDelay(a.dealerOpenMessage())
			}
			startDealerOpenHeartbeat(a)
		}
	}()
}

func (a *App) autoAddPayoutItems() {
	time.Sleep(600 * time.Millisecond) // settle time after trade opens

	betItems := pokerGameBetItems
	if len(betItems) == 0 {
		a.AddLogMsg("[PAYOUT] no bet items recorded, skipping auto-add")
		log.Printf("[PAYOUT] no bet items recorded, skipping auto-add")
		return
	}

	a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] auto-add start: bet item types=%d", len(betItems)))
	for i, betItem := range betItems {
		a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] bet[%d] name=%q qty=%d payoutTarget=%d", i, betItem.Name, betItem.Quantity, betItem.Quantity*2))
	}

	required := payoutRequirementsFromBetItems(betItems)
	selectedByName := map[string][]int{}
	usedIDs := map[int]struct{}{}

	// Try current hand first, then rescan hand pages if we are still short.
	for attempt := 1; attempt <= 3; attempt++ {
		handSnapshot := snapshotHandItemIDs()
		for name, needQty := range required {
			if needQty <= 0 {
				continue
			}

			already := len(selectedByName[name])
			if already >= needQty {
				continue
			}

			candidates := uniqueInts(handSnapshot[name])
			for _, id := range candidates {
				if _, seen := usedIDs[id]; seen {
					continue
				}
				selectedByName[name] = append(selectedByName[name], id)
				usedIDs[id] = struct{}{}
				if len(selectedByName[name]) >= needQty {
					break
				}
			}
		}

		missing := payoutMissingCounts(required, selectedByName)
		if len(missing) == 0 {
			break
		}

		if attempt < 3 {
			a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] payout still short after hand scan attempt %d, requesting next hand scan: %s", attempt, formatMissingCounts(missing)))
			log.Printf("[PAYOUT_DEBUG] payout still short after hand scan attempt %d, requesting next hand scan: %s", attempt, formatMissingCounts(missing))
			go a.requestPlayerStrip()
			time.Sleep(8 * time.Second)
		} else {
			a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] payout still short after final hand scan attempt: %s", formatMissingCounts(missing)))
			log.Printf("[PAYOUT_DEBUG] payout still short after final hand scan attempt: %s", formatMissingCounts(missing))
		}
	}

	total := 0
	plannedIDs := make([]int, 0)
	for _, betItem := range betItems {
		needed := required[betItem.Name]
		toAdd := selectedByName[betItem.Name]
		a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] selected ids for %s: selected=%d needed=%d", betItem.Name, len(toAdd), needed))
		if len(toAdd) < needed {
			a.AddLogMsg(fmt.Sprintf("[PAYOUT] warning: need %d of %s but only found %d unique item ids", needed, betItem.Name, len(toAdd)))
			log.Printf("[PAYOUT] warning: need %d of %s but only found %d unique item ids", needed, betItem.Name, len(toAdd))
		}
		for _, itemID := range toAdd {
			if !pokerPayoutTradeActive {
				a.AddLogMsg("[PAYOUT] trade closed mid-add, stopping")
				return
			}
			time.Sleep(550 * time.Millisecond)
			ext.Send(out.TRADE_ADDITEM, -itemID)
			if pokerPayoutTradeActive {
				payoutActualAddCount++
			}
			plannedIDs = append(plannedIDs, itemID)
			total++
			payload := string(ext.NewPacket(out.TRADE_ADDITEM, -itemID).Data)
			a.AddLogMsg(fmt.Sprintf("[PAYOUT] added item %d (%s) %d/%d payload=%q", itemID, betItem.Name, total, needed, payload))
			log.Printf("[PAYOUT] added item %d (%s) %d/%d payload=%q", itemID, betItem.Name, total, needed, payload)
			a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] sent count now %d/%d", payoutActualAddCount, payoutExpectedAddCount))
			log.Printf("[PAYOUT_DEBUG] sent count now %d/%d", payoutActualAddCount, payoutExpectedAddCount)
		}
	}
	a.AddLogMsg(fmt.Sprintf("[PAYOUT] auto-add complete: %d item(s) offered", total))
	log.Printf("[PAYOUT] auto-add complete: %d item(s) offered", total)

	// Accept only after full payout placement has been queued.
	requiredTotal := 0
	fullyPlanned := true
	for name, need := range required {
		requiredTotal += need
		if len(selectedByName[name]) < need {
			fullyPlanned = false
		}
	}
	payoutExpectedAddCount = requiredTotal
	payoutActualAddCount = 0

	if pokerPayoutTradeActive && !tradeAutoAccepted {
		if fullyPlanned && total >= requiredTotal && payoutActualAddCount >= requiredTotal {
			time.Sleep(450 * time.Millisecond)
			ext.Send(out.TRADE_ACCEPT)
			tradeAutoAccepted = true
			tradeAutoAcceptPending = false
			a.AddLogMsg(fmt.Sprintf("[PAYOUT] auto-accept sent after full payout placement (%d/%d sent=%d)", total, requiredTotal, payoutActualAddCount))
			log.Printf("[PAYOUT] auto-accept sent after full payout placement (%d/%d sent=%d)", total, requiredTotal, payoutActualAddCount)
		} else {
			a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] accept deferred: planned=%d/%d sent=%d/%d", total, requiredTotal, payoutActualAddCount, requiredTotal))
			log.Printf("[PAYOUT_DEBUG] accept deferred: planned=%d/%d sent=%d/%d", total, requiredTotal, payoutActualAddCount, requiredTotal)
		}
	}

	go a.verifyAndRetryPayoutAdds(plannedIDs)
}

func snapshotHandItemIDs() map[string][]int {
	handItemsMu.Lock()
	defer handItemsMu.Unlock()

	snapshot := make(map[string][]int, len(currentHandItemIDs))
	for name, ids := range currentHandItemIDs {
		copyIDs := make([]int, len(ids))
		copy(copyIDs, ids)
		snapshot[name] = copyIDs
	}
	return snapshot
}

func uniqueInts(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	seen := map[int]struct{}{}
	unique := make([]int, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		key := absInt(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func payoutMissingCounts(required map[string]int, selected map[string][]int) map[string]int {
	missing := map[string]int{}
	for name, need := range required {
		have := len(selected[name])
		if have < need {
			missing[name] = need - have
		}
	}
	return missing
}

func formatMissingCounts(missing map[string]int) string {
	if len(missing) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(missing))
	for name, qty := range missing {
		parts = append(parts, fmt.Sprintf("%s:%d", name, qty))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func ownTradeOfferTotal() int {
	tradeItemsMu.Lock()
	defer tradeItemsMu.Unlock()
	total := 0
	for _, item := range currentOwnTradeItems {
		total += item.Quantity
	}
	return total
}

func payoutRequirementsFromBetItems(betItems []TradeItem) map[string]int {
	required := map[string]int{}
	for _, item := range betItems {
		if item.Quantity <= 0 {
			continue
		}
		required[item.Name] += item.Quantity * 2
	}
	return required
}

func ownTradeOfferCounts() map[string]int {
	tradeItemsMu.Lock()
	defer tradeItemsMu.Unlock()
	counts := map[string]int{}
	for _, item := range currentOwnTradeItems {
		counts[item.Name] += item.Quantity
	}
	return counts
}

func (a *App) ownTradeHasRequiredPayoutOffer(required map[string]int) (bool, string) {
	have := ownTradeOfferCounts()
	for name, qty := range required {
		if have[name] < qty {
			return false, fmt.Sprintf("%s have=%d need=%d", name, have[name], qty)
		}
	}
	return true, ""
}

func (a *App) tryAcceptPayoutTrade(required map[string]int, source string) bool {
	if !pokerPayoutTradeActive {
		return false
	}
	if tradeAutoAccepted {
		return true
	}
	ok, detail := a.ownTradeHasRequiredPayoutOffer(required)
	if !ok {
		a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] not accepting yet (%s): %s", source, detail))
		log.Printf("[PAYOUT_DEBUG] not accepting yet (%s): %s", source, detail)
		return false
	}

	ext.Send(out.TRADE_ACCEPT)
	tradeAutoAccepted = true
	tradeAutoAcceptPending = false
	a.AddLogMsg(fmt.Sprintf("[PAYOUT] auto-accept sent after verifying payout items (%s)", source))
	log.Printf("[PAYOUT] auto-accept sent after verifying payout items (%s)", source)
	return true
}

func (a *App) verifyAndRetryPayoutAdds(plannedIDs []int) {
	if len(plannedIDs) == 0 {
		return
	}

	required := payoutRequirementsFromBetItems(pokerGameBetItems)
	if len(required) == 0 {
		a.AddLogMsg("[PAYOUT_DEBUG] no payout requirements found while verifying add")
		return
	}
	requiredTotal := 0
	for _, need := range required {
		requiredTotal += need
	}

	// Give the server time to echo TRADE_ITEMS updates.
	time.Sleep(2500 * time.Millisecond)
	if !pokerPayoutTradeActive {
		return
	}

	ownTotal := ownTradeOfferTotal()
	a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] post-add own offer total=%d planned=%d", ownTotal, len(plannedIDs)))
	log.Printf("[PAYOUT_DEBUG] post-add own offer total=%d planned=%d", ownTotal, len(plannedIDs))
	if a.tryAcceptPayoutTrade(required, "after-negative-add") {
		return
	}

	// Fallback for sessions where TRADE_ADDITEM expects a positive item id.
	a.AddLogMsg("[PAYOUT_DEBUG] own offer still empty after auto-add, retrying with positive item IDs")
	log.Printf("[PAYOUT_DEBUG] own offer still empty after auto-add, retrying with positive item IDs")
	for _, itemID := range plannedIDs {
		if !pokerPayoutTradeActive {
			return
		}
		time.Sleep(450 * time.Millisecond)
		ext.Send(out.TRADE_ADDITEM, itemID)
		if pokerPayoutTradeActive {
			payoutActualAddCount++
		}
		payload := string(ext.NewPacket(out.TRADE_ADDITEM, itemID).Data)
		a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] retry add item +%d payload=%q", itemID, payload))
		log.Printf("[PAYOUT_DEBUG] retry add item +%d payload=%q", itemID, payload)
		a.AddLogMsg(fmt.Sprintf("[PAYOUT_DEBUG] sent count now %d/%d", payoutActualAddCount, payoutExpectedAddCount))
		log.Printf("[PAYOUT_DEBUG] sent count now %d/%d", payoutActualAddCount, payoutExpectedAddCount)
	}

	// Wait again for trade echo, then only accept if exact payout offer is present.
	time.Sleep(2200 * time.Millisecond)
	if a.tryAcceptPayoutTrade(required, "after-positive-retry") {
		return
	}

	if pokerPayoutTradeActive && !tradeAutoAccepted && len(plannedIDs) >= requiredTotal && payoutActualAddCount >= requiredTotal {
		// Attribution can be unreliable in this direction; once full payout has been queued,
		// accept without waiting for the player to accept first.
		ext.Send(out.TRADE_ACCEPT)
		tradeAutoAccepted = true
		tradeAutoAcceptPending = false
		a.AddLogMsg(fmt.Sprintf("[PAYOUT] auto-accept fallback after queued full payout (%d/%d sent=%d)", len(plannedIDs), requiredTotal, payoutActualAddCount))
		log.Printf("[PAYOUT] auto-accept fallback after queued full payout (%d/%d sent=%d)", len(plannedIDs), requiredTotal, payoutActualAddCount)
		return
	}

	a.AddLogMsg("[PAYOUT_DEBUG] payout items still not fully reflected in own trade offer; waiting for manual intervention")
	log.Printf("[PAYOUT_DEBUG] payout items still not fully reflected in own trade offer; waiting for manual intervention")
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

			if !awaitingTradeOpen || !dealerTradeWindowOpen {
				dealerOpenHeartbeatActive = false
				return
			}

			a.AddLogMsg("[TRADE_REOPEN] no new trade yet, re-announcing dealer open")
			log.Printf("[TRADE_REOPEN] no new trade yet, re-announcing dealer open")
			if canAnnounceDealerOpen() {
				sendMessageWithDelay(a.dealerOpenMessage())
			} else {
				dealerTradeWindowOpen = false
				dealerOpenHeartbeatActive = false
				return
			}
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
	if !pokerPayoutTradeActive && len(a.getTradeCoverageShortages()) > 0 {
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

		if !pokerPayoutTradeActive && len(a.getTradeCoverageShortages()) > 0 {
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
	if !pokerPayoutTradeActive && len(a.getTradeCoverageShortages()) > 0 {
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

			if !pokerPayoutTradeActive && len(a.getTradeCoverageShortages()) > 0 {
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
	if canAnnounceDealerOpen() {
		tradeCloseAnnounced = true
		sendMessageWithDelay(closeMsg)
		sendMessageWithDelay(a.dealerOpenMessage())
		dealerTradeWindowOpen = true
	} else {
		dealerTradeWindowOpen = false
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
	prev := map[string]RoomIdentityEntry{}
	for k, v := range roomIdentityByShortToken {
		prev[k] = v
	}
	clear(users28ByToken)
	clear(users28ByIndex)
	clear(users28ByShortToken)
	clear(roomIdentityByShortToken)

	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		chatIndex := 0
		if idx, ok := chatIndexFromShortToken(entry.ShortToken); ok && idx > 0 {
			chatIndex = idx
		}

		if entry.Token != "" {
			users28ByToken[entry.Token] = name
		}
		if entry.ShortToken != "" {
			users28ByShortToken[entry.ShortToken] = name
			if chatIndex > 0 {
				users28ByIndex[chatIndex] = name
			}
		}
		if entry.RoomIndex > 0 {
			users28ByIndex[entry.RoomIndex] = name
			if short := shortTokenFromIndex(entry.RoomIndex); isLikelyChatToken(short) {
				users28ByShortToken[short] = name
			}
		}

		for _, short := range shortTokenCandidates(entry.Token) {
			users28ByShortToken[short] = name
			if idx, ok := chatIndexFromShortToken(short); ok && idx > 0 {
				users28ByIndex[idx] = name
				if chatIndex <= 0 {
					chatIndex = idx
				}
			}
		}

		if entry.ShortToken != "" {
			roomIdentityByShortToken[entry.ShortToken] = RoomIdentityEntry{
				Name:      name,
				Token:     entry.Token,
				Short:     entry.ShortToken,
				ChatIndex: chatIndex,
				RoomIndex: entry.RoomIndex,
			}
		}
	}

	joined := make([]string, 0)
	left := make([]string, 0)
	for short, curr := range roomIdentityByShortToken {
		if _, ok := prev[short]; !ok {
			joined = append(joined, fmt.Sprintf("%s(%s)", curr.Name, short))
		}
	}
	for short, old := range prev {
		if _, ok := roomIdentityByShortToken[short]; !ok {
			left = append(left, fmt.Sprintf("%s(%s)", old.Name, short))
		}
	}
	users28Mu.Unlock()

	sort.Strings(joined)
	sort.Strings(left)
	if len(joined) > 0 {
		a.AddLogMsg(fmt.Sprintf("[ROOM_USERS] joined: %s", strings.Join(joined, ", ")))
		log.Printf("[ROOM_USERS] joined: %s", strings.Join(joined, ", "))
	}
	if len(left) > 0 {
		a.AddLogMsg(fmt.Sprintf("[ROOM_USERS] left: %s", strings.Join(left, ", ")))
		log.Printf("[ROOM_USERS] left: %s", strings.Join(left, ", "))
	}
	a.emitRoomIdentityUpdate()

	for _, entry := range entries {
		a.AddLogMsg(fmt.Sprintf("[USERS28] header=%d token=%q short=%q name=%q roomIndex=%d", e.Packet.Header.Value, entry.Token, entry.ShortToken, entry.Name, entry.RoomIndex))
		log.Printf("[USERS28] header=%d token=%q short=%q name=%q roomIndex=%d", e.Packet.Header.Value, entry.Token, entry.ShortToken, entry.Name, entry.RoomIndex)
	}
}

func lookupUsers28Token(token string) (string, bool) {
	users28Mu.Lock()
	defer users28Mu.Unlock()
	name, ok := users28ByToken[token]
	return name, ok
}

func lookupUsers28Index(index int) (string, bool) {
	users28Mu.Lock()
	defer users28Mu.Unlock()
	name, ok := users28ByIndex[index]
	if (!ok || strings.TrimSpace(name) == "") && index > 0 {
		if short := shortTokenFromIndex(index); short != "" {
			if byShort, ok2 := users28ByShortToken[short]; ok2 {
				name = byShort
				ok = true
			}
		}
	}
	if !ok {
		return "", false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	return name, true
}

func waitForUsers28IndexName(index int, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if name, ok := lookupUsers28Index(index); ok {
			return name, true
		}
		time.Sleep(75 * time.Millisecond)
	}
	return "", false
}

func lookupUsers28NameIndex(name string) (int, bool) {
	needle := strings.ToLower(strings.TrimSpace(name))
	if needle == "" {
		return 0, false
	}

	users28Mu.Lock()
	defer users28Mu.Unlock()
	for idx, cachedName := range users28ByIndex {
		if strings.ToLower(strings.TrimSpace(cachedName)) == needle {
			return idx, true
		}
	}
	return 0, false
}

func waitForUsers28NameIndex(name string, timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if idx, ok := lookupUsers28NameIndex(name); ok {
			return idx, true
		}
		time.Sleep(75 * time.Millisecond)
	}
	return 0, false
}

func shortTokenFromIndex(index int) string {
	if index <= 0 {
		return ""
	}
	return encodeB64(index, 2)
}

func isLikelyChatToken(s string) bool {
	if len(s) != 2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] > 126 {
			return false
		}
	}
	return true
}

func decodeShortChatToken(token string) (idx int, ok bool) {
	if !isLikelyChatToken(token) {
		return 0, false
	}
	defer func() {
		if recover() != nil {
			idx = 0
			ok = false
		}
	}()
	idx = gencoding.B64Decode([]byte(token))
	if idx <= 0 {
		return 0, false
	}
	return idx, true
}

func chatIndexFromShortToken(token string) (idx int, ok bool) {
	if !isLikelyChatToken(token) {
		return 0, false
	}
	defer func() {
		if recover() != nil {
			idx = 0
			ok = false
		}
	}()
	idx = gencoding.B64Decode([]byte(token[:1]))
	if idx <= 0 {
		return 0, false
	}
	return idx, true
}

func lookupRoomIdentityByChatIndex(index int) (string, bool) {
	if index <= 0 {
		return "", false
	}

	users28Mu.Lock()
	defer users28Mu.Unlock()

	for short, entry := range roomIdentityByShortToken {
		if idx, ok := chatIndexFromShortToken(short); ok && idx == index {
			name := strings.TrimSpace(entry.Name)
			if name != "" {
				return name, true
			}
		}
	}

	return "", false
}

func shortTokenCandidates(token string) []string {
	if len(token) < 2 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for i := 0; i+2 <= len(token); i++ {
		cand := token[i : i+2]
		if !isLikelyChatToken(cand) {
			continue
		}
		if _, ok := seen[cand]; ok {
			continue
		}
		seen[cand] = struct{}{}
		out = append(out, cand)
	}
	return out
}

func lookupRoomEntityIndexByName(name string) (int, bool) {
	needle := strings.ToLower(strings.TrimSpace(name))
	if needle == "" {
		return 0, false
	}

	roomMu.Lock()
	defer roomMu.Unlock()
	for _, entity := range roomEntities {
		entityName := strings.TrimSpace(entity.Name)
		if _, clean, ok := splitTokenAndName(entityName); ok {
			entityName = clean
		}
		if strings.ToLower(entityName) == needle {
			return entity.Index, true
		}
	}

	return 0, false
}

func waitForRoomEntityIndexByName(name string, timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if idx, ok := lookupRoomEntityIndexByName(name); ok {
			return idx, true
		}
		time.Sleep(75 * time.Millisecond)
	}
	return 0, false
}

func lookupRoomEntityNameByIndex(index int) (string, bool) {
	if index <= 0 {
		return "", false
	}

	roomMu.Lock()
	defer roomMu.Unlock()
	entity, ok := roomEntities[index]
	if !ok {
		return "", false
	}

	name := strings.TrimSpace(entity.Name)
	if _, clean, hasToken := splitTokenAndName(name); hasToken {
		name = strings.TrimSpace(clean)
	}
	if name == "" {
		return "", false
	}

	return name, true
}

// parseTradeItemsPacket extracts trade items from TRADE_ITEMS packet (header 108)
// Items are separated by \x02 bytes and may contain item names and quantities.
func (a *App) parseTradeItemsPacket(data []byte) []TradeItem {
	counts := map[string]int{}
	rawByName := map[string]string{}

	// Split on \x02 separator byte.
	fields := bytes.Split(data, []byte{0x02})
	for _, field := range fields {
		if len(field) == 0 {
			continue
		}

		fieldStr := strings.TrimSpace(string(field))
		itemName, qty, ok := a.extractTradeItemAndQuantity(fieldStr)
		if !ok {
			continue
		}
		if qty <= 0 {
			qty = 1
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

func (a *App) extractTradeItemAndQuantity(field string) (string, int, bool) {
	// Legacy format example: "itkoHP|club_sofa"
	if strings.Contains(field, "|") {
		parts := strings.Split(field, "|")
		if len(parts) >= 2 {
			if name, qty, ok := a.normalizeTradeFieldClassWithQty(parts[len(parts)-1]); ok {
				return name, qty, true
			}
		}
	}

	// Current format example: "irbUAXb{chair_plasty*2" or "irbUAXb{chair_plasty*109"
	if strings.Contains(field, "{") {
		parts := strings.SplitN(field, "{", 2)
		if len(parts) == 2 {
			if name, qty, ok := a.normalizeTradeFieldClassWithQty(parts[1]); ok {
				return name, qty, true
			}
		}
	}

	return "", 0, false
}

func (a *App) extractTradeItemName(field string) (string, bool) {
	name, _, ok := a.extractTradeItemAndQuantity(field)
	return name, ok
}

func (a *App) normalizeTradeFieldClass(raw string) (string, bool) {
	name, _, ok := a.normalizeTradeFieldClassWithQty(raw)
	return name, ok
}

func (a *App) normalizeTradeFieldClassWithQty(raw string) (string, int, bool) {
	normalized, ok := normalizeClassKeyWithVariant(raw)
	if !ok {
		return "", 0, false
	}

	if !isKnownTradeClassName(a, normalized) {
		return "", 0, false
	}

	// Trade quantity is represented by repeated item entries, not by the *n variant suffix.
	return normalized, 1, true
}

func isKnownTradeClassName(a *App, name string) bool {
	// Accept names that match the strict furni class-name regex (e.g. chair_plasty, cf_10_coin_gold).
	if stripItemNameRe.MatchString(name) && stripItemNameRe.FindString(name) == name {
		return true
	}

	// Also accept single-word items that appear in the dealer's scanned hand (e.g. edice).
	handItemsMu.Lock()
	for _, item := range currentHandItems {
		if item.Name == name {
			handItemsMu.Unlock()
			return true
		}
	}
	handItemsMu.Unlock()

	return false
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

func normalizeClassKeyWithVariant(raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "null" {
		return "", false
	}

	if star := strings.LastIndex(raw, "*"); star > 0 {
		suffix := raw[star+1:]
		if suffix == "" {
			return "", false
		}
		for _, r := range suffix {
			if r < '0' || r > '9' {
				return "", false
			}
		}

		base, ok := normalizeTradeItemName(raw[:star])
		if !ok {
			return "", false
		}
		return base + "*" + suffix, true
	}

	return normalizeTradeItemName(raw)
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
	if strings.TrimSpace(lastTradePartnerName) != "" && lastTradePartnerName != "Unknown" {
		if idx, ok := lookupRoomEntityIndexByName(lastTradePartnerName); ok && idx > 0 && idx != lastTradePartnerID {
			a.AddLogMsg(fmt.Sprintf("Open Last Trade refreshed partner index from ROOM_USERS: %s (%d -> %d)", lastTradePartnerName, lastTradePartnerID, idx))
			lastTradePartnerID = idx
		} else if idx, ok := lookupUsers28NameIndex(lastTradePartnerName); ok && idx > 0 && idx != lastTradePartnerID {
			a.AddLogMsg(fmt.Sprintf("Open Last Trade refreshed partner index from USERS28: %s (%d -> %d)", lastTradePartnerName, lastTradePartnerID, idx))
			lastTradePartnerID = idx
		}
	}

	if lastTradePartnerID <= 0 {
		a.AddLogMsg("Open Last Trade failed: no last trader cached yet")
		go requestRoomUsers(a)
		return
	}

	ext.Send(out.TRADE_OPEN, lastTradePartnerID)
	rememberOutgoingTradeOpenTarget(lastTradePartnerID)
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
func (a *App) requestPlayerStrip() int {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[STRIP] GETSTRIP request panicked: %v", r)
		}
	}()

	stripScanMu.Lock()
	stripScanSessionID++
	sessionID := stripScanSessionID
	stripScanActive = true
	stripScanPageCount = 0
	stripScanSeenItemIDs = map[int]struct{}{}
	stripScanCounts = map[string]int{}
	stripScanItemIDs = map[string][]int{}
	stripScanMu.Unlock()
	a.AddLogMsg(fmt.Sprintf("[STRIP_DEBUG] start scan session=%d", sessionID))
	log.Printf("[STRIP_DEBUG] start scan session=%d", sessionID)

	sendGetStripRaw(a, stripGetNewPayload)
	a.AddLogMsg("[STRIP] requested player hand scan (GETSTRIP new)")
	log.Printf("[STRIP] requested player hand scan (GETSTRIP new)")
	return sessionID
}

func waitForStripScanCompletion(sessionID int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stripScanMu.Lock()
		active := stripScanActive
		current := stripScanSessionID
		stripScanMu.Unlock()

		if current > sessionID {
			return true
		}
		if current == sessionID && !active {
			return true
		}

		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func (a *App) resyncHandThenOpenDealer() {
	awaitingTradeOpen = false
	dealerTradeWindowOpen = false
	dealerResyncInProgress = true
	stopDealerOpenHeartbeat()

	a.AddLogMsg("[TRADE_REOPEN] payout complete; syncing hand before reopening trades")
	log.Printf("[TRADE_REOPEN] payout complete; syncing hand before reopening trades")

	scanID := a.requestPlayerStrip()
	if ok := waitForStripScanCompletion(scanID, 20*time.Second); ok {
		a.AddLogMsg(fmt.Sprintf("[TRADE_REOPEN] hand sync complete (session=%d)", scanID))
		log.Printf("[TRADE_REOPEN] hand sync complete (session=%d)", scanID)
	} else {
		a.AddLogMsg(fmt.Sprintf("[TRADE_REOPEN] hand sync timeout (session=%d), reopening anyway", scanID))
		log.Printf("[TRADE_REOPEN] hand sync timeout (session=%d), reopening anyway", scanID)
	}

	dealerResyncInProgress = false
	awaitingTradeOpen = true
	if canAnnounceDealerOpen() {
		dealerTradeWindowOpen = true
		openMsg := a.dealerOpenMessage()
		a.AddLogMsg(fmt.Sprintf("[TRADE_REOPEN] shouting: %q", openMsg))
		log.Printf("[TRADE_REOPEN] shouting: %q", openMsg)
		go sendMessageWithDelay(openMsg)
	} else {
		dealerTradeWindowOpen = false
		log.Printf("User is muted. Dealer open announcement skipped; incoming trades will be blocked.")
	}
	startDealerOpenHeartbeat(a)
}

// handleStripPacket parses STRIPINFO_2 [140] to track items in the player's hand.
func handleStripPacket(a *App, e *g.Intercept) {
	if e.Packet.Header.Dir != g.In || e.Packet.Header.Value != 140 {
		return
	}

	rawData := append([]byte(nil), e.Packet.Data...)

	stripScanMu.Lock()
	active := stripScanActive
	if !active {
		stripScanActive = true
		stripScanSessionID++
		stripScanPageCount = 0
		stripScanSeenItemIDs = map[int]struct{}{}
		stripScanCounts = map[string]int{}
		stripScanItemIDs = map[string][]int{}
		active = true
		a.AddLogMsg(fmt.Sprintf("[STRIP_DEBUG] packet-triggered scan init session=%d", stripScanSessionID))
		log.Printf("[STRIP_DEBUG] packet-triggered scan init session=%d", stripScanSessionID)
	}
	scanID := stripScanSessionID
	stripScanPageCount++
	currentPage := stripScanPageCount

	firstMainID, pageRecords, classQtys, classItemIDs := parseStripInfoPageRaw(rawData)

	pageRepeated := false
	if firstMainID != 0 {
		if _, seen := stripScanSeenItemIDs[firstMainID]; seen {
			pageRepeated = true
		} else {
			stripScanSeenItemIDs[firstMainID] = struct{}{}
		}
	}

	if !pageRepeated {
		for className, qty := range classQtys {
			stripScanCounts[className] += qty
			for className, ids := range classItemIDs {
				stripScanItemIDs[className] = append(stripScanItemIDs[className], ids...)
			}
		}
	}

	pageLimitReached := stripScanPageCount >= 25
	a.AddLogMsg(fmt.Sprintf("[STRIP_DEBUG] session=%d page=%d items=%d repeated=%t pageLimit=%t", scanID, currentPage, pageRecords, pageRepeated, pageLimitReached))
	log.Printf("[STRIP_DEBUG] session=%d page=%d items=%d repeated=%t pageLimit=%t", scanID, currentPage, pageRecords, pageRepeated, pageLimitReached)

	stripScanMu.Unlock()

	if pageRepeated || pageLimitReached {
		reason := "wrapped"
		if pageRepeated {
			reason = "repeated page"
		} else if pageLimitReached {
			reason = "page limit"
		}
		a.finalizeStripScan(scanID, reason)
		return
	}

	go func() {
		time.Sleep(stripNextDelay)
		sendGetStripRaw(a, stripGetNextPayload)
	}()

	a.AddLogMsg(fmt.Sprintf("[STRIP] continuing scan: page %d had %d item(s), requesting next after %s", currentPage, pageRecords, stripNextDelay))
	log.Printf("[STRIP] continuing scan: page %d had %d item(s), requesting next after %s", currentPage, pageRecords, stripNextDelay)
	return
}

func (a *App) finalizeStripScan(sessionID int, reason string) {
	stripScanMu.Lock()
	if !stripScanActive || stripScanSessionID != sessionID {
		a.AddLogMsg(fmt.Sprintf("[STRIP_DEBUG] finalize skipped session=%d active=%t currentSession=%d", sessionID, stripScanActive, stripScanSessionID))
		log.Printf("[STRIP_DEBUG] finalize skipped session=%d active=%t currentSession=%d", sessionID, stripScanActive, stripScanSessionID)
		stripScanMu.Unlock()
		return
	}

	items := buildStripScanItems()
	itemIDs := stripScanItemIDs
	pagesScanned := stripScanPageCount
	stripScanActive = false
	stripScanMu.Unlock()
	a.AddLogMsg(fmt.Sprintf("[STRIP_DEBUG] finalize session=%d reason=%s pages=%d", sessionID, reason, pagesScanned))
	log.Printf("[STRIP_DEBUG] finalize session=%d reason=%s pages=%d", sessionID, reason, pagesScanned)

	handItemsMu.Lock()
	currentHandItems = items
	currentHandItemIDs = itemIDs
	handItemsMu.Unlock()

	a.AddLogMsg(fmt.Sprintf("[STRIP] scan complete after %d page(s), reason=%s; hand item types=%d", pagesScanned, reason, len(items)))
	log.Printf("[STRIP] scan complete after %d page(s), reason=%s; hand item types=%d", pagesScanned, reason, len(items))
	for i, item := range items {
		a.AddLogMsg(fmt.Sprintf("[STRIP] hand[%d] name=%q qty=%d", i, item.Name, item.Quantity))
		log.Printf("[STRIP] hand[%d] name=%q qty=%d", i, item.Name, item.Quantity)
	}
	a.emitHandItemsUpdate()
}

func (a *App) handleOutgoingGetStrip(e *g.Intercept) {
	if e.Packet.Header.Dir != g.Out {
		return
	}
	payload := strings.TrimSpace(string(e.Packet.Data))
	a.AddLogMsg(fmt.Sprintf("[STRIP_DEBUG] outgoing GETSTRIP[65] payload=%q", payload))
	log.Printf("[STRIP_DEBUG] outgoing GETSTRIP[65] payload=%q", payload)
}

func sendGetStripRaw(a *App, payload string) {
	trimmed := strings.TrimSpace(payload)
	ext.Send(g.Out.Id("GETSTRIP"), []byte(trimmed))
	a.AddLogMsg(fmt.Sprintf("[STRIP_DEBUG] raw GETSTRIP send payload=%q", trimmed))
	log.Printf("[STRIP_DEBUG] raw GETSTRIP send payload=%q", trimmed)
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
		})
	}

	return items
}

// diffItems returns the items in `all` that exceed the quantities in `subtract`.

// parseStripInfoPageRaw decodes a STRIPINFO_2 packet body (e.Packet.Data) using
// the real Shockwave grouped format.
// Each record groups all physical items of the same class:
//
//	Field 1 (until \x02): [mainItemId VL64][extraCount VL64]([extraItemId VL64]×N)[Pos VL64][S|I]
//	Field 2 (until \x02): [templateId VL64][VL64][VL64][className string]
//	Field 3 (until \x02): for "S": [DimX VL64][DimY VL64][Colors string]
//	                       for "I": [Props string]
//
// Quantity per record = 1 + extraCount.
// Returns: firstMainID (for wrap detection), record count, className→quantity map.
func parseStripInfoPageRaw(data []byte) (firstMainID int, pageRecords int, classQtys map[string]int, classItemIDs map[string][]int) {
	classQtys = map[string]int{}
	classItemIDs = map[string][]int{}
	pos := 0

	readVL64 := func() (int, bool) {
		if pos >= len(data) {
			return 0, false
		}
		n := gencoding.VL64DecodeLen(data[pos])
		if n <= 0 || pos+n > len(data) {
			return 0, false
		}
		v := gencoding.VL64Decode(data[pos : pos+n])
		pos += n
		return v, true
	}

	skipUntilDelim := func() {
		for pos < len(data) && data[pos] != 0x02 {
			pos++
		}
		if pos < len(data) {
			pos++ // skip \x02
		}
	}

	// record count
	count, ok := readVL64()
	if !ok {
		return
	}
	pageRecords = count

	for i := 0; i < count; i++ {
		if pos >= len(data) {
			break
		}

		// --- Field 1 ---
		mainID, ok := readVL64()
		if !ok {
			break
		}
		if i == 0 {
			firstMainID = mainID
		}

		extraCount, ok := readVL64()
		if !ok {
			break
		}
		extraIDs := make([]int, 0, extraCount)
		for j := 0; j < extraCount; j++ {
			if extraID, ok := readVL64(); ok {
				extraIDs = append(extraIDs, extraID)
			} else {
				break
			}
		}

		if _, ok := readVL64(); !ok { // Pos
			break
		}

		if pos >= len(data) {
			break
		}
		typeChar := data[pos]
		pos++
		skipUntilDelim() // eat remainder of field 1

		// --- Field 2 ---
		if _, ok := readVL64(); !ok { // templateId
			break
		}
		if _, ok := readVL64(); !ok { // extra field 0
			break
		}
		if _, ok := readVL64(); !ok { // extra field 1
			break
		}

		classStart := pos
		for pos < len(data) && data[pos] != 0x02 {
			pos++
		}
		classRaw := strings.ToLower(string(data[classStart:pos]))
		if pos < len(data) {
			pos++ // skip \x02
		}

		// --- Field 3 ---
		switch typeChar {
		case 'S':
			readVL64()       // DimX
			readVL64()       // DimY
			skipUntilDelim() // Colors
		case 'I':
			skipUntilDelim() // Props
		default:
			skipUntilDelim()
		}

		normalizedClass, ok := normalizeClassKeyWithVariant(classRaw)
		if !ok {
			continue
		}

		classQtys[normalizedClass] += 1 + extraCount
		classItemIDs[normalizedClass] = append(classItemIDs[normalizedClass], mainID)
		classItemIDs[normalizedClass] = append(classItemIDs[normalizedClass], extraIDs...)
	}
	return
}

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

func extractTradeFieldNameRaw(field string) (string, bool) {
	// Legacy format example: "itkoHP|club_sofa"
	if strings.Contains(field, "|") {
		parts := strings.Split(field, "|")
		if len(parts) >= 2 {
			if name, ok := normalizeTradeFieldClassRaw(parts[len(parts)-1]); ok {
				return name, true
			}
		}
	}

	// Current format example: "irbUAXb{chair_plasty*109"
	if strings.Contains(field, "{") {
		parts := strings.SplitN(field, "{", 2)
		if len(parts) == 2 {
			if name, ok := normalizeTradeFieldClassRaw(parts[1]); ok {
				return name, true
			}
		}
	}

	return "", false
}

func normalizeTradeFieldClassRaw(raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", false
	}

	if star := strings.Index(raw, "*"); star >= 0 {
		raw = raw[:star]
	}

	return normalizeTradeItemName(raw)
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

	return name, qty, true
}

// extractStripItemName extracts the furniture class name from a STRIPINFO_2 field.
func extractStripItemName(field string) (string, bool) {
	name, _, ok := extractStripItemAndQuantity(field)
	return name, ok
}

func extractStripItemAndQuantity(field string) (string, int, bool) {
	// Try existing trade formats first (handles | and { delimiters).
	if name, ok := extractTradeFieldNameRaw(field); ok {
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
	msg := fmt.Sprintf("Total \"%s\" available \"%d\" but payout needs \"%d\": please offer less.", formatTradeItemName(primary.Name), primary.Have, primary.PayoutTotal)
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

	incomingByName := map[string]int{}
	for _, item := range partnerItems {
		incomingByName[item.Name] += item.Quantity
	}

	shortages := make([]tradeShortage, 0)
	for _, item := range partnerItems {
		required := item.Quantity
		payoutTotal := item.Quantity * 2
		// Payout happens after this trade completes, so include incoming bet items.
		have := haveByName[item.Name] + incomingByName[item.Name]
		if have < payoutTotal {
			shortages = append(shortages, tradeShortage{
				Name:        item.Name,
				Required:    required,
				Have:        have,
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
	tradeItemsMu.Lock()
	pokerGameBetItems = make([]TradeItem, len(currentTradeItems))
	copy(pokerGameBetItems, currentTradeItems)
	tradeItemsMu.Unlock()

	if len(pokerGameBetItems) == 0 {
		a.AddLogMsg("[TRADE_MESSAGE] no items detected in trade, continuing anyway")
		log.Printf("[TRADE_MESSAGE] no items detected in trade, continuing anyway")
	} else {
		a.AddLogMsg(fmt.Sprintf("[TRADE_MESSAGE] recorded %d bet item type(s) for payout", len(pokerGameBetItems)))
	}

	partnerName := strings.TrimSpace(lastTradePartnerName)
	if partnerName == "" || strings.EqualFold(partnerName, "Unknown") {
		if resolved, ok := lookupUsers28Index(lastTradePartnerID); ok {
			partnerName = strings.TrimSpace(resolved)
			lastTradePartnerName = partnerName
		} else if resolved, ok := lookupUsers28Token(lastTradePartnerToken); ok {
			partnerName = strings.TrimSpace(resolved)
			lastTradePartnerName = partnerName
		} else if resolved, ok := lookupRoomEntityNameByIndex(lastTradePartnerID); ok {
			partnerName = strings.TrimSpace(resolved)
			lastTradePartnerName = partnerName
		}
	}
	if partnerName == "" || strings.EqualFold(partnerName, "Unknown") {
		partnerName = "Player"
	}

	first := fmt.Sprintf("%s what game do you want to play?", partnerName)
	second := "Say Poker, 21, 13"
	awaitingGameChoice = true
	awaitingGameChoicePartnerName = strings.TrimSpace(lastTradePartnerName)
	// Prefer the live room entity index for chat sender matching.
	// USERS28 indices are often larger room ids and can differ from chat indices.
	if chatIdx, ok := lookupRoomEntityIndexByName(awaitingGameChoicePartnerName); ok && chatIdx > 0 {
		awaitingGameChoicePartnerID = chatIdx
	} else if chatIdx, ok := lookupUsers28NameIndex(awaitingGameChoicePartnerName); ok && chatIdx > 0 {
		awaitingGameChoicePartnerID = chatIdx
	} else {
		awaitingGameChoicePartnerID = lastTradePartnerID
	}

	a.AddLogMsg(fmt.Sprintf("[TRADE_MESSAGE] shouting: %q", first))
	log.Printf("[TRADE_MESSAGE] shouting: %q", first)
	ext.Send(out.SHOUT, first)

	go func(msg string) {
		time.Sleep(1750 * time.Millisecond)
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
	Token      string
	ShortToken string
	Name       string
	RoomIndex  int
}

func extractUsers28Entries(raw string) []user28Entry {
	b := []byte(raw)
	entries := make([]user28Entry, 0)
	seen := map[string]struct{}{}

	for i := 0; i < len(b)-6; i++ {
		if b[i] != 0x02 {
			continue
		}

		// Name must end at this delimiter.
		nameEnd := i
		nameStart := nameEnd
		for nameStart > 0 && isLikelyNameChar(b[nameStart-1]) {
			nameStart--
		}
		if nameEnd-nameStart < 2 {
			continue
		}

		// Shockwave often prefixes names with a length marker (e.g. MWebsedit).
		// If first two chars are uppercase, drop the first byte as the marker.
		adjNameStart := nameStart
		if nameEnd-nameStart >= 3 && b[nameStart] >= 'A' && b[nameStart] <= 'Z' && b[nameStart+1] >= 'A' && b[nameStart+1] <= 'Z' {
			adjNameStart = nameStart + 1
		}

		name := strings.TrimSpace(string(b[adjNameStart:nameEnd]))
		if len(name) < 2 {
			continue
		}

		roomIndex := 0
		for startOff := adjNameStart - 1; startOff >= 0 && startOff >= adjNameStart-8; startOff-- {
			vlen := gencoding.VL64DecodeLen(b[startOff])
			if vlen > 0 && vlen <= 6 && startOff+vlen == adjNameStart {
				v := gencoding.VL64Decode(b[startOff : startOff+vlen])
				if v > 0 {
					roomIndex = v
					break
				}
			}
		}
		if roomIndex <= 0 {
			continue
		}

		token := ""
		if nameStart >= 4 {
			candidate := string(b[nameStart-4 : nameStart])
			if isLikelyToken(candidate) {
				token = candidate
			}
		}

		shortToken := ""
		if adjNameStart >= 6 {
			shortCandidate := string(b[adjNameStart-6 : adjNameStart-4])
			if isLikelyChatToken(shortCandidate) {
				shortToken = shortCandidate
			}
		}

		key := fmt.Sprintf("%d|%s|%s", roomIndex, strings.ToLower(name), shortToken)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		entries = append(entries, user28Entry{
			Token:      token,
			ShortToken: shortToken,
			Name:       name,
			RoomIndex:  roomIndex,
		})
	}

	return entries
}

func parseUsers28Head(part string) (name string, token string, roomIndex int, ok bool) {
	if len(part) < 7 {
		return "", "", 0, false
	}

	nameStart := len(part)
	for nameStart > 0 && isLikelyNameChar(part[nameStart-1]) {
		nameStart--
	}

	if nameStart < 5 || nameStart >= len(part) {
		return "", "", 0, false
	}

	name = part[nameStart:]
	if len(name) < 2 {
		return "", "", 0, false
	}

	token = part[nameStart-4 : nameStart]
	if !isLikelyToken(token) {
		return "", "", 0, false
	}

	// Try to decode a VL64 room index from the bytes immediately before the token.
	// In Shockwave USERS packets the entry layout is: [roomIndex VL64][name string] ...
	// The 4-byte legacy token appears just before the name; the VL64 may start before it.
	tokenOffset := nameStart - 4
	for startOff := tokenOffset - 1; startOff >= 0 && startOff >= tokenOffset-6; startOff-- {
		b := part[startOff]
		vlen := gencoding.VL64DecodeLen(b)
		if vlen > 0 && startOff+vlen == tokenOffset {
			v := gencoding.VL64Decode([]byte(part[startOff : startOff+vlen]))
			if v > 0 {
				roomIndex = v
			}
			break
		}
	}

	// Also try decoding the token itself as a VL64 room index (older format).
	if roomIndex == 0 {
		tb := []byte(token)
		vlen := gencoding.VL64DecodeLen(tb[0])
		if vlen > 0 && vlen <= 4 {
			v := gencoding.VL64Decode(tb[:vlen])
			if v > 0 {
				roomIndex = v
			}
		}
	}

	return name, token, roomIndex, true
}

func isLikelyNameChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

func isLikelyFigureField(field string) bool {
	f := strings.ToLower(strings.TrimSpace(field))
	if len(f) < 12 {
		return false
	}

	if !(strings.HasPrefix(f, "hr-") || strings.HasPrefix(f, "hd-")) {
		return false
	}

	if !strings.Contains(f, "hd-") || !strings.Contains(f, "ch-") || !strings.Contains(f, "lg-") || !strings.Contains(f, "sh-") {
		return false
	}

	return true
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
	users28Mu.Lock()
	clear(users28ByToken)
	clear(users28ByIndex)
	clear(users28ByShortToken)
	clear(roomIdentityByShortToken)
	users28Mu.Unlock()
	a.emitRoomIdentityUpdate()
	a.AddLogMsg("[ROOM_USERS] cleared cached room users")
	log.Printf("[ROOM_USERS] cleared cached room users")
	go requestRoomUsers(a)
}

func (a *App) handleRoomUsers(e *g.Intercept) {
	defer func() {
		if r := recover(); r != nil {
			a.AddLogMsg(fmt.Sprintf("[ROOM_USERS] failed to parse packet %d: %v", e.Packet.Header.Value, r))
			log.Printf("[ROOM_USERS] failed to parse packet %d: %v", e.Packet.Header.Value, r)
		}
	}()

	a.AddLogMsg(fmt.Sprintf("[ROOM_USERS] received packet %d len=%d", e.Packet.Header.Value, len(e.Packet.Data)))
	log.Printf("[ROOM_USERS] received packet %d len=%d", e.Packet.Header.Value, len(e.Packet.Data))

	// In this client, header 28 often uses raw USERS28 layout, not room.Entity wire format.
	// That payload is handled by handleUsers28Packet; skip structured decode here.
	if e.Packet.Header.Value == 28 {
		a.AddLogMsg("[ROOM_USERS] header 28 uses raw USERS28 layout; skipping room.Entity decode")
		log.Printf("[ROOM_USERS] header 28 uses raw USERS28 layout; skipping room.Entity decode")
		return
	}

	count := e.Packet.ReadInt()
	parsedUsers := map[int]room.Entity{}

	for range count {
		var entity room.Entity
		e.Packet.Read(&entity)
		if entity.Type == room.User {
			parsedUsers[entity.Index] = entity
		}
	}

	// Keep USERS28-like caches warm from the structured USERS list too.
	users28Mu.Lock()
	for idx, entity := range parsedUsers {
		name := strings.TrimSpace(entity.Name)
		token := ""
		if entityToken, clean, ok := splitTokenAndName(name); ok {
			name = clean
			token = entityToken
			users28ByToken[entityToken] = clean
			for _, short := range shortTokenCandidates(entityToken) {
				users28ByShortToken[short] = clean
			}
		}
		if name != "" {
			users28ByIndex[idx] = name
			if short := shortTokenFromIndex(idx); isLikelyChatToken(short) {
				users28ByShortToken[short] = name
				roomIdentityByShortToken[short] = RoomIdentityEntry{
					Name:      name,
					Token:     token,
					Short:     short,
					ChatIndex: idx,
					RoomIndex: idx,
				}
			}
		}
	}
	users28Mu.Unlock()
	a.emitRoomIdentityUpdate()

	roomMu.Lock()
	for index, entity := range parsedUsers {
		roomEntities[index] = entity
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

	roomUsersReqMu.Lock()
	lastRoomUsersRequestAt = time.Now()
	roomUsersReqMu.Unlock()

	// G_USRS is the packet this client uses to request the in-room USERS list (header 61).
	a.ext.Send(out.G_USRS)
	// Keep legacy request as a secondary path in case the server expects both in some sessions.
	a.ext.Send(out.GETSPACENODEUSERS)
	startIncomingHeaderSniff(8 * time.Second)
	a.AddLogMsg("[ROOM_USERS] requested current room users via G_USRS + GETSPACENODEUSERS")
	log.Printf("[ROOM_USERS] requested current room users via G_USRS + GETSPACENODEUSERS")
}

func shouldRefreshRoomUsers() bool {
	roomUsersReqMu.Lock()
	last := lastRoomUsersRequestAt
	roomUsersReqMu.Unlock()
	return time.Since(last) > 5*time.Second
}

func (a *App) emitRoomIdentityUpdate() {
	users28Mu.Lock()
	entries := make([]RoomIdentityEntry, 0, len(roomIdentityByShortToken))
	for _, entry := range roomIdentityByShortToken {
		entries = append(entries, entry)
	}
	users28Mu.Unlock()

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ChatIndex == entries[j].ChatIndex {
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		}
		if entries[i].ChatIndex <= 0 {
			return false
		}
		if entries[j].ChatIndex <= 0 {
			return true
		}
		return entries[i].ChatIndex < entries[j].ChatIndex
	})

	jsonData, err := json.Marshal(entries)
	if err != nil {
		a.AddLogMsg(fmt.Sprintf("[ROOM_USERS] failed to marshal room identity update: %v", err))
		log.Printf("[ROOM_USERS] failed to marshal room identity update: %v", err)
		return
	}
	runtime.EventsEmit(a.ctx, "roomIdentityUpdate", string(jsonData))
}

func resolveChatSenderName(index int) (string, bool) {
	if index <= 0 {
		return "", false
	}

	// Prefer the structured room user cache.
	roomMu.Lock()
	entity, ok := roomEntities[index]
	roomMu.Unlock()
	if ok {
		name := strings.TrimSpace(entity.Name)
		if _, clean, ok2 := splitTokenAndName(name); ok2 {
			name = clean
		}
		if name != "" {
			return name, true
		}
	}

	// Fall back to the USERS28 scan cache.
	name, ok := lookupUsers28Index(index)
	if ok {
		return name, true
	}

	// Fall back to live room identity map keyed by short token -> name.
	if mappedName, mappedOk := lookupRoomIdentityByChatIndex(index); mappedOk {
		return mappedName, true
	}

	// Final fallback: match via 2-char chat token (e.g. "SD", "QD", "RD").
	short := shortTokenFromIndex(index)
	if !isLikelyChatToken(short) {
		return "", false
	}
	users28Mu.Lock()
	name, ok = users28ByShortToken[short]
	users28Mu.Unlock()
	return name, ok
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

func decodeLeadingVL64(data []byte) (int, bool) {
	if len(data) == 0 {
		return 0, false
	}
	vlen := gencoding.VL64DecodeLen(data[0])
	if vlen <= 0 || vlen > 6 || vlen > len(data) {
		return 0, false
	}
	v := gencoding.VL64Decode(data[:vlen])
	if v <= 0 {
		return 0, false
	}
	return v, true
}

func packetContainsVL64Value(data []byte, value int) bool {
	if value <= 0 || len(data) == 0 {
		return false
	}

	for i := 0; i < len(data); i++ {
		vlen := gencoding.VL64DecodeLen(data[i])
		if vlen <= 0 || vlen > 6 || i+vlen > len(data) {
			continue
		}
		if gencoding.VL64Decode(data[i:i+vlen]) == value {
			return true
		}
	}

	return false
}

func rememberOutgoingTradeOpenTarget(targetID int) {
	if targetID <= 0 {
		return
	}
	tradeOpenStateMu.Lock()
	lastOutgoingTradeOpenID = targetID
	lastOutgoingTradeOpenAt = time.Now()
	tradeOpenStateMu.Unlock()
}

func matchesRecentOutgoingTradeOpen(data []byte, leadingIncomingID int) (int, bool) {
	tradeOpenStateMu.Lock()
	targetID := lastOutgoingTradeOpenID
	at := lastOutgoingTradeOpenAt
	tradeOpenStateMu.Unlock()

	if targetID <= 0 {
		return 0, false
	}

	if time.Since(at) > 8*time.Second {
		return targetID, false
	}

	if leadingIncomingID > 0 && leadingIncomingID == targetID {
		return targetID, true
	}

	if packetContainsVL64Value(data, targetID) {
		return targetID, true
	}

	return targetID, false
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
	commandMsg := extractInlineCommand(msg)

	// Process commands based on the message prefix and suffix
	if strings.HasPrefix(commandMsg, ":") {
		// Check if already rolling or closing
		if isPokerRolling || isTriRolling || isBJRolling || is13Rolling || isHitting || is13Hitting || isClosing {
			log.Println("Already rolling or closing...")
			e.Block()
			return
		}

		command := strings.TrimPrefix(commandMsg, ":")
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

func extractInlineCommand(msg string) string {
	trimmed := strings.TrimSpace(msg)
	if strings.HasPrefix(trimmed, ":") {
		return trimmed
	}

	idx := strings.Index(trimmed, ":")
	if idx < 0 {
		return trimmed
	}

	return strings.TrimSpace(trimmed[idx:])
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

func (a *App) beginPokerSequence() {
	playerName := strings.TrimSpace(lastTradePartnerName)
	if playerName == "" {
		playerName = "Player"
	}

	resetPokerSequence()
	pokerSequenceStage = 1
	pokerSequencePlayerName = playerName

	first := "Lets Play!"
	second := fmt.Sprintf("%s Roll", playerName)

	a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] shouting: %q", first))
	log.Printf("[GAME_SELECT] shouting: %q", first)
	ext.Send(out.SHOUT, first)

	go func(msg string) {
		time.Sleep(700 * time.Millisecond)
		a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] shouting: %q", msg))
		log.Printf("[GAME_SELECT] shouting: %q", msg)
		ext.Send(out.SHOUT, msg)

		time.Sleep(700 * time.Millisecond)
		a.startPokerRoll()
	}(second)
}

// Reset all saved dice states
func resetDiceState() {
	mutex.Lock()
	defer mutex.Unlock()
	resultsWaitGroup.Wait() // Ensure all dice roll results are processed
	diceList = []*Dice{}
	awaitingTradeOpen = false
	dealerTradeWindowOpen = false
	resetPokerSequence()
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
	if canAnnounceDealerOpenLocked() {
		dealerTradeWindowOpen = true
		go sendMessageWithDelay(a.dealerOpenMessage())
	} else {
		dealerTradeWindowOpen = false
		log.Printf("User is muted. Dealer open announcement skipped; incoming trades will be blocked.")
	}
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
			if canAnnounceDealerOpenLocked() {
				dealerTradeWindowOpen = true
				go sendMessageWithDelay(a.dealerOpenMessage())
			} else {
				dealerTradeWindowOpen = false
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

	senderName, senderOk := resolveChatSenderName(index)
	if !senderOk && shouldRefreshRoomUsers() {
		go requestRoomUsers(a)
	}

	if senderOk {
		log.Printf("[INCOMING %s] %s(%d) -> %s", chatType, senderName, index, msg)
		a.AddChatLog(fmt.Sprintf("[IN %s] %s(%d) -> %s", chatType, senderName, index, msg))
	} else {
		log.Printf("[INCOMING %s] %d -> %s", chatType, index, msg)
		a.AddChatLog(fmt.Sprintf("[IN %s] %d -> %s", chatType, index, msg))
	}

	if !awaitingGameChoice {
		return
	}

	choice, ok := normalizeIncomingGameChoice(msg)
	if !ok {
		return
	}

	// senderName already resolved above.

	// Accept only if sender matches by room index OR by name.
	indexMatch := awaitingGameChoicePartnerID > 0 && index == awaitingGameChoicePartnerID
	nameMatch := awaitingGameChoicePartnerName != "" && strings.EqualFold(senderName, awaitingGameChoicePartnerName)
	// Fallback 1: stored ID may be a USERS28/virtual id instead of chat index;
	// look up the partner's chat index via roomEntities by name.
	if !indexMatch && !nameMatch && awaitingGameChoicePartnerName != "" {
		if expectedIdx, ok := lookupRoomEntityIndexByName(awaitingGameChoicePartnerName); ok && expectedIdx > 0 && expectedIdx == index {
			indexMatch = true
		}
	}
	// Fallback 2: look up the partner's chat index via the users28ByIndex cache.
	if !indexMatch && !nameMatch && awaitingGameChoicePartnerName != "" {
		if expectedIdx, ok := lookupUsers28NameIndex(awaitingGameChoicePartnerName); ok && expectedIdx > 0 && expectedIdx == index {
			indexMatch = true
		}
	}
	// Fallback 3: if this incoming index maps to the same trade partner name, accept it.
	if !indexMatch && !nameMatch {
		partnerName := strings.TrimSpace(awaitingGameChoicePartnerName)
		if partnerName == "" || strings.EqualFold(partnerName, "Unknown") {
			partnerName = strings.TrimSpace(lastTradePartnerName)
		}
		if partnerName != "" && !strings.EqualFold(partnerName, "Unknown") {
			if mappedName, ok := lookupRoomIdentityByChatIndex(index); ok {
				if strings.EqualFold(strings.TrimSpace(mappedName), partnerName) {
					nameMatch = true
					senderName = mappedName
					a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] accepted %q by partner-name match: index %d -> %q", choice, index, mappedName))
					log.Printf("[GAME_SELECT] accepted %q by partner-name match: index %d -> %q", choice, index, mappedName)
				}
			}
		}
	}
	if !indexMatch && !nameMatch {
		partnerUnknown := strings.TrimSpace(awaitingGameChoicePartnerName) == "" || strings.EqualFold(strings.TrimSpace(awaitingGameChoicePartnerName), "Unknown")
		senderKnown := strings.TrimSpace(senderName) != "" && !strings.EqualFold(strings.TrimSpace(senderName), "Unknown")
		if partnerUnknown {
			if senderKnown {
				nameMatch = true
				awaitingGameChoicePartnerName = strings.TrimSpace(senderName)
				lastTradePartnerName = strings.TrimSpace(senderName)
				a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] accepted %q from resolved sender %q while partner name was unknown", choice, senderName))
				log.Printf("[GAME_SELECT] accepted %q from resolved sender %q while partner name was unknown", choice, senderName)
			} else if (awaitingGameChoicePartnerID <= 0 || awaitingGameChoicePartnerID > 512) && index > 0 && index <= 512 {
				// Last-resort path when trade partner id is unresolved or in a different id space.
				indexMatch = true
				a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] accepted %q by index fallback (incoming=%d expected=%d partner=%q)", choice, index, awaitingGameChoicePartnerID, awaitingGameChoicePartnerName))
				log.Printf("[GAME_SELECT] accepted %q by index fallback (incoming=%d expected=%d partner=%q)", choice, index, awaitingGameChoicePartnerID, awaitingGameChoicePartnerName)
			}
		}
	}
	if !indexMatch && !nameMatch {
		a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] ignoring %q from %q (index %d); waiting for %q (index %d)", choice, senderName, index, awaitingGameChoicePartnerName, awaitingGameChoicePartnerID))
		log.Printf("[GAME_SELECT] ignoring %q from %q (index %d); waiting for %q (index %d)", choice, senderName, index, awaitingGameChoicePartnerName, awaitingGameChoicePartnerID)
		return
	}

	if strings.TrimSpace(senderName) != "" && (strings.TrimSpace(lastTradePartnerName) == "" || strings.EqualFold(strings.TrimSpace(lastTradePartnerName), "Unknown")) {
		lastTradePartnerName = strings.TrimSpace(senderName)
		a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] backfilled trade partner name from chat sender: %q", lastTradePartnerName))
		log.Printf("[GAME_SELECT] backfilled trade partner name from chat sender: %q", lastTradePartnerName)
	}

	e.Block()
	if isPokerRolling || isTriRolling || isBJRolling || is13Rolling || isHitting || is13Hitting || isClosing {
		a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] %s selected but dice are busy", choice))
		log.Printf("[GAME_SELECT] %s selected but dice are busy", choice)
		return
	}

	awaitingGameChoice = false
	awaitingGameChoicePartnerID = 0
	awaitingGameChoicePartnerName = ""

	ack := fmt.Sprintf("%s! Lets Play!", gameChoiceDisplay(choice))
	a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] shouting: %q", ack))
	log.Printf("[GAME_SELECT] shouting: %q", ack)
	ext.Send(out.SHOUT, ack)

	switch choice {
	case "poker":
		a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] %d selected Poker; starting player/dealer poker sequence", index))
		log.Printf("[GAME_SELECT] %d selected Poker; starting player/dealer poker sequence", index)
		a.beginPokerSequence()
	case "21":
		resetPokerSequence()
		a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] %d selected 21; starting internal roll", index))
		log.Printf("[GAME_SELECT] %d selected 21; starting internal roll", index)
		isBJRolling = true
		a.AddLogMsg("21 Roll:\n")
		go a.rollBjDice()
	case "13":
		resetPokerSequence()
		a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] %d selected 13; starting internal roll", index))
		log.Printf("[GAME_SELECT] %d selected 13; starting internal roll", index)
		is13Rolling = true
		a.AddLogMsg("13 Roll:\n")
		go a.roll13Dice()
	}
}

func normalizeIncomingGameChoice(msg string) (string, bool) {
	cleaned := strings.ToLower(strings.TrimSpace(msg))
	cleaned = gameChoiceCleanupRe.ReplaceAllString(cleaned, "")
	switch cleaned {
	case "poker":
		return "poker", true
	case "21":
		return "21", true
	case "13":
		return "13", true
	default:
		return "", false
	}
}

func gameChoiceDisplay(choice string) string {
	switch choice {
	case "poker":
		return "Poker"
	case "21":
		return "21"
	case "13":
		return "13"
	default:
		return choice
	}
}
