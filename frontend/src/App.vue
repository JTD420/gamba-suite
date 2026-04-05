<template>
  <div class="poker-config-section">

    <!-- Tab bar -->
    <div class="tab-bar">
      <button
        v-for="tab in ['Config', 'Trade', 'Logs']"
        :key="tab"
        :class="['tab-btn', { active: activeTab === tab }]"
        @click="activeTab = tab"
      >{{ tab }}</button>
    </div>

    <!-- Config tab -->
    <div v-if="activeTab === 'Config'">
      <h2 class="section-title">Poker Hand Configurations</h2>
      <form @submit.prevent="saveConfig">
        <div class="form-group" v-for="(value, key) in config" :key="key">
          <label :for="key">{{ formatLabel(key) }}:</label>
          <input v-model="config[key]" type="text" :id="key" />
        </div>
        <button type="submit" class="save-button">Save</button>
      </form>

      <button @click="handleShowCommands" class="save-button">Show Commands</button>
      <button @click="handleOpenLastTrade" class="save-button">
        Open Last Trade: {{ lastTradePartnerName }}
      </button>
      <button @click="handleSkipDiceSetup" class="save-button">
        Skip Dice Setup (Testing)
      </button>

      <div v-if="isOutdated" class="update-notice">
        A new version of this application is available. Please update to the latest version.
      </div>
    </div>

    <!-- Trade tab -->
    <div v-if="activeTab === 'Trade'">

      <h2 class="section-title">Double Payout Check</h2>
      <div class="trade-empty" v-if="tradeItems.length === 0">
        No active bet in trade yet.
      </div>
      <table v-else class="catalog-table">
        <thead><tr><th>Item</th><th>Bet</th><th>Need Stock</th><th>Payout</th><th>Have</th><th>Status</th></tr></thead>
        <tbody>
          <tr v-for="(row, index) in payoutRows" :key="`payout-${index}`">
            <td><span class="catalog-label">{{ row.displayName }}</span></td>
            <td><span class="catalog-label">{{ row.betQty }}</span></td>
            <td><span class="catalog-label">{{ row.required }}</span></td>
            <td><span class="catalog-label">{{ row.payoutTotal }}</span></td>
            <td><span class="catalog-label">{{ row.have }}</span></td>
            <td><span class="catalog-label" :class="{ 'unlisted-value': row.short > 0 }">{{ row.short > 0 ? `Short ${row.short}` : 'OK' }}</span></td>
          </tr>
        </tbody>
      </table>
      <p class="trade-hint" v-if="tradeItems.length > 0 && !canCoverPayout">
        You do not have enough stock to return double payout (bet + match).
      </p>
      <p class="trade-hint" v-if="tradeItems.length > 0 && canCoverPayout">
        Your hand can return double payout (bet + match).
      </p>

      <hr class="trade-divider" />

      <!-- Your Hand -->
      <h2 class="section-title">Your Hand</h2>
      <div v-if="handItems.length === 0" class="trade-empty">
        No hand data yet.
        <span style="font-size:12px;color:#666;">Refreshes every 30s and when a trade opens.</span>
      </div>
      <table v-else class="catalog-table">
        <thead><tr><th>Item</th><th>Qty</th></tr></thead>
        <tbody>
          <tr v-for="(item, index) in handItems" :key="`hand-${index}`">
            <td><span class="catalog-label">{{ item.displayName }}</span></td>
            <td><span class="catalog-label">{{ item.Quantity }}</span></td>
          </tr>
        </tbody>
      </table>

      <hr class="trade-divider" />

      <!-- Partner's Offer -->
      <h2 class="section-title">Partner's Offer</h2>
      <div v-if="tradeItems.length === 0" class="trade-empty">
        No active trade items detected.<br />
        <span style="font-size:12px;color:#666">Items appear here when the partner places furniture in the trade.</span>
      </div>
      <table v-else class="catalog-table">
        <thead><tr><th>Item</th><th>Qty</th></tr></thead>
        <tbody>
          <tr v-for="(item, index) in tradeItems" :key="`trade-${index}`">
            <td><span class="catalog-label">{{ item.displayName }}</span></td>
            <td><span class="catalog-label">{{ item.Quantity }}</span></td>
          </tr>
        </tbody>
      </table>

      <hr class="trade-divider" />

      <!-- Your Offer -->
      <h2 class="section-title">Your Offer</h2>
      <div v-if="ownTradeItems.length === 0" class="trade-empty">
        You are not offering any items.
      </div>
      <table v-else class="catalog-table">
        <thead><tr><th>Item</th><th>Qty</th></tr></thead>
        <tbody>
          <tr v-for="(item, index) in ownTradeItems" :key="`own-trade-${index}`">
            <td><span class="catalog-label">{{ item.displayName }}</span></td>
            <td><span class="catalog-label">{{ item.Quantity }}</span></td>
          </tr>
        </tbody>
      </table>

    </div>

    <!-- Logs tab -->
    <div v-if="activeTab === 'Logs'">
      <div class="log-grid">
        <div>
          <div class="log-title-row">
            <h2 class="section-title">Roll Logs</h2>
            <button class="copy-btn" @click="copyRollLogs">Copy</button>
          </div>
          <div id="log" ref="logbox" class="log-section">
            <div v-for="(msg, index) in log" :key="`roll-${index}`">{{ msg }}</div>
          </div>
        </div>
        <div>
          <div class="log-title-row">
            <h2 class="section-title">Chat Logs</h2>
            <button class="copy-btn" @click="copyChatLogs">Copy</button>
          </div>
          <div ref="chatlogbox" class="log-section chat-log-section">
            <div v-for="(msg, index) in chatLog" :key="`chat-${index}`">{{ msg }}</div>
          </div>
        </div>
      </div>
    </div>

  </div>
