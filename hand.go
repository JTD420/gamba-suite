package main

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"xabbo.b7c.io/goearth/shockwave/out"
)

// Send message with a delay to simulate user typing/waiting
func sendMessageWithDelay(message string) {
	// sleep random between 250 and 500ms
	time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
	ext.Send(out.SHOUT, message)
	log.Printf("Sent message: %s", message)
}

// Wait for all dice results and evaluate the poker hand
func (a *App) evaluatePokerHand() {
	result := evaluatePokerRules(diceList)
	hand := a.toPokerString(diceList)
	logRollResult := fmt.Sprintf("Poker Result: %s\n", hand)
	time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
	a.AddLogMsg(logRollResult)

	if !ChatIsDisabled {
		if !isMuted {
			// If the user is not muted, send the message
			sendMessageWithDelay(hand)
		} else {
			// If the user is muted, queue the message to send later
			log.Printf("User is muted. Queuing message: %s", hand)
			// ToDo:
			// messageQueue = append(messageQueue, hand)
		}
	}

	if pokerSequenceStage == 1 {
		pokerSequencePlayerResult = result
		pokerSequencePlayerHand = hand
		a.setCurrentGameHistoryResults(hand, "", "", "In Progress", false)
		a.noteCurrentGameHistory("Player poker hand recorded")
		pokerSequenceStage = 2
		go func() {
			time.Sleep(700 * time.Millisecond)
			message := "Dealer Roll"
			a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] shouting: %q", message))
			log.Printf("[GAME_SELECT] shouting: %q", message)
			ext.Send(out.SHOUT, message)

			time.Sleep(700 * time.Millisecond)
			a.startPokerRoll()
		}()
	} else if pokerSequenceStage == 2 {
		winner := comparePokerHands(pokerSequencePlayerResult, result)
		playerName := strings.TrimSpace(pokerSequencePlayerName)
		if playerName == "" {
			playerName = "Player"
		}
		winnerName := "Dealer"
		if winner == PokerWinnerPlayer {
			winnerName = playerName
		}
		winnerMsg := fmt.Sprintf("%s Wins - %s: %s | Dealer: %s", winnerName, playerName, pokerSequencePlayerHand, hand)

		a.AddLogMsg(fmt.Sprintf("[POKER_RULES] player=%d dealer=%d winner=%s", pokerSequencePlayerResult.Category, result.Category, winnerMsg))
		log.Printf("[POKER_RULES] player=%d dealer=%d winner=%s", pokerSequencePlayerResult.Category, result.Category, winnerMsg)

		a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] shouting: %q", winnerMsg))
		log.Printf("[GAME_SELECT] shouting: %q", winnerMsg)
		time.Sleep(800 * time.Millisecond)
		sendMessageWithDelay(winnerMsg)

		payoutTargetID := lastTradePartnerID
		payoutTargetName := playerName
		playerHand := pokerSequencePlayerHand
		resetPokerSequence()

		if winner == PokerWinnerPlayer && payoutTargetID > 0 {
			a.setCurrentGameHistoryResults(playerHand, hand, playerName, "Payout Pending", false)
			a.noteCurrentGameHistory(winnerMsg)
			a.AddLogMsg(fmt.Sprintf("[PAYOUT] player won, initiating payout trade to %s (%d)", payoutTargetName, payoutTargetID))
			log.Printf("[PAYOUT] player won, initiating payout trade to %s (%d)", payoutTargetName, payoutTargetID)
			startPokerPayout(a, payoutTargetID, payoutTargetName)
		} else {
			a.setCurrentGameHistoryResults(playerHand, hand, "Dealer", "Completed", true)
			a.noteCurrentGameHistory(winnerMsg)
			// Dealer wins — resume normal dealer-open cycle
			awaitingTradeOpen = true
			if canAnnounceDealerOpen() {
				dealerTradeWindowOpen = true
				go sendMessageWithDelay(a.dealerOpenMessage())
			}
			startDealerOpenHeartbeat(a)
		}
	}

	isPokerRolling = false
}

