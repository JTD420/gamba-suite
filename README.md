# [AIO] Gamba Suite

Gamba Suite is a powerful tool for managing, rolling, and resetting dice with automated poker, 13/21 & Tri hand evaluation and game interaction.

## Features

- Manage multiple dice: Handle operations involving multiple dice, including adding, removing, and tracking their states.
- Roll and reset dice: Simulate rolling dice and resetting them to their initial state.
- Automatically evaluate poker hands: Determine the value of poker hands based on the rolled dice.
- Automatically evaluate tri sum: Calculate the sum of three dice and evaluate specific conditions or outcomes.
- Automatically evaluate 13 sum: Rolls two dice and calculates their sum. If the sum is less than 7, it will roll additional dice, recalculating the sum with each new roll until the sum is 7 or higher.
- Automatically evaluate 21 sum: Rolls three dice and calculates their sum. If the sum is less than 15, additional dice will be rolled, with recalculations after each roll until the sum is 15 or higher.

## Installation

1. **Clone the repository:**

   Open your terminal and clone the repository using the following command:

   ```bash
   git clone https://github.com/JTD420/gamba-suite.git
   ```

2. **Navigate to the project directory:**

   Change your working directory to the project's directory:

   ```bash
   cd Gamba-Suite
   ```

3. **Build the project:**

   Use the `go build` command to build the project:

   ```bash
   go build
   ```

4. **Run the project:**

   After building, execute the project with:

   ```bash
   ./Gamba-Suite
   ```

## Usage

### Setup

1. **Run the Project:**

   After executing `./Gamba-Suite`, the application will start running.

2. **Initialize Dice:**

   To set up, simply doubleclick all the dices. The program will record the dice in the order they were rolled. After setup is complete, you can begin using the available commands.


### Chat Commands

- `:roll` - Rolls all dice used in poker and announces the results of their values.
- `:tri` - Rolls all dice used in the tri game and announces the total sum of the three dice.
- `:13` - Rolls all dice used in the 13 sum game and announces the total sum once it is 7 or higher.
- `:21` - Rolls all dice used in the 21 sum game and announces the total sum once it is 15 or higher.
- `:verify` - Re-announces the most recent total sum for 13/21 in chat. Useful if the user was muted during the original announcement.
- `:close` - Closes all dices.
- `:reset` - Clears any previously stored dice data for a fresh start.
- `:chaton` - Enables announcing the results of the dice rolls.
- `:chatoff` - Disables announcing the results of the dice rolls.
- `:commands` - View a hotel alert with all available commands.

## Contributing

Contributions are welcome! Please submit a pull request or open an issue to discuss any changes.

## License

This project is licensed under the MIT License.

```

Feel free to customize the content according to your needs.
```

## Special Thanks

Special thanks to Nanobyte for his original [poker](https://github.com/boydmeyer/poker) Extension <3
