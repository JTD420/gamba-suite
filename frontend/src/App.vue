<template>
  <div class="poker-config-section">
    <h2 class="section-title">Poker Hand Configurations</h2>
    <form @submit.prevent="saveConfig">
      <div class="form-group" v-for="(value, key) in config" :key="key">
        <label :for="key">{{ formatLabel(key) }}:</label>
        <input v-model="config[key]" type="text" :id="key" />
      </div>
      <button type="submit" class="save-button">Save</button>
    </form>

    <button @click="handleShowCommands" class="show-commands-button save-button">Show Commands</button>

    <!-- Update notice -->
    <div v-if="isOutdated" class="update-notice">
      A new version of this application is available. Please update to the latest version.
    </div>

    <h2 class="section-title">Roll Logs</h2>
    <div id="log" ref="logbox" class="log-section">
      <div v-for="(msg, index) in log" :key="index">{{ msg }}</div>
    </div>
  </div>
</template>

<script>
export default {
  data() {
    return {
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
      log: [],
      isOutdated: false, // Add this line to initialize isOutdated
      currentVersion: "", // Will be fetched from backend
    };
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
    addLogMsg(msg) {
      this.log.push(msg);
      this.$nextTick(() => {
        const logbox = this.$refs.logbox;
        logbox.scrollTop = logbox.scrollHeight;
      });
    },
    formatLabel(key) {
      return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
    },
    scrolldown() {
      this.$nextTick(() => {
        const box = this.$refs.logbox;
        box.scrollTop = box.scrollHeight;
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
    await this.checkForUpdates();
    window.runtime.EventsOn("logUpdate", (message) => {
      this.log = message.split('\n');
      this.scrolldown();
    });
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
</style>