func (a *App) evaluateBlackjackHand() {
	mutex.Lock()
	mutex.Unlock()

	if !ChatIsDisabled {
		// Log the current sum for debugging purposes
		log.Printf("Evaluating hand: Current sum = %d\n", currentSum)

		// If sum is less than 15, call hitBjDice to roll another dice
		if currentSum < 15 {
			log.Println("Sum is less than 15. Hitting another dice.")
			a.hitBjDice() // This will hit the dice and then re-evaluate the hand
			return        // Return early after hitting, so we don't send a message yet
		}

		// Convert sum to string and send to chat
		hand := strconv.Itoa(currentSum)
		logRollResult := fmt.Sprintf("21 Result: %s\n", hand)
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		a.AddLogMsg(logRollResult)
		a.setCurrentGameHistoryResults(hand, "", "Not Recorded", "Result Recorded", true)
		a.noteCurrentGameHistory("21 result recorded; winner was not automatically tracked")

		if !isMuted {
			// If the user is not muted, send the message
			sendMessageWithDelay(hand)
		} else {
			// If the user is muted, queue the message to send later
			log.Printf("User is muted. Queuing message: %s", hand)
			// ToDo:
			// messageQueue = append(messageQueue, hand)
		}
	} else {
		// Log the current sum for debugging purposes
		log.Printf("Evaluating hand: Current sum = %d\n", currentSum)

		// If sum is less than 15, call hitBjDice to roll another dice
		if currentSum < 15 {
			log.Println("Sum is less than 15. Hitting another dice.")
			a.hitBjDice() // This will hit the dice and then re-evaluate the hand
			return        // Return early after hitting, so we don't send a message yet
		}

		// Convert sum to string and send to chat
		hand := strconv.Itoa(currentSum)
		logRollResult := fmt.Sprintf("21 Result: %s\n", hand)
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		a.AddLogMsg(logRollResult)
		a.setCurrentGameHistoryResults(hand, "", "Not Recorded", "Result Recorded", true)
		a.noteCurrentGameHistory("21 result recorded; winner was not automatically tracked")
	}
	isBJRolling = false
	isHitting = false
}

func (a *App) evaluate13Hand() {
	mutex.Lock()
	mutex.Unlock()

	if !ChatIsDisabled {
		// Log the current sum for debugging purposes
		log.Printf("Evaluating hand: Current sum = %d\n", currentSum)

		// If sum is less than 15, call hitBjDice to roll another dice
		if currentSum < 7 {
			log.Println("Sum is less than 7. Hitting another dice.")
			a.hit13Dice() // This will hit the dice and then re-evaluate the hand
			return        // Return early after hitting, so we don't send a message yet
		}

		// Convert sum to string and send to chat
		hand := strconv.Itoa(currentSum)
		logRollResult := fmt.Sprintf("13 Result: %s\n", hand)
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		a.AddLogMsg(logRollResult)
		a.setCurrentGameHistoryResults(hand, "", "Not Recorded", "Result Recorded", true)
		a.noteCurrentGameHistory("13 result recorded; winner was not automatically tracked")
		if !isMuted {
			// If the user is not muted, send the message
			sendMessageWithDelay(hand)
		} else {
			// If the user is muted, queue the message to send later
			log.Printf("User is muted. Queuing message: %s", hand)
			// ToDo:
			// messageQueue = append(messageQueue, hand)
		}
	} else {
		// Log the current sum for debugging purposes
		log.Printf("Evaluating hand: Current sum = %d\n", currentSum)

		// If sum is less than 15, call hitBjDice to roll another dice
		if currentSum < 7 {
			log.Println("Sum is less than 7. Hitting another dice.")
			a.hit13Dice() // This will hit the dice and then re-evaluate the hand
			return        // Return early after hitting, so we don't send a message yet
		}

		// Convert sum to string and send to chat
		hand := strconv.Itoa(currentSum)
		logRollResult := fmt.Sprintf("13 Result: %s\n", hand)
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		a.AddLogMsg(logRollResult)
		a.setCurrentGameHistoryResults(hand, "", "Not Recorded", "Result Recorded", true)
		a.noteCurrentGameHistory("13 result recorded; winner was not automatically tracked")
	}
	is13Rolling = false
	is13Hitting = false
}

// Wait for all dice results and evaluate the tri hand
func (a *App) evaluateTriHand() {
	if !ChatIsDisabled {
		hand := sumHand([]int{
			diceList[0].Value,
			diceList[2].Value,
			diceList[4].Value,
		})
		logRollResult := fmt.Sprintf("Tri Result: %s\n", hand)
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		a.AddLogMsg(logRollResult)

		if !isMuted {
			// If the user is not muted, send the message
			sendMessageWithDelay(hand)
		} else {
			// If the user is muted, queue the message to send later
			log.Printf("User is muted. Queuing message: %s", hand)
			// ToDo:
			// messageQueue = append(messageQueue, hand)
		}
	} else {
		hand := sumHand([]int{
			diceList[0].Value,
			diceList[2].Value,
			diceList[4].Value,
		})
		logRollResult := fmt.Sprintf("Tri Result: %s\n", hand)
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		a.AddLogMsg(logRollResult)
	}
	isTriRolling = false
}

// Sum the values of the dice and return a string representation
func sumHand(values []int) string {
	sum := 0
	for _, val := range values {
		sum += val
	}
	return strconv.Itoa(sum)
}

// Sum the values of the dice and return the integer sum
func sumHandInt(values []int) int {
	sum := 0
	for _, val := range values {
		sum += val
	}
	return sum
}

// Evaluate the hand of dice and return a string representation
// thank you b7 <3 (and me, eduard, selfplug lol)
func (a *App) toPokerString(dices []*Dice) string {
	config := a.LoadConfig()
	if config == nil {
		fmt.Println("Using default configuration")
		config = defaultPokerDisplayConfig()
	}

	return formatPokerHandResult(config, evaluatePokerRules(dices))
}
