export namespace main {
	
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

}

