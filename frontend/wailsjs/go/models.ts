export namespace main {
	
	export class IssueStats {
	    totalIssues: number;
	    byReason: Record<string, number>;
	    gameChoiceTimeouts: number;
	    payoutTimeouts: number;
	    payoutCancelFlags: number;
	    tradeConfirmTimeouts: number;
	
	    static createFrom(source: any = {}) {
	        return new IssueStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalIssues = source["totalIssues"];
	        this.byReason = source["byReason"];
	        this.gameChoiceTimeouts = source["gameChoiceTimeouts"];
	        this.payoutTimeouts = source["payoutTimeouts"];
	        this.payoutCancelFlags = source["payoutCancelFlags"];
	        this.tradeConfirmTimeouts = source["tradeConfirmTimeouts"];
	    }
	}
	export class StreakStats {
	    currentDealerWinStreak: number;
	    currentPlayerWinStreak: number;
	    longestDealerWinStreak: number;
	    longestPlayerWinStreak: number;
	
	    static createFrom(source: any = {}) {
	        return new StreakStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentDealerWinStreak = source["currentDealerWinStreak"];
	        this.currentPlayerWinStreak = source["currentPlayerWinStreak"];
	        this.longestDealerWinStreak = source["longestDealerWinStreak"];
	        this.longestPlayerWinStreak = source["longestPlayerWinStreak"];
	    }
	}
	export class PlayerStats {
	    playerName: string;
	    totalRounds: number;
	    playerWins: number;
	    dealerWins: number;
	    issueRounds: number;
	    betItemsIn: number;
	    payoutItemsOut: number;
	    netAgainstCasino: number;
	    byGameRounds: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new PlayerStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.playerName = source["playerName"];
	        this.totalRounds = source["totalRounds"];
	        this.playerWins = source["playerWins"];
	        this.dealerWins = source["dealerWins"];
	        this.issueRounds = source["issueRounds"];
	        this.betItemsIn = source["betItemsIn"];
	        this.payoutItemsOut = source["payoutItemsOut"];
	        this.netAgainstCasino = source["netAgainstCasino"];
	        this.byGameRounds = source["byGameRounds"];
	    }
	}
	export class ItemStats {
	    name: string;
	    betIn: number;
	    payoutOut: number;
	    net: number;
	    byGameBetIn: Record<string, number>;
	    byGamePayoutOut: Record<string, number>;
	    byGameNet: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new ItemStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.betIn = source["betIn"];
	        this.payoutOut = source["payoutOut"];
	        this.net = source["net"];
	        this.byGameBetIn = source["byGameBetIn"];
	        this.byGamePayoutOut = source["byGamePayoutOut"];
	        this.byGameNet = source["byGameNet"];
	    }
	}
	export class GameStats {
	    game: string;
	    totalRounds: number;
	    completedRounds: number;
	    issueRounds: number;
	    playerWins: number;
	    dealerWins: number;
	    pushes: number;
	    playerWinRate: number;
	    dealerWinRate: number;
	    totalBetItemsIn: number;
	    totalPayoutItemsOut: number;
	    netItems: number;
	    averageBetSize: number;
	    largestBet: number;
	    largestPayout: number;
	    worstCasinoLoss: number;
	    bestCasinoWin: number;
	    betItemCounts: Record<string, number>;
	    payoutItemCounts: Record<string, number>;
	    netItemCounts: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new GameStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.game = source["game"];
	        this.totalRounds = source["totalRounds"];
	        this.completedRounds = source["completedRounds"];
	        this.issueRounds = source["issueRounds"];
	        this.playerWins = source["playerWins"];
	        this.dealerWins = source["dealerWins"];
	        this.pushes = source["pushes"];
	        this.playerWinRate = source["playerWinRate"];
	        this.dealerWinRate = source["dealerWinRate"];
	        this.totalBetItemsIn = source["totalBetItemsIn"];
	        this.totalPayoutItemsOut = source["totalPayoutItemsOut"];
	        this.netItems = source["netItems"];
	        this.averageBetSize = source["averageBetSize"];
	        this.largestBet = source["largestBet"];
	        this.largestPayout = source["largestPayout"];
	        this.worstCasinoLoss = source["worstCasinoLoss"];
	        this.bestCasinoWin = source["bestCasinoWin"];
	        this.betItemCounts = source["betItemCounts"];
	        this.payoutItemCounts = source["payoutItemCounts"];
	        this.netItemCounts = source["netItemCounts"];
	    }
	}
	export class CasinoStatsSummary {
	    totalRounds: number;
	    completedRounds: number;
	    issueRounds: number;
	    playerWins: number;
	    dealerWins: number;
	    pushes: number;
	    playerWinRate: number;
	    dealerWinRate: number;
	    issueRate: number;
	    totalBetItemsIn: number;
	    totalPayoutItemsOut: number;
	    netItems: number;
	    rtpPercent: number;
	    profitMarginPercent: number;
	    betItemCounts: Record<string, number>;
	    payoutItemCounts: Record<string, number>;
	    netItemCounts: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new CasinoStatsSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalRounds = source["totalRounds"];
	        this.completedRounds = source["completedRounds"];
	        this.issueRounds = source["issueRounds"];
	        this.playerWins = source["playerWins"];
	        this.dealerWins = source["dealerWins"];
	        this.pushes = source["pushes"];
	        this.playerWinRate = source["playerWinRate"];
	        this.dealerWinRate = source["dealerWinRate"];
	        this.issueRate = source["issueRate"];
	        this.totalBetItemsIn = source["totalBetItemsIn"];
	        this.totalPayoutItemsOut = source["totalPayoutItemsOut"];
	        this.netItems = source["netItems"];
	        this.rtpPercent = source["rtpPercent"];
	        this.profitMarginPercent = source["profitMarginPercent"];
	        this.betItemCounts = source["betItemCounts"];
	        this.payoutItemCounts = source["payoutItemCounts"];
	        this.netItemCounts = source["netItemCounts"];
	    }
	}
	export class StatsRange {
	    key: string;
	    startAt?: string;
	    endAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new StatsRange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.startAt = source["startAt"];
	        this.endAt = source["endAt"];
	    }
	}
	export class CasinoStats {
	    generatedAt: string;
	    range: StatsRange;
	    overall: CasinoStatsSummary;
	    byGame: Record<string, GameStats>;
	    byItem: ItemStats[];
	    byPlayer: PlayerStats[];
	    streaks: StreakStats;
	    issues: IssueStats;
	
	    static createFrom(source: any = {}) {
	        return new CasinoStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generatedAt = source["generatedAt"];
	        this.range = this.convertValues(source["range"], StatsRange);
	        this.overall = this.convertValues(source["overall"], CasinoStatsSummary);
	        this.byGame = this.convertValues(source["byGame"], GameStats, true);
	        this.byItem = this.convertValues(source["byItem"], ItemStats);
	        this.byPlayer = this.convertValues(source["byPlayer"], PlayerStats);
	        this.streaks = this.convertValues(source["streaks"], StreakStats);
	        this.issues = this.convertValues(source["issues"], IssueStats);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class CatalogItem {
	    name: string;
	    display_name: string;
	    value: number;
	
	    static createFrom(source: any = {}) {
	        return new CatalogItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.display_name = source["display_name"];
	        this.value = source["value"];
	    }
	}
	
	
	
	
	export class PokerDisplayConfig {
	    five_of_a_kind: string;
	    four_of_a_kind: string;
	    full_house: string;
	    high_straight: string;
	    low_straight: string;
	    three_of_a_kind: string;
	    two_pair: string;
	    one_pair: string;
	    nothing: string;
	
	    static createFrom(source: any = {}) {
	        return new PokerDisplayConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.five_of_a_kind = source["five_of_a_kind"];
	        this.four_of_a_kind = source["four_of_a_kind"];
	        this.full_house = source["full_house"];
	        this.high_straight = source["high_straight"];
	        this.low_straight = source["low_straight"];
	        this.three_of_a_kind = source["three_of_a_kind"];
	        this.two_pair = source["two_pair"];
	        this.one_pair = source["one_pair"];
	        this.nothing = source["nothing"];
	    }
	}
	
	
	export class TradeItem {
	    Name: string;
	    Quantity: number;
	    RawData: string;
	
	    static createFrom(source: any = {}) {
	        return new TradeItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Quantity = source["Quantity"];
	        this.RawData = source["RawData"];
	    }
	}

}

