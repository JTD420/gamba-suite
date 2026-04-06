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
		if !ChatIsDisabled {
			waitForUnmute(90 * time.Second)
			time.Sleep(800 * time.Millisecond)
			sendMessageWithDelay(winnerMsg)
		}

		payoutTargetID := lastTradePartnerID
		payoutTargetName := playerName
		playerHand := pokerSequencePlayerHand
		resetPokerSequence()

		if winner == PokerWinnerPlayer && payoutTargetID > 0 {
			a.setCurrentGameHistoryResults(playerHand, hand, playerName, "Payout Pending", false)
			a.noteCurrentGameHistory(winnerMsg)
			a.AddLogMsg(fmt.Sprintf("[PAYOUT] player won, initiating payout trade to %s (%d)", payoutTargetName, payoutTargetID))
			log.Printf("[PAYOUT] player won, initiating payout trade to %s (%d)", payoutTargetName, payoutTargetID)
			startPayout(a, payoutTargetID, payoutTargetName)
		} else {
			a.setCurrentGameHistoryResults(playerHand, hand, "Dealer", "Completed", true)
			a.noteCurrentGameHistory(winnerMsg)
			go a.openDealerAfterRound()
		}
	}

	isPokerRolling = false
}

func (a *App) evaluateBlackjackHand() {
	defer func() {
		if r := recover(); r != nil {
			a.AddLogMsg(fmt.Sprintf("[BJ_CRASH_GUARD] recovered panic in evaluateBlackjackHand: %v", r))
			log.Printf("[BJ_CRASH_GUARD] recovered panic in evaluateBlackjackHand: %v", r)
			resetBlackjackSequence()
			isBJRolling = false
			isHitting = false
		}
	}()

	mutex.Lock()
	mutex.Unlock()
	if !blackjackRoundActive {
		isBJRolling = false
		isHitting = false
		return
	}

	log.Printf("[BJ] evaluating sum=%d playerTurn=%t", currentSum, blackjackPlayerTurn)
	a.AddLogMsg(fmt.Sprintf("[BJ_DEBUG] evaluate start roundActive=%t playerTurn=%t sum=%d hitInFlight=%t awaitingDecision=%t", blackjackRoundActive, blackjackPlayerTurn, currentSum, blackjackHitInFlight, awaitingBlackjackDecision))
	log.Printf("[BJ_DEBUG] evaluate start roundActive=%t playerTurn=%t sum=%d hitInFlight=%t awaitingDecision=%t", blackjackRoundActive, blackjackPlayerTurn, currentSum, blackjackHitInFlight, awaitingBlackjackDecision)

	if blackjackPlayerTurn {
		playerLabel := strings.TrimSpace(blackjackPlayerName)
		if playerLabel == "" {
			playerLabel = strings.TrimSpace(lastTradePartnerName)
		}
		if playerLabel == "" {
			playerLabel = "Player"
		}

		blackjackPlayerTotal = currentSum
		a.AddLogMsg(fmt.Sprintf("[BJ] player total now %d", blackjackPlayerTotal))
		log.Printf("[BJ] player total now %d", blackjackPlayerTotal)

		if blackjackPlayerTotal > 21 {
			a.finalizeBlackjackRound(false, "player bust")
			isBJRolling = false
			isHitting = false
			return
		}

		if blackjackPlayerTotal == 21 {
			a.AddLogMsg("[BJ] player total 21; auto-stay and moving to dealer roll")
			log.Printf("[BJ] player total 21; auto-stay and moving to dealer roll")
			if !ChatIsDisabled && !isMuted {
				sendMessageWithDelay(fmt.Sprintf("%s total %d", playerLabel, blackjackPlayerTotal))
			}
			a.startBlackjackDealerTurn("player reached 21 auto-stay")
			isBJRolling = false
			isHitting = false
			return
		}

		if blackjackPlayerTotal < 17 {
			a.AddLogMsg(fmt.Sprintf("[BJ] player total %d < 17; auto-hit", blackjackPlayerTotal))
			log.Printf("[BJ] player total %d < 17; auto-hit", blackjackPlayerTotal)
			if blackjackHitInFlight {
				a.AddLogMsg("[BJ] auto-hit deferred: hit already in flight")
				log.Printf("[BJ] auto-hit deferred: hit already in flight")
				go func() {
					time.Sleep(150 * time.Millisecond)
					a.evaluateBlackjackHand()
				}()
				return
			}
			blackjackHitInFlight = true
			isHitting = true
			go a.hitBjDice()
			return
		}

		awaitingBlackjackDecision = true
		awaitingBlackjackDecisionPartnerName = strings.TrimSpace(blackjackPlayerName)
		if awaitingBlackjackDecisionPartnerName == "" {
			awaitingBlackjackDecisionPartnerName = strings.TrimSpace(lastTradePartnerName)
		}
		if awaitingBlackjackDecisionPartnerName == "" {
			awaitingBlackjackDecisionPartnerName = "Player"
		}
		if chatIdx, ok := lookupRoomEntityIndexByName(awaitingBlackjackDecisionPartnerName); ok && chatIdx > 0 {
			awaitingBlackjackDecisionPartnerID = chatIdx
			a.AddLogMsg(fmt.Sprintf("[BJ_DEBUG] prompt target resolved via room index=%d name=%q", awaitingBlackjackDecisionPartnerID, awaitingBlackjackDecisionPartnerName))
			log.Printf("[BJ_DEBUG] prompt target resolved via room index=%d name=%q", awaitingBlackjackDecisionPartnerID, awaitingBlackjackDecisionPartnerName)
		} else if chatIdx, ok := lookupUsers28RoomIndexByName(awaitingBlackjackDecisionPartnerName); ok && chatIdx > 0 {
			awaitingBlackjackDecisionPartnerID = chatIdx
			a.AddLogMsg(fmt.Sprintf("[BJ_DEBUG] prompt target resolved via users28 roomIndex=%d name=%q", awaitingBlackjackDecisionPartnerID, awaitingBlackjackDecisionPartnerName))
			log.Printf("[BJ_DEBUG] prompt target resolved via users28 roomIndex=%d name=%q", awaitingBlackjackDecisionPartnerID, awaitingBlackjackDecisionPartnerName)
		} else {
			awaitingBlackjackDecisionPartnerID = lastTradePartnerID
			a.AddLogMsg(fmt.Sprintf("[BJ_DEBUG] prompt target fallback index=%d name=%q", awaitingBlackjackDecisionPartnerID, awaitingBlackjackDecisionPartnerName))
			log.Printf("[BJ_DEBUG] prompt target fallback index=%d name=%q", awaitingBlackjackDecisionPartnerID, awaitingBlackjackDecisionPartnerName)
		}

		prompt := fmt.Sprintf("%s total %d. Hit or stay?", playerLabel, blackjackPlayerTotal)
		a.AddLogMsg(fmt.Sprintf("[BJ] prompting decision: %q", prompt))
		log.Printf("[BJ] prompting decision: %q", prompt)
		if !ChatIsDisabled && !isMuted {
			a.AddLogMsg("[BJ_DEBUG] sending hit/stay prompt to chat")
			log.Printf("[BJ_DEBUG] sending hit/stay prompt to chat")
			sendMessageWithDelay(prompt)
		} else {
			a.AddLogMsg("[BJ] prompt not sent (chat disabled or muted); auto-staying")
			log.Printf("[BJ] prompt not sent (chat disabled or muted); auto-staying")
			awaitingBlackjackDecision = false
			a.startBlackjackDealerTurn("prompt unavailable auto-stay")
			isBJRolling = false
			isHitting = false
			return
		}

		isBJRolling = false
		isHitting = false
		return
	}

	blackjackDealerTotal = currentSum
	a.AddLogMsg(fmt.Sprintf("[BJ] dealer total now %d (player=%d)", blackjackDealerTotal, blackjackPlayerTotal))
	log.Printf("[BJ] dealer total now %d (player=%d)", blackjackDealerTotal, blackjackPlayerTotal)

	if blackjackDealerTotal > 21 {
		a.finalizeBlackjackRound(true, "dealer bust")
		isBJRolling = false
		isHitting = false
		return
	}

	if blackjackDealerTotal < blackjackPlayerTotal {
		a.AddLogMsg(fmt.Sprintf("[BJ] dealer total %d < player %d; dealer hits", blackjackDealerTotal, blackjackPlayerTotal))
		log.Printf("[BJ] dealer total %d < player %d; dealer hits", blackjackDealerTotal, blackjackPlayerTotal)
		if blackjackHitInFlight {
			a.AddLogMsg("[BJ] dealer hit deferred: hit already in flight")
			log.Printf("[BJ] dealer hit deferred: hit already in flight")
			go func() {
				time.Sleep(150 * time.Millisecond)
				a.evaluateBlackjackHand()
			}()
			return
		}
		blackjackHitInFlight = true
		isHitting = true
		go a.hitBjDice()
		return
	}

	a.finalizeBlackjackRound(false, "dealer beat-or-tie")
	isBJRolling = false
	isHitting = false
}

