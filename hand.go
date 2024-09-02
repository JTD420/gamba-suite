package main

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"xabbo.b7c.io/goearth/shockwave/out"
)

// Wait for all dice results and evaluate the poker hand
func (a *App) evaluatePokerHand() {
	if !ChatIsDisabled {
		hand := a.toPokerString(diceList)

		// sleep random between 250 and 500ms
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		ext.Send(out.SHOUT, hand)
		logRollResult := fmt.Sprintf("Poker Result: %s\n", hand)
		a.AddLogMsg(logRollResult)
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

		// sleep random between 250 and 500ms
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		ext.Send(out.SHOUT, hand)
		logRollResult := fmt.Sprintf("21 Result: %s\n", hand)
		a.AddLogMsg(logRollResult)
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

		// sleep random between 250 and 500ms
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		ext.Send(out.SHOUT, hand)
		logRollResult := fmt.Sprintf("13 Result: %s\n", hand)
		a.AddLogMsg(logRollResult)
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

		// sleep random between 250 and 500ms
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		ext.Send(out.SHOUT, hand)
		logRollResult := fmt.Sprintf("Tri Result: %s\n", hand)
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
	// Load user configuration
	config := a.LoadConfig()

	if config != nil {
		// Use the loaded config
		// fmt.Println("Config loaded:", config)
	} else {
		// Use default values if no config is found
		fmt.Println("Using default configuration")
		config = &PokerDisplayConfig{
			FiveOfAKind:  "F%s",
			FourOfAKind:  "Q%s",
			FullHouse:    "FH %s",
			HighStraight: "High Stright",
			LowStraight:  "Lo Stright",
			ThreeOfAKind: "T%s",
			TwoPair:      "%s",
			OnePair:      "%s",
			Nothing:      "Nothing",
		}
	}

	s := ""
	for _, dice := range dices {
		s += strconv.Itoa(dice.Value)
	}
	runes := []rune(s)
	sort.Slice(runes, func(i, j int) bool {
		return runes[i] < runes[j]
	})
	s = string(runes)

	if s == "12345" {
		return fmt.Sprintf(config.LowStraight)
	}
	if s == "23456" {
		return fmt.Sprintf(config.HighStraight)
	}

	mapCount := make(map[int]int)
	for _, c := range s {
		mapCount[int(c-'0')]++
	}

	keys := []int{}
	values := []int{}
	for k, v := range mapCount {
		if v > 1 {
			keys = append(keys, k)
			values = append(values, v)
		}
	}

	if len(keys) == 0 {
		return fmt.Sprintf(config.Nothing)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i] > keys[j] })
	sort.Slice(values, func(i, j int) bool { return values[i] > values[j] })

	n := strings.Trim(strings.Replace(fmt.Sprint(keys), " ", "", -1), "[]")
	c := strings.Trim(strings.Replace(fmt.Sprint(values), " ", "", -1), "[]")

	switch c {
	case "5":
		return fmt.Sprintf(config.FiveOfAKind, n)
	case "4":
		return fmt.Sprintf(config.FourOfAKind, n)
	case "3":
		return fmt.Sprintf(config.ThreeOfAKind, n)
	case "32":
		var threeOfAKind, pair int

		// Loop through the map to find the three-of-a-kind and the pair
		for num, count := range mapCount {
			if count == 3 {
				threeOfAKind = num
			} else if count == 2 {
				pair = num
			}
		}

		// Construct the string with the three-of-a-kind first
		n = strconv.Itoa(threeOfAKind) + strconv.Itoa(pair)
		return fmt.Sprintf(config.FullHouse, n)
	case "22":
		return fmt.Sprintf(config.TwoPair, n)
	case "2":
		return fmt.Sprintf(config.OnePair, n)
	default:
		return n + ""
	}
}
