package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	g "xabbo.b7c.io/goearth"
	"xabbo.b7c.io/goearth/shockwave/in"
	"xabbo.b7c.io/goearth/shockwave/inventory"
	"xabbo.b7c.io/goearth/shockwave/out"
	"xabbo.b7c.io/goearth/shockwave/profile"
	"xabbo.b7c.io/goearth/shockwave/room"
	"xabbo.b7c.io/goearth/shockwave/trade"
)

var ext = g.NewExt(g.ExtInfo{
	Title:       "Trade Tracker",
	Description: "TSA's Trade Tracker",
	Version:     "2.0",
	Author:      "Thauan",
})

var profileMgr = profile.NewManager(ext)
var tradeMgr = trade.NewManager(ext)
var roomMgr = room.NewManager(ext)
var inventoryMgr = inventory.NewManager(ext)
var authToken = ""
var isAuthenticating = false
var enabled = false
var host string
var currentMotto string
var addItem string
var addItemQty int
var looping = false
var notifyTrade = false
var checkQty = false
var counts []CountItem

func main() {

	ext.Intercept(out.CHAT, out.SHOUT, out.WHISPER).With(handleCommand)

	ext.Intercept(out.TRADE_ADDITEM).With(interceptTradeAddItem)
	ext.Intercept(in.TRADE_CLOSE).With(interceptTradeClose)
	ext.Intercept(in.ACTIVEOBJECT_ADD).With(interceptAddItem)
	ext.Intercept(in.ACTIVEOBJECT_REMOVE).With(interceptRemoveItem)

	ext.Connected(func(e g.ConnectArgs) {
		authToken = ""

		loadExternalTexts(e.Host)

		if e.Host == "game-obr.habbo.com" {
			host = "BR"
		}
		if e.Host == "game-ous.habbo.com" {
			host = "COM"
		}
		if e.Host == "game-oes.habbo.com" {
			host = "ES"
		}

		go scanInventory(false)

		profileMgr.Updated(func(e profile.Args) {
			if authToken != "" || isAuthenticating {
				return
			}
			isAuthenticating = true
			currentMotto = profileMgr.CustomData
		})

	})

	ext.Run()

}

func interceptRemoveItem(e *g.Intercept) {
	e.Packet.ReadString()
	class := e.Packet.ReadString()

	for i, countItem := range counts {
		if countItem.Class == class {
			counts[i].Count++
			break
		}
	}
}

func interceptAddItem(e *g.Intercept) {
	e.Packet.ReadString()
	class := e.Packet.ReadString()

	for i, countItem := range counts {
		if countItem.Class == class {
			counts[i].Count--
			break
		}
	}
}

func interceptTradeAddItem(e *g.Intercept) {

	if looping {
		e.Block()
		return
	}

	furniId := e.Packet.ReadInt()

	var className string
	inventoryMgr.Items(func(item inventory.Item) bool {
		if item.ItemId == furniId {
			className = item.Class
			return false
		}
		return true
	})

	if className == "" {
		return
	}

	if addItemQty > 0 {
		addItem = className
		go loopTrader()
		return
	}

	if checkQty {
		for _, countItem := range counts {
			if countItem.Class == className {
				ext.Send(in.SYSTEM_BROADCAST, []byte(fmt.Sprintf("You have %d of %s.", countItem.Count, countItem.Name)))
				break
			}
		}

		checkQty = false
		addItem = ""
		addItemQty = 0
		e.Block()
	}
}

func interceptTradeClose(e *g.Intercept) {
	addItem = ""
	addItemQty = 0
}

func loopTrader() {
	if looping == false {
		if addItemQty > 0 && addItem != "" {
			looping = true
			go loopTraderAddItems()
		}
	}
}

func loopTraderAddItems() {

	for _, countItem := range counts {
		if countItem.Class == addItem && addItemQty > countItem.Count {
			ext.Send(in.SYSTEM_BROADCAST, []byte("You don't have enough items to trade, you have "+strconv.Itoa(countItem.Count)+" "+countItem.Name+"."))
			addItem = ""
			addItemQty = 0
			looping = false
			return
		}
	}

	inventoryMgr.Items(func(item inventory.Item) bool {
		if item.Class == addItem && addItemQty > 0 {
			time.Sleep(time.Millisecond * 550)

			var items []trade.TradeItem
			if tradeMgr.Offers.Tradee().UserId == profileMgr.UserId {
				items = tradeMgr.Offers.Tradee().Items
			} else {
				items = tradeMgr.Offers.Trader().Items
			}

			itemFound := false
			for _, tradeItem := range items {
				if tradeItem.ItemId == item.ItemId {
					itemFound = true
					break
				}
			}

			if !itemFound {
				tradeMgr.Offer(item.ItemId)
				addItemQty--
			}
		}
		return addItemQty > 0
	})
	looping = false
}

