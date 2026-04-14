package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
	"xabbo.b7c.io/goearth/shockwave/in"
	"xabbo.b7c.io/goearth/shockwave/inventory"
	"xabbo.b7c.io/goearth/shockwave/trade"
)

var lastTrade trade.Offers
var messageSent bool

func init() {
	tradeMgr.Completed(handleTradeComplete)
}

type TrackerData struct {
	Furni            string `json:"furni"`
	Note             string `json:"note"`
	Quantity         int    `json:"quantity"`
	Result           string `json:"result"`
	AddedByExtension bool   `json:"addedByExtension"`
}

func sendTrackerData(offers trade.Offers) {
	itemCounts := make(map[string]map[string]int)

	// Log details of offers
	for _, offer := range offers {
		for _, item := range offer.Items {
			if itemCounts[item.Class] == nil {
				itemCounts[item.Class] = make(map[string]int)
			}
			itemCounts[item.Class][strconv.Itoa(offer.UserId)]++
		}
	}

	for className, owners := range itemCounts {
		for owner, count := range owners {
			result := "won"
			if owner == strconv.Itoa(profileMgr.UserId) {
				result = "lose"
			}

			currentTime := time.Now().Format("2006-01-02 15:04:05")

			note := ""

			roomName := roomMgr.Info().Name
			if roomName == "" {
				roomName = "Unknown"
			}

			if offers[1].UserId == profileMgr.UserId {
				entity := roomMgr.EntityByUserId(offers[0].UserId)
				name := strconv.Itoa(offers[0].UserId)
				if entity != nil {
					name = entity.Name
				}
				note = "Traded with " + name + " at " + currentTime + " at room " + roomName
			} else {
				entity := roomMgr.EntityByUserId(offers[1].UserId)
				name := strconv.Itoa(offers[1].UserId)
				if entity != nil {
					name = entity.Name
				}
				note = "Traded with " + name + " at " + currentTime + " at room " + roomName
			}

			data := TrackerData{
				Furni:            className,
				Note:             note,
				Quantity:         count,
				Result:           result,
				AddedByExtension: true,
			}

			dataBytes, err := json.Marshal(data)
			if err != nil {
				log.Println("Error marshaling JSON:", err)
				continue
			}

			req, err := http.NewRequest("POST", "https://legacyhabbo.me/tracker/add", bytes.NewBuffer(dataBytes))
			if err != nil {
				log.Println("Error creating POST request:", err)
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-AUTH-TOKEN", authToken)

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				log.Println("Error sending POST request:", err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				ext.Send(in.SYSTEM_BROADCAST, []byte("There was a issue with the tracker, contact the developer."))
				log.Printf("Received non-OK response: %s\n", resp.Status)
			}
		}
	}
}

func handleTradeComplete(args trade.Args) {
	lastTrade = args.Offers

	if !notifyTrade && authToken == "" {
		ext.Send(in.SYSTEM_BROADCAST, []byte("Trade completed, but this trade wasn't tracked, if you want to track through webpage your wins/losses\n\nType :authenticate to enable.\n\nYou then can check in https://tsarcade.com your trade history and use TC or Bobba Pro as a price API."))
		notifyTrade = true
	}

	if enabled {
		go sendTrackerData(lastTrade)

		if !messageSent {
			ext.Send(in.SYSTEM_BROADCAST, []byte("Tracker is enabled, so this trade was tracked, you can disable by typing :tracker.\nYou can check it in https://tsarcade.com"))
			messageSent = true
		}
	}

	var items []trade.TradeItem
	var otherItems []trade.TradeItem
	if tradeMgr.Offers.Tradee().UserId == profileMgr.UserId {
		items = tradeMgr.Offers.Tradee().Items
		otherItems = tradeMgr.Offers.Trader().Items
	} else {
		items = tradeMgr.Offers.Trader().Items
		otherItems = tradeMgr.Offers.Tradee().Items
	}

	for _, item := range items {
		for i, countItem := range counts {
			if countItem.Class == item.Class {
				counts[i].Count--
				break
			}
		}
	}

	for _, item := range otherItems {
		for i, countItem := range counts {
			if countItem.Class == item.Class {
				counts[i].Count++
				break
			} else {
				item := inventory.Item{
					ItemId: item.ItemId,
					Type:   inventory.ItemType(item.Type),
					Class:  item.Class,
				}

				counts = append(counts, CountItem{
					Name:  getFullName(item),
					Count: 1,
					Class: item.Class,
				})
			}
		}
	}

}
