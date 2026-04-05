# Arcade Tracker - BETA

Arcade Tracker is a extension to track your trades so you know your profits and losses in Habbo Hotel: Origins.

## Current Arcade Track integrations:
- [LegacyHabbo](https://legacyhabbo.me/) - This is the main API which tracks your trades, and authenticates you.
- [TraderClub](https://traderclub.gg/) - The Legacy API uses this API to get the prices of the furnitures
- [BobbaPRO](https://bobba.pro/) - The Legacy API uses this API to get the prices of the furnitures
- [HabboAPI](https://origins.habbo.com/api/public/users?name={USERNAME}) - The Legacy API uses this API to verify your motto and authenticate you

## Current Supported Webapps:
- [TSA](https://tsarcade.com/) - This is the first webapp that supports the tracker, where you can see your tracks in a friendly way. (This webapp is fully integrated with LegacyHabbo)

## MUST READ - What info this extension collects?
Hello, so for doing a automated tracker we needed to make a login feature, so when you use this extension you
agree that you will must share this information to `https://legacyhabbo.me/`:
- Your Habbo Username
- Server you play at (COM, ES, BR)*
- If you have the extension enabled, will track your trades itemdata, quantity, who you traded with and in which room

*currently the tracker only works for COM.

Why do we need this information?
- We need this so we can associate the account to the player and separate each player's tracks. We can't make this API public so we made a authentication system that will change your motto. We will never ask for any credential besides asking you to change your motto.

What will you do with this data?
- Nothing, we won't do anything with your tracks, you are able to delete all your tracks whenever you want.

I have a Arcade, can I have my own Tracker?
- Yes sure, you can contact me and I will make a custom tracker for you, but note that the databases are shared across all arcades so this still private.

## Features

- **User Authentication**: Authenticate users with the API.
- **Trade Tracking**: Track and log details of trades, checking directly your profits and losses with known APIs such as TraderClub, BobbaPRO.


## **In-Game Commands**:
    - :tracker // > Enable or disable the tracker.
    - :authenticate // > Authenticate the user in the Legacy API.