func handleCommand(e *g.Intercept) {
	msg := e.Packet.ReadString()
	args := strings.Split(msg, " ")
	if args[0] == ":tracker" {
		if authToken == "" {
			ext.Send(in.SYSTEM_BROADCAST, []byte("You must authenticate first, type :authenticate to try authenticating."))
			e.Block()
			return
		}

		if !enabled {
			enabled = true
			ext.Send(in.SYSTEM_BROADCAST, []byte("[TSA] Tracker enabled."))
		} else {
			enabled = false
			ext.Send(in.SYSTEM_BROADCAST, []byte("[TSA] Tracker disabled."))
		}
		e.Block()
	}

	if args[0] == ":authenticate" {
		currentMotto = profileMgr.CustomData
		go authenticateLegacy(profileMgr.Name, host)
		e.Block()
	}

	if args[0] == ":trade" {
		tradeAmount, err := strconv.Atoi(args[1])
		if err != nil {
			ext.Send(in.SYSTEM_BROADCAST, []byte("Invalid trade amount. Please enter a valid number."))
			return
		}
		if tradeAmount > 0 {
			addItemQty = tradeAmount - 1
		}
		e.Block()
	}

	if args[0] == ":qtd" {
		addItem = ""
		addItemQty = 0
		checkQty = true
		e.Block()
	}

	if args[0] == ":scan" {
		counts = []CountItem{}
		go scanInventory(true)
		e.Block()
	}

	if args[0] == ":count" {
		printCountResults(counts)
		e.Block()
	}

}

func scanInventory(printResult bool) {
	scan := inventoryMgr.Scan()
	<-scan.Done()
	if err := context.Cause(scan); !errors.Is(err, inventory.ErrScanSuccess) {
		return
	}
	ext.Send(in.SYSTEM_BROADCAST, []byte("Inventory scanned successfully."))
	retrieveAndProcessItems(printResult)
}

func retrieveAndProcessItems(printResult bool) {
	var allItems []inventory.Item

	inventoryMgr.Items(func(item inventory.Item) bool {
		allItems = append(allItems, item)
		name := getFullName(item)

		itemIndex := make(map[string]int)
		for i, countItem := range counts {
			itemIndex[countItem.Class] = i
		}

		if i, found := itemIndex[item.Class]; found {
			counts[i].Count++
		} else {
			counts = append(counts, CountItem{
				Name:  name,
				Count: 1,
				Class: item.Class,
			})
			itemIndex[item.Class] = len(counts) - 1
		}

		return true
	})

	if printResult {
		printCountResults(counts)
	}
}

func authenticateLegacy(habboName, server string) {
	log.Println("Starting authenticateLegacy function")
	data := map[string]string{
		"habboName": habboName,
		"server":    server,
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		log.Println("Error marshaling JSON:", err)
		return
	}
	log.Println("JSON marshaled successfully:", string(dataBytes))

	resp, err := http.Post("https://legacyhabbo.me/generate", "application/json", bytes.NewBuffer(dataBytes))
	if err != nil {
		log.Println("Error sending POST request:", err)
		return
	}
	defer resp.Body.Close()
	log.Println("POST request sent successfully")

	if resp.StatusCode != http.StatusOK {
		ext.Send(out.UPDATE, int16(6), currentMotto)
		ext.Send(out.UPDATE, int16(6), currentMotto)
		ext.Send(out.UPDATE, int16(6), currentMotto)
		log.Printf("Received non-OK response: %s\n", resp.Status)
		return
	}
	log.Println("Received OK response from server")

	var response map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Println("Error decoding response JSON:", err)
		return
	}
	log.Println("Response JSON decoded successfully:", response)

	oldMotto, ok := response["previousMotto"]
	if !ok {
		log.Println("Motto not found in response")
		return
	}

	motto, ok := response["motto"]
	if !ok {
		log.Println("Motto not found in response")
		return
	}
	log.Println("Motto found in response:", motto)

	ext.Send(out.UPDATE, int16(6), motto)
	log.Println("Sent UPDATE with motto " + motto)

	loginData := map[string]string{
		"habboName": habboName,
		"server":    server,
		"motto":     motto,
	}
	loginDataBytes, err := json.Marshal(loginData)
	if err != nil {
		log.Println("Error marshaling login JSON:", err)
		return
	}
	log.Println("Login JSON marshaled successfully:", string(loginDataBytes))

	loginResp, err := http.Post("https://legacyhabbo.me/login", "application/json", bytes.NewBuffer(loginDataBytes))
	if err != nil {
		ext.Send(out.UPDATE, int16(6), oldMotto)
		ext.Send(out.UPDATE, int16(6), oldMotto)
		ext.Send(out.UPDATE, int16(6), oldMotto)
		ext.Send(in.SYSTEM_BROADCAST, []byte("There was an error authenticating you in TSA Tracker."))
		log.Println("Error sending login POST request:", err)
		return
	}
	defer loginResp.Body.Close()
	log.Println("Login POST request sent successfully")

	if loginResp.StatusCode != http.StatusOK {

		log.Printf("Login received non-OK response for login: %s\n", loginResp.Status)

	} else {
		log.Println("Successfully sent login request")

		var loginResponse map[string]string
		if err := json.NewDecoder(loginResp.Body).Decode(&loginResponse); err != nil {
			log.Println("Error decoding login response JSON:", err)
			return
		}
		log.Println("Login response JSON decoded successfully:", loginResponse)

		if authToken, ok = loginResponse["authToken"]; ok {
			log.Println("authToken found in response:", authToken)
		} else {
			log.Println("authToken not found in response")
		}

		log.Println("Sent UPDATE with motto" + oldMotto)
		enabled = true
		ext.Send(in.SYSTEM_BROADCAST, []byte("You have been authenticated successfully in TSA Tracker and Tracker was enabled automatically, disable it by typing :tracker."))
		ext.Send(out.UPDATE, int16(6), oldMotto)
		ext.Send(out.UPDATE, int16(6), oldMotto)
		ext.Send(out.UPDATE, int16(6), oldMotto)
	}
}
