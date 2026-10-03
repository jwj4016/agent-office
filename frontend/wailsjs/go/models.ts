export namespace main {
	
	export class PingResult {
	    sequence: number;
	    message: string;
	    at: string;
	
	    static createFrom(source: any = {}) {
	        return new PingResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sequence = source["sequence"];
	        this.message = source["message"];
	        this.at = source["at"];
	    }
	}

}

