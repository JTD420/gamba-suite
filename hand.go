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
func evaluatePokerHand() {
	if !ChatIsDisabled {
		hand := toPokerString(diceList)

		// sleep random between 250 and 500ms
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		ext.Send(out.SHOUT, hand)
	}
	isPokerRolling = false
}

func evaluateBlackjackHand() {
	mutex.Lock()
	mutex.Unlock()

	if !ChatIsDisabled {
		// Log the current sum for debugging purposes
		log.Printf("Evaluating hand: Current sum = %d\n", currentSum)

		// If sum is less than 15, call hitBjDice to roll another dice
		if currentSum < 15 {
			log.Println("Sum is less than 15. Hitting another dice.")
			hitBjDice() // This will hit the dice and then re-evaluate the hand
			return      // Return early after hitting, so we don't send a message yet
		}

		// Convert sum to string and send to chat
		hand := strconv.Itoa(currentSum)

		// sleep random between 250 and 500ms
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		ext.Send(out.SHOUT, hand)
	}
	isBJRolling = false
	isHitting = false
}

func evaluate13Hand() {
	mutex.Lock()
	mutex.Unlock()

	if !ChatIsDisabled {
		// Log the current sum for debugging purposes
		log.Printf("Evaluating hand: Current sum = %d\n", currentSum)

		// If sum is less than 15, call hitBjDice to roll another dice
		if currentSum < 7 {
			log.Println("Sum is less than 7. Hitting another dice.")
			hit13Dice() // This will hit the dice and then re-evaluate the hand
			return      // Return early after hitting, so we don't send a message yet
		}

		// Convert sum to string and send to chat
		hand := strconv.Itoa(currentSum)

		// sleep random between 250 and 500ms
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		ext.Send(out.SHOUT, hand)
	}
	is13Rolling = false
	is13Hitting = false
}

// Wait for all dice results and evaluate the tri hand
func evaluateTriHand() {
	if !ChatIsDisabled {
		hand := sumHand([]int{
			diceList[0].Value,
			diceList[2].Value,
			diceList[4].Value,
		})

		// sleep random between 250 and 500ms
		time.Sleep(time.Duration(rand.Intn(250)+250) * time.Millisecond)
		ext.Send(out.SHOUT, hand)
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
func toPokerString(dices []*Dice) string {
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
		return "Low Str8"
	}
	if s == "23456" {
		return "High Str8"
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
		return "nothing"
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i] > keys[j] })
	sort.Slice(values, func(i, j int) bool { return values[i] > values[j] })

	n := strings.Trim(strings.Replace(fmt.Sprint(keys), " ", "", -1), "[]")
	c := strings.Trim(strings.Replace(fmt.Sprint(values), " ", "", -1), "[]")

	switch c {
	case "5":
		return "Five of a kind: " + n
	case "4":
		return "Four of a kind: " + n
	case "3":
		return "Three of a kind: " + n
	case "32":
		return "Full House: " + n
	case "22":
		return "Two Pair: " + n + "s"
	case "2":
		return "One Pair: " + n + "s"
	default:
		return n + ""
	}
}