</template>

<script>
export default {
  data() {
    return {
      activeTab: 'Config',
      config: {
        five_of_a_kind: '',
        four_of_a_kind: '',
        full_house: '',
        high_straight: '',
        low_straight: '',
        three_of_a_kind: '',
        two_pair: '',
        one_pair: '',
        nothing: '',
      },
      tradeItems: [],
      ownTradeItems: [],
      handItems: [],
      log: [],
      chatLog: [],
      isOutdated: false,
      currentVersion: '',
      lastTradePartnerName: 'None',
      tradeNamePollTimer: null,
    };
  },
  computed: {
    tradeItemsWithDisplay() {
      return this.tradeItems.map(item => {
        return { ...item, displayName: this.formatItemName(item.Name) };
      });
    },
    ownTradeItemsWithDisplay() {
      return this.ownTradeItems.map(item => {
        return { ...item, displayName: this.formatItemName(item.Name) };
      });
    },
    handItemsWithDisplay() {
      return this.handItems.map(item => {
        return { ...item, displayName: this.formatItemName(item.Name) };
      });
    },
    payoutRows() {
      const handByName = this.handItems.reduce((acc, item) => {
        acc[item.Name] = (acc[item.Name] || 0) + item.Quantity;
        return acc;
      }, {});

      return this.tradeItemsWithDisplay.map(item => {
        const required = item.Quantity;
        const payoutTotal = item.Quantity * 2;
        const have = handByName[item.Name] || 0;
        const short = Math.max(required - have, 0);
        return {
          name: item.Name,
          displayName: item.displayName,
          betQty: item.Quantity,
          required,
          payoutTotal,
          have,
          short,
        };
      });
    },
    canCoverPayout() {
      return this.payoutRows.every((row) => row.short === 0);
    },
  },
  methods: {
     async handleShowCommands() {
      try {
        await window.go.main.App.ShowCommands();
      } catch (error) {
        this.addLogMsg('Error showing commands');
        console.error(error);
      }
    },
    async loadConfig() {
      try {
        const response = await window.go.main.App.LoadConfig();
        if (response) {
          this.config = response;
        }
        this.addLogMsg('Configuration loaded');
      } catch (error) {
        this.addLogMsg('Error loading configuration');
        console.error(error);
      }
    },
    async saveConfig() {
      try {
        await window.go.main.App.SaveConfig(this.config);
        this.addLogMsg('Configuration saved');
      } catch (error) {
        this.addLogMsg('Error saving configuration');
        console.error(error);
      }
    },
    async refreshLastTradePartnerName() {
      try {
        const name = await window.go.main.App.GetLastTradePartnerName();
        this.lastTradePartnerName = name || 'None';
      } catch (error) {
        console.error(error);
      }
    },
    async handleOpenLastTrade() {
      try {
        await window.go.main.App.OpenLastTrade();
        this.addLogMsg(`Open Last Trade clicked (${this.lastTradePartnerName})`);
        await this.refreshLastTradePartnerName();
      } catch (error) {
        this.addLogMsg('Error opening last trade');
        console.error(error);
      }
    },
    async handleSkipDiceSetup() {
      try {
        await window.go.main.App.SkipDiceSetupForTesting();
      } catch (error) {
        this.addLogMsg('Error skipping dice setup');
        console.error(error);
      }
    },
    addLogMsg(msg) {
      this.log.push(msg);
      this.scrollBox('logbox');
    },
    addChatLogMsg(msg) {
      this.chatLog.push(msg);
      this.scrollBox('chatlogbox');
    },
    async copyTextToClipboard(text, label) {
      try {
        if (navigator && navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(text);
        } else {
          const ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.left = '-9999px';
          document.body.appendChild(ta);
          ta.focus();
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
        }
        this.addLogMsg(`[UI] Copied ${label} to clipboard`);
      } catch (error) {
        this.addLogMsg(`[UI] Failed to copy ${label}`);
        console.error(error);
      }
    },
    async copyRollLogs() {
      const text = this.log.join('\n');
      await this.copyTextToClipboard(text, 'roll logs');
    },
    async copyChatLogs() {
      const text = this.chatLog.join('\n');
      await this.copyTextToClipboard(text, 'chat logs');
    },
    formatLabel(key) {
      return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
    },
    formatItemName(name) {
      return String(name || '')
        .split('_')
        .filter(Boolean)
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(' ');
    },
    scrollBox(refName) {
      this.$nextTick(() => {
        const box = this.$refs[refName];
        if (box) {
          box.scrollTop = box.scrollHeight;
        }
      });
    },
    async checkForUpdates() {
      try {
        const response = await fetch(
          "https://raw.githubusercontent.com/JTD420/G-ExtensionStore/repo/1.5.3/store/extensions/%5BAIO%5D%20Gamba%20Suite/extension.json"
        );
        const data = await response.json();
        const latestVersion = data.version;

        if (this.currentVersion !== latestVersion) {
          this.isOutdated = true;
        }
      } catch (error) {
        this.addLogMsg('Error checking for updates');
        console.error(error);
      }
    },
    async fetchCurrentVersion() {
      try {
        const version = await window.go.main.App.GetCurrentVersion(); // Fetch version from Go
        this.currentVersion = version;
      } catch (error) {
        this.addLogMsg('Error fetching current version');
        console.error(error);
      }
    },
    fetch() {
      this.loadConfig();
    },
  },
  async mounted() {
    await this.fetchCurrentVersion();
    this.fetch();
    await this.refreshLastTradePartnerName();
    await this.checkForUpdates();
    window.runtime.EventsOn("logUpdate", (message) => {
      this.log = message.split('\n');
      this.scrollBox('logbox');
    });
    window.runtime.EventsOn("chatLogUpdate", (message) => {
      this.chatLog = message.split('\n');
      this.scrollBox('chatlogbox');
    });

    window.runtime.EventsOn("tradeItemsUpdate", (jsonStr) => {
      try {
        this.tradeItems = (JSON.parse(jsonStr) || []).map(item => ({
          ...item,
          displayName: this.formatItemName(item.Name),
        }));
      } catch (_) {
        this.tradeItems = [];
      }
    });

    window.runtime.EventsOn("ownTradeItemsUpdate", (jsonStr) => {
      try {
        this.ownTradeItems = (JSON.parse(jsonStr) || []).map(item => ({
          ...item,
          displayName: this.formatItemName(item.Name),
        }));
      } catch (_) {
        this.ownTradeItems = [];
      }
    });

    window.runtime.EventsOn("handItemsUpdate", (jsonStr) => {
      try {
        this.handItems = (JSON.parse(jsonStr) || []).map(item => ({
          ...item,
          displayName: this.formatItemName(item.Name),
        }));
      } catch (_) {
        this.handItems = [];
      }
    });

    this.tradeNamePollTimer = setInterval(() => {
      this.refreshLastTradePartnerName();
    }, 2000);
  },
  beforeUnmount() {
    if (this.tradeNamePollTimer) {
      clearInterval(this.tradeNamePollTimer);
      this.tradeNamePollTimer = null;
    }
  }
};
</script>


