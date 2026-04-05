<template>
  <div class="poker-config-section">

    <!-- Tab bar -->
    <div class="tab-bar">
      <button
        v-for="tab in ['Config', 'Trade', 'Game History', 'Logs']"
        :key="tab"
        :class="['tab-btn', { active: activeTab === tab }]"
        @click="activeTab = tab"
      >{{ tab }}</button>
    </div>

    <!-- Config tab -->
    <div v-if="activeTab === 'Config'">
      <h2 class="section-title">Games</h2>
      <p class="config-intro">
        This page is now read-only. Game behaviour is controlled in code, so players only see what each game does and how the flow works.
      </p>

      <div class="game-card-grid">
        <button
          v-for="game in gameGuides"
          :key="game.key"
          type="button"
          class="game-card"
          @click="openGameGuide(game)"
        >
          <span class="game-card-title">{{ game.title }}</span>
          <span class="game-card-summary">{{ game.summary }}</span>
          <span class="game-card-action">Click for details</span>
        </button>
      </div>

      <div class="game-guide-modal-backdrop" v-if="activeGameGuide" @click="closeGameGuide">
        <div class="game-guide-modal" @click.stop>
          <div class="game-guide-header">
            <h3 class="section-title game-guide-title">{{ activeGameGuide.title }}</h3>
            <button type="button" class="copy-btn" @click="closeGameGuide">Close</button>
          </div>
          <p class="game-guide-text">{{ activeGameGuide.description }}</p>
          <div class="game-guide-block">
            <div class="game-guide-label">How it works</div>
            <div class="game-guide-text">{{ activeGameGuide.howItWorks }}</div>
          </div>
          <div class="game-guide-block">
            <div class="game-guide-label">What the player does</div>
            <div class="game-guide-text">{{ activeGameGuide.playerFlow }}</div>
          </div>
          <div class="game-guide-block">
            <div class="game-guide-label">What the dealer does</div>
            <div class="game-guide-text">{{ activeGameGuide.dealerFlow }}</div>
          </div>
        </div>
      </div>

      <h2 class="section-title">Utilities</h2>
      <button @click="handleShowCommands" class="save-button">Show Commands</button>
      <button @click="handleOpenLastTrade" class="save-button">
        Open Last Trade: {{ lastTradePartnerName }}
      </button>
      <button @click="handleSkipDiceSetup" class="save-button">
        Skip Dice Setup (Testing)
      </button>

    </div>

    <!-- Trade tab -->
    <div v-if="activeTab === 'Trade'">

      <h2 class="section-title">Double Payout Check</h2>
      <p class="trade-hint" v-if="activeBetSourceLabel">
        Showing {{ activeBetSourceLabel }} data until the round is fully complete.
      </p>
      <div class="trade-empty" v-if="activeBetItems.length === 0">
        No live or saved round bet data yet.
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
      <p class="trade-hint" v-if="activeBetItems.length > 0 && !canCoverPayout">
        You do not have enough stock to return double payout (bet + match).
      </p>
      <p class="trade-hint" v-if="activeBetItems.length > 0 && canCoverPayout">
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

      <!-- Player's Offer -->
      <h2 class="section-title">Player Offer</h2>
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
      <h2 class="section-title">Your Offer To Them</h2>
      <div v-if="ownTradeItems.length === 0" class="trade-empty">
        Nothing added by you yet.
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

    <div v-if="activeTab === 'Game History'">
      <h2 class="section-title">Game History</h2>
      <p class="config-intro">
        Saved on disk and kept between sessions so you can review previous rounds, payout issues, and manual follow-up cases.
      </p>

      <div class="history-actions">
        <button type="button" class="copy-btn history-danger-btn" @click="showClearHistoryConfirm = true">Clear History</button>
      </div>

      <input
        v-model="historySearch"
        type="text"
        class="history-search"
        placeholder="Search by player name"
      />

      <div v-if="filteredGameHistory.length === 0" class="trade-empty">
        No game history matched your search.
      </div>

      <div v-else class="history-list">
        <button
          v-for="entry in filteredGameHistory"
          :key="entry.id"
          type="button"
          class="history-card"
          :class="{ 'history-card-issue': entry.issue }"
          @click="openHistoryEntry(entry)"
        >
          <div class="history-card-top">
            <span class="history-player">{{ entry.playerName || 'Unknown' }}</span>
            <span class="history-status" :class="historyStatusClass(entry)">{{ entry.status || 'Unknown' }}</span>
          </div>
          <div class="history-meta-row">
            <span>{{ entry.game || 'Unknown Game' }}</span>
            <span>{{ formatDateTime(entry.startedAt) }}</span>
          </div>
          <div class="history-meta-row">
            <span>Winner: {{ entry.winner || 'Not Recorded' }}</span>
            <span v-if="entry.issue" class="history-issue-text">Flagged</span>
          </div>
          <div class="history-summary">
            Bet: {{ summarizeTradeItems(entry.betItems) || 'No bet items recorded' }}
          </div>
          <div class="history-summary" v-if="entry.issueReason">
            Issue: {{ entry.issueReason }}
          </div>
          <div class="game-card-action">Click for full details</div>
        </button>
      </div>

      <div class="game-guide-modal-backdrop" v-if="selectedHistory" @click="closeHistoryEntry">
        <div class="game-guide-modal history-modal" @click.stop>
          <div class="game-guide-header">
            <h3 class="section-title game-guide-title">{{ selectedHistory.playerName || 'Unknown' }}</h3>
            <button type="button" class="copy-btn" @click="closeHistoryEntry">Close</button>
          </div>

          <div class="history-detail-grid">
            <div class="history-detail-item">
              <div class="game-guide-label">Game</div>
              <div class="game-guide-text">{{ selectedHistory.game || 'Unknown' }}</div>
            </div>
            <div class="history-detail-item">
              <div class="game-guide-label">Started</div>
              <div class="game-guide-text">{{ formatDateTime(selectedHistory.startedAt) }}</div>
            </div>
            <div class="history-detail-item">
              <div class="game-guide-label">Completed</div>
              <div class="game-guide-text">{{ formatDateTime(selectedHistory.completedAt) || 'Still open / not recorded' }}</div>
            </div>
            <div class="history-detail-item">
              <div class="game-guide-label">Winner</div>
              <div class="game-guide-text">{{ selectedHistory.winner || 'Not Recorded' }}</div>
            </div>
            <div class="history-detail-item">
              <div class="game-guide-label">Status</div>
              <div class="game-guide-text">{{ selectedHistory.status || 'Unknown' }}</div>
            </div>
            <div class="history-detail-item">
              <div class="game-guide-label">Issue</div>
              <div class="game-guide-text">{{ selectedHistory.issue ? (selectedHistory.issueReason || 'Flagged for review') : 'No issue flagged' }}</div>
            </div>
            <div class="history-detail-item">
              <div class="game-guide-label">Player Result</div>
              <div class="game-guide-text">{{ selectedHistory.playerResult || 'Not recorded' }}</div>
            </div>
            <div class="history-detail-item">
              <div class="game-guide-label">Dealer Result</div>
              <div class="game-guide-text">{{ selectedHistory.dealerResult || 'Not recorded' }}</div>
            </div>
          </div>

          <div class="game-guide-block">
            <div class="game-guide-label">Bet Items</div>
            <div class="game-guide-text">{{ summarizeTradeItems(selectedHistory.betItems) || 'No bet items recorded' }}</div>
          </div>
          <div class="game-guide-block">
            <div class="game-guide-label">Payout Items</div>
            <div class="game-guide-text">{{ summarizeTradeItems(selectedHistory.payoutItems) || 'No payout items recorded' }}</div>
          </div>
          <div class="game-guide-block">
            <div class="game-guide-label">Round Notes</div>
            <div v-if="(selectedHistory.notes || []).length === 0" class="game-guide-text">No additional notes recorded.</div>
            <div v-else class="history-notes">
              <div v-for="(note, index) in selectedHistory.notes" :key="`note-${index}`" class="game-guide-text">{{ note }}</div>
            </div>
          </div>
        </div>
      </div>

      <div class="game-guide-modal-backdrop" v-if="showClearHistoryConfirm" @click="closeClearHistoryConfirm">
        <div class="game-guide-modal confirm-modal" @click.stop>
          <div class="game-guide-header">
            <h3 class="section-title game-guide-title">Clear Game History</h3>
          </div>
          <p class="game-guide-text">Are you sure? This will remove all saved game history from the app and delete the persisted history file.</p>
          <div class="confirm-actions">
            <button type="button" class="copy-btn" @click="closeClearHistoryConfirm">Cancel</button>
            <button type="button" class="copy-btn history-danger-btn" @click="confirmClearHistory">Clear Everything</button>
          </div>
        </div>
      </div>
    </div>

    <!-- Logs tab -->
    <div v-if="activeTab === 'Logs'">
      <div class="log-grid">
        <div>
          <div class="log-title-row">
            <h2 class="section-title">Activity Log</h2>
            <button class="copy-btn" @click="copyActivityLogs">Copy</button>
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
        <div>
          <div class="log-title-row">
            <h2 class="section-title">Debugging</h2>
            <button class="copy-btn" @click="copyDebugLogs">Copy</button>
          </div>
          <div ref="debuglogbox" class="log-section debug-log-section">
            <div v-for="(msg, index) in debugLog" :key="`debug-${index}`">{{ msg }}</div>
          </div>
        </div>
      </div>

      <hr class="trade-divider" />

      <h2 class="section-title">Room Identity Map (G_USERS)</h2>
      <div v-if="roomIdentity.length === 0" class="trade-empty">
        No room users decoded yet.<br />
        <span style="font-size:12px;color:#666">Updates when users join/leave and on periodic G_USRS refresh.</span>
      </div>
      <table v-else class="catalog-table">
        <thead><tr><th>Name</th><th>Short</th><th>Chat ID</th><th>Room ID</th><th>Token</th></tr></thead>
        <tbody>
          <tr v-for="(entry, index) in roomIdentity" :key="`room-id-${index}`">
            <td><span class="catalog-label">{{ entry.name || '-' }}</span></td>
            <td><span class="catalog-label">{{ entry.short || '-' }}</span></td>
            <td><span class="catalog-label">{{ entry.chatIndex > 0 ? entry.chatIndex : '-' }}</span></td>
            <td><span class="catalog-label">{{ entry.roomIndex > 0 ? entry.roomIndex : '-' }}</span></td>
            <td><span class="catalog-label">{{ entry.token || '-' }}</span></td>
          </tr>
        </tbody>
      </table>
    </div>

  </div>