func (a *App) startBlackjackDealerTurn(reason string) {
	awaitingBlackjackDecision = false
	blackjackPlayerTurn = false
	a.AddLogMsg(fmt.Sprintf("[BJ_DEBUG] dealer turn starting reason=%s playerTotal=%d dealerTotal=%d", reason, blackjackPlayerTotal, blackjackDealerTotal))
	log.Printf("[BJ_DEBUG] dealer turn starting reason=%s playerTotal=%d dealerTotal=%d", reason, blackjackPlayerTotal, blackjackDealerTotal)
	message := "Dealer Roll"
	a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] shouting: %q", message))
	log.Printf("[GAME_SELECT] shouting: %q", message)
	ext.Send(out.SHOUT, message)
	go func() {
		time.Sleep(700 * time.Millisecond)
		isBJRolling = true
		a.rollBjDice()
	}()
}

func (a *App) finalizeBlackjackRound(playerWins bool, reason string) {
	playerName := strings.TrimSpace(blackjackPlayerName)
	if playerName == "" {
		playerName = strings.TrimSpace(lastTradePartnerName)
	}
	if playerName == "" {
		playerName = "Player"
	}

	playerHand := strconv.Itoa(blackjackPlayerTotal)
	dealerHand := strconv.Itoa(blackjackDealerTotal)
	winnerName := "Dealer"
	if playerWins {
		winnerName = playerName
	}
	winnerMsg := fmt.Sprintf("%s Wins - %s: %s | Dealer: %s", winnerName, playerName, playerHand, dealerHand)

	a.AddLogMsg(fmt.Sprintf("[BJ_RULES] winner=%s reason=%s player=%d dealer=%d", winnerName, reason, blackjackPlayerTotal, blackjackDealerTotal))
	log.Printf("[BJ_RULES] winner=%s reason=%s player=%d dealer=%d", winnerName, reason, blackjackPlayerTotal, blackjackDealerTotal)
	a.AddLogMsg(fmt.Sprintf("[GAME_SELECT] shouting: %q", winnerMsg))
	log.Printf("[GAME_SELECT] shouting: %q", winnerMsg)
	if !ChatIsDisabled {
		waitForUnmute(90 * time.Second)
		time.Sleep(800 * time.Millisecond)
		sendMessageWithDelay(winnerMsg)
	}

	payoutTargetID := lastTradePartnerID
	payoutTargetName := playerName
	resetBlackjackSequence()

	if playerWins && payoutTargetID > 0 {
		a.setCurrentGameHistoryResults(playerHand, dealerHand, playerName, "Payout Pending", false)
		a.noteCurrentGameHistory(winnerMsg)
		a.AddLogMsg(fmt.Sprintf("[PAYOUT] 21 player won, initiating payout trade to %s (%d)", payoutTargetName, payoutTargetID))
		log.Printf("[PAYOUT] 21 player won, initiating payout trade to %s (%d)", payoutTargetName, payoutTargetID)
		startPayout(a, payoutTargetID, payoutTargetName)
		return
	}

	a.setCurrentGameHistoryResults(playerHand, dealerHand, "Dealer", "Completed", true)
	a.noteCurrentGameHistory(winnerMsg)
	go a.openDealerAfterRound()
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