<style scoped>
body {
  background-color: #100e0e!important;
}
.poker-config-section {
  padding: 20px;
  background-color: #100e0e;
  border-radius: 8px;
  color: #fff;
}

.section-title {
  font-size: 18px;
  margin-bottom: 10px;
  color: #e0e0e0;
  text-align: center;
}

.form-group {
  display: flex;
  justify-content: center;
  align-items: center;
  margin-bottom: 10px;
}

label {
  flex: 1;
  font-weight: bold;
  text-align: right;
  margin-right: 10px;
  font-size: 14px;
  color: #c0c0c0;
}

input[type="text"] {
  flex: 2;
  padding: 8px;
  background-color: #2e2e2e;
  border: 1px solid #444;
  border-radius: 4px;
  color: #fff;
  font-size: 14px;
  max-width: 300px;
}

input[type="text"]::placeholder {
  color: #888;
}

.save-button {
  display: block;
  width: 100%;
  padding: 8px;
  background-color: #2f2f2f;
  color: white;
  border: solid .2px #444;
  border-radius: 4px;
  cursor: pointer;
  margin-top: 15px;
  font-size: 14px;
}

.save-button:hover {
  background-color: #1e1e1e;
}

.log-section {
  background-color: #000000; /* Black background for the console */
  padding: 10px;
  border-radius: 4px;
  height: 200px;
  overflow-y: auto;
  font-family: monospace;
  margin-top: 10px;
  color: #00ff00; /* Green text color for the hacker console style */
  font-size: 13px;
  line-height: 1.4em;
  border: 2px solid #00ff00; /* Optional: Green border for console look */
}