</template>

<script>
export default {
  data() {
    return {
      activeTab: 'Config',
      activeGameGuide: null,
      gameGuides: [
        {
          key: 'poker',
          title: 'Poker',
          summary: 'Five dice poker hand showdown with automatic dealer response.',
          description: 'Poker uses five dice and compares the player hand against the dealer hand after the trade is completed. The higher poker hand wins the round.',
          howItWorks: 'The app asks which game the player wants, waits for Poker, then runs the poker sequence, evaluates both hands, and handles the result flow automatically.',
          playerFlow: 'Trade the bet, choose Poker in chat when prompted, then wait for the player and dealer rolls to finish.',
          dealerFlow: 'The dealer records the bet, starts the poker sequence, calculates which hand wins, and prepares payout handling if the player beats the dealer.',
        },
        {
          key: '21',
          title: '21',
          summary: 'Automatic dice roll aiming for a strong total without going too low.',
          description: '21 is a fast internal dice game where the app rolls and evaluates the result automatically. The side with the better valid 21 result wins the round.',
          howItWorks: 'After trade completion the player chooses 21, the app starts the 21 roll flow, compares totals, and announces the result through the normal game pipeline.',
          playerFlow: 'Trade the bet, say 21 when prompted, and let the roll complete.',
          dealerFlow: 'The dealer starts the 21 routine, tracks the totals, decides who won the round, and continues into payout handling if the player wins.',
        },
        {
          key: '13',
          title: '13',
          summary: 'Automatic dice roll flow tuned for the 13 game rules.',
          description: '13 runs as its own internal dice routine after the player picks it from chat. The better valid 13 result wins the round.',
          howItWorks: 'The app listens for the 13 selection, starts the dedicated 13 rolling logic, compares the outcome, then processes the winner the same way as other games.',
          playerFlow: 'Trade the bet, say 13 when prompted, and wait for the roll and outcome.',
          dealerFlow: 'The dealer starts the 13 routine, calculates who won, and manages the rest of the round automatically.',
        },
        {
          key: 'tri',
          title: 'Tri',
          summary: 'Three-dice formation roll available through command flow.',
          description: 'Tri is a command-driven roll mode using three dice instead of the trade-selected round flow.',
          howItWorks: 'It is triggered from chat commands and rolls three dice in the configured formation. This is a utility roll mode rather than the main trade-game flow.',
          playerFlow: 'Use the Tri command when needed and let the app roll the three dice.',
          dealerFlow: 'The dealer/app handles the roll output and result logging automatically, but the house rules for who wins depend on how you are using the tri command.',
        },
        {
          key: 'roll',
          title: 'Standard Roll',
          summary: 'Basic five-dice roll and result output using commands.',
          description: 'Standard Roll is the quick manual dice roll mode available from the command list.',
          howItWorks: 'The app rolls the available dice, tracks the values, and outputs the result. This is a utility roll and does not decide a winner by itself.',
          playerFlow: 'Use the roll command when you want a standard five-dice result outside the trade-selected games.',
          dealerFlow: 'The dealer/app tracks the active dice, gathers the results, and logs the final roll for you to use however you want.',
        },
      ],
      tradeItems: [],
      activeGameBetItems: [],
      ownTradeItems: [],
      handItems: [],
      gameHistory: [],
      historySearch: '',
      selectedHistory: null,
      showClearHistoryConfirm: false,
      roomIdentity: [],
      log: [],
      debugLog: [],
      chatLog: [],
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

      const liveIncomingByName = this.tradeItems.reduce((acc, item) => {
        acc[item.Name] = (acc[item.Name] || 0) + item.Quantity;
        return acc;
      }, {});

      return this.activeBetItemsWithDisplay.map(item => {
        const required = item.Quantity;
        const payoutTotal = item.Quantity * 2;
        const includeLiveIncoming = this.tradeItems.length > 0 ? (liveIncomingByName[item.Name] || 0) : 0;
        const have = (handByName[item.Name] || 0) + includeLiveIncoming;
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
    activeBetItems() {
      return this.tradeItems.length > 0 ? this.tradeItems : this.activeGameBetItems;
    },
    activeBetItemsWithDisplay() {
      return this.activeBetItems.map(item => {
        return { ...item, displayName: this.formatItemName(item.Name) };
      });
    },
    activeBetSourceLabel() {
      if (this.tradeItems.length > 0) {
        return 'live trade';
      }
      if (this.activeGameBetItems.length > 0) {
        return 'current round';
      }
      return '';
    },
    filteredGameHistory() {
      const query = this.historySearch.trim().toLowerCase();
      if (!query) {
        return this.gameHistory;
      }
      return this.gameHistory.filter((entry) => String(entry.playerName || '').toLowerCase().includes(query));
    },
  },
  methods: {
    openGameGuide(game) {
      this.activeGameGuide = game;
    },
    closeGameGuide() {
      this.activeGameGuide = null;
    },
    openHistoryEntry(entry) {
      this.selectedHistory = entry;
    },
    closeHistoryEntry() {
      this.selectedHistory = null;
    },
    closeClearHistoryConfirm() {
      this.showClearHistoryConfirm = false;
    },
    async confirmClearHistory() {
      try {
        await window.go.main.App.ClearGameHistory();
        this.selectedHistory = null;
        this.historySearch = '';
        this.showClearHistoryConfirm = false;
        this.addLogMsg('[UI] Cleared game history');
      } catch (error) {
        this.addLogMsg('Error clearing game history');
        console.error(error);
      }
    },
    historyStatusClass(entry) {
      if (entry.issue) {
        return 'history-status-issue';
      }
      const status = String(entry.status || '').toLowerCase();
      if (status.includes('completed')) {
        return 'history-status-complete';
      }
      if (status.includes('pending') || status.includes('awaiting')) {
        return 'history-status-pending';
      }
      if (status.includes('result')) {
        return 'history-status-info';
      }
      return 'history-status-info';
    },
    summarizeTradeItems(items) {
      return (items || []).map((item) => `${item.Quantity}x ${this.formatItemName(item.Name)}`).join(', ');
    },
    formatDateTime(value) {
      if (!value) {
        return '';
      }
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) {
        return value;
      }
      return date.toLocaleString();
    },
    async refreshGameHistory() {
      try {
        const jsonStr = await window.go.main.App.GetGameHistoryJSON();
        this.gameHistory = JSON.parse(jsonStr || '[]') || [];
      } catch (error) {
        this.addLogMsg('Error loading game history');
        console.error(error);
      }
    },
     async handleShowCommands() {
      try {
        await window.go.main.App.ShowCommands();
      } catch (error) {
        this.addLogMsg('Error showing commands');
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
    async copyActivityLogs() {
      const text = this.log.join('\n');
      await this.copyTextToClipboard(text, 'activity logs');
    },
    async copyChatLogs() {
      const text = this.chatLog.join('\n');
      await this.copyTextToClipboard(text, 'chat logs');
    },
    async copyDebugLogs() {
      const text = this.debugLog.join('\n');
      await this.copyTextToClipboard(text, 'debug logs');
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
  },
  async mounted() {
    await this.refreshLastTradePartnerName();
    await this.refreshGameHistory();
    window.runtime.EventsOn("logUpdate", (message) => {
      this.log = message.split('\n');
      this.scrollBox('logbox');
    });
    window.runtime.EventsOn("debugLogUpdate", (message) => {
      this.debugLog = message.split('\n');
      this.scrollBox('debuglogbox');
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

    window.runtime.EventsOn("activeGameBetItemsUpdate", (jsonStr) => {
      try {
        this.activeGameBetItems = (JSON.parse(jsonStr) || []).map(item => ({
          ...item,
          displayName: this.formatItemName(item.Name),
        }));
      } catch (_) {
        this.activeGameBetItems = [];
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

    window.runtime.EventsOn("roomIdentityUpdate", (jsonStr) => {
      try {
        this.roomIdentity = JSON.parse(jsonStr) || [];
      } catch (_) {
        this.roomIdentity = [];
      }
    });
    window.runtime.EventsOn("gameHistoryUpdate", (jsonStr) => {
      try {
        this.gameHistory = JSON.parse(jsonStr) || [];
      } catch (_) {
        this.gameHistory = [];
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

.config-intro {
  margin: 0 0 16px;
  color: #b8b8b8;
  font-size: 14px;
  line-height: 1.6;
  text-align: center;
}

.game-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  margin-bottom: 18px;
}

.game-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
  padding: 14px;
  background: #181818;
  border: 1px solid #3a3a3a;
  border-radius: 8px;
  color: #e8e8e8;
  text-align: left;
  cursor: pointer;
}

.game-card:hover {
  background: #202020;
  border-color: #5a5a5a;
}

.game-card-title {
  font-size: 15px;
  font-weight: 700;
}

.game-card-summary {
  color: #a6a6a6;
  font-size: 13px;
  line-height: 1.5;
}

.game-card-action {
  color: #ffd700;
  font-size: 12px;
}

.game-guide-modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  z-index: 999;
}

.game-guide-modal {
  width: min(680px, 100%);
  background: #141414;
  border: 1px solid #444;
  border-radius: 10px;
  padding: 18px;
}

.game-guide-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.game-guide-title {
  margin: 0;
  text-align: left;
}

.game-guide-block {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid #2f2f2f;
}

.game-guide-label {
  color: #ffd700;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
  margin-bottom: 6px;
}

.game-guide-text {
  color: #d0d0d0;
  font-size: 14px;
  line-height: 1.6;
}

.history-search {
  width: 100%;
  padding: 10px 12px;
  background-color: #1a1a1a;
  border: 1px solid #3f3f3f;
  border-radius: 8px;
  color: #f0f0f0;
  margin-bottom: 14px;
  box-sizing: border-box;
}

.history-actions {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}

.history-list {
  display: grid;
  gap: 12px;
}

.history-card {
  padding: 14px;
  background: #161616;
  border: 1px solid #343434;
  border-radius: 8px;
  text-align: left;
  color: #efefef;
  cursor: pointer;
}

.history-card-issue {
  border-color: #a94442;
  background: #1d1414;
}

.history-danger-btn {
  background: #4a1f1f;
  border-color: #a94442;
  color: #ffd0d0;
}

.history-danger-btn:hover {
  background: #5a2323;
}

.history-card-top,
.history-meta-row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.history-player {
  font-size: 15px;
  font-weight: 700;
}

.history-status {
  padding: 3px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
}

.history-status-complete {
  background: #16351f;
  color: #8ef0aa;
}

.history-status-pending {
  background: #3f3113;
  color: #ffd36d;
}

.history-status-info {
  background: #1b3042;
  color: #8fc8ff;
}

.history-status-issue {
  background: #4a1f1f;
  color: #ff9e9e;
}

.history-summary,
.history-issue-text {
  color: #bdbdbd;
  font-size: 13px;
  line-height: 1.5;
}

.history-issue-text {
  color: #ff9e9e;
}

.history-modal {
  max-height: 85vh;
  overflow-y: auto;
}

.history-detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.history-detail-item {
  padding: 10px;
  background: #191919;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
}

.history-notes {
  display: grid;
  gap: 8px;
}

.confirm-modal {
  width: min(520px, 100%);
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
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

.debug-log-section {
  color: #fda4af;
  border-color: #fda4af;
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