/* Add some padding to each log message for readability */
.log-section div {
  padding: 2px 0;
}

.log-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 16px;
}

.log-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.copy-btn {
  padding: 6px 10px;
  background-color: #1f1f1f;
  color: #d8d8d8;
  border: 1px solid #4a4a4a;
  border-radius: 4px;
  cursor: pointer;
  font-size: 12px;
}

.copy-btn:hover {
  background-color: #2a2a2a;
}

.chat-log-section {
  color: #7dd3fc;
  border-color: #7dd3fc;
}

/* Update notice style */
.update-notice {
  margin-top: 15px;
  padding: 10px;
  background-color: #ffcc00;
  color: #000;
  text-align: center;
  border-radius: 4px;
  font-weight: bold;
}

/* Tab navigation */
.tab-bar {
  display: flex;
  gap: 4px;
  margin-bottom: 16px;
}
.tab-btn {
  flex: 1;
  padding: 8px;
  background-color: #2f2f2f;
  color: #c0c0c0;
  border: 1px solid #444;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
}
.tab-btn:hover {
  background-color: #1e1e1e;
}
.tab-btn.active {
  background-color: #444;
  color: #fff;
  border-color: #888;
}

/* Catalog table */
.catalog-table {
  width: 100%;
  border-collapse: collapse;
  margin-bottom: 10px;
  font-size: 13px;
}
.catalog-table th {
  text-align: left;
  padding: 6px 8px;
  color: #c0c0c0;
  border-bottom: 1px solid #444;
}
.catalog-table td {
  padding: 4px 4px;
}
.catalog-input {
  width: 100%;
  padding: 5px 6px;
  background-color: #2e2e2e;
  border: 1px solid #444;
  border-radius: 4px;
  color: #fff;
  font-size: 13px;
  box-sizing: border-box;
}
.catalog-value {
  width: 80px;
}
.catalog-label {
  display: block;
  padding: 5px 6px;
  color: #aaa;
  font-size: 13px;
}

.trade-divider {
  border: none;
  border-top: 1px solid #333;
  margin: 18px 0;
}

.trade-empty {
  text-align: center;
  color: #888;
  padding: 24px 12px;
  font-size: 14px;
  line-height: 1.8;
}
.trade-total-label {
  text-align: right;
  padding: 8px 8px;
  color: #e0e0e0;
  font-weight: bold;
  font-size: 14px;
  border-top: 1px solid #444;
}
.trade-total-value {
  padding: 8px 8px;
  color: #ffd700;
  font-weight: bold;
  font-size: 14px;
  border-top: 1px solid #444;
}
.unlisted-value {
  color: #ff8888;
}
.trade-hint {
  font-size: 12px;
  color: #888;
  margin-top: 10px;
  text-align: center;
}

.game-settings-box {
  border: 1px solid #333;
  border-radius: 6px;
  padding: 12px;
  background: #151515;
}

.game-settings-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.game-settings-row:last-of-type {
  margin-bottom: 0;
}

.game-settings-label {
  flex: 1;
  text-align: left;
  margin-right: 0;
}

.game-settings-input {
  flex: 1;
  max-width: 180px;
  padding: 8px;
  background-color: #2e2e2e;
  border: 1px solid #444;
  border-radius: 4px;
  color: #fff;
  font-size: 14px;
}

.game-settings-value {
  flex: 1;
  max-width: 180px;
  padding: 8px;
  border: 1px solid #333;
  border-radius: 4px;
  background-color: #1f1f1f;
  color: #ffd700;
  font-weight: 700;
}

.game-settings-save {
  margin-top: 12px;
}
</style>