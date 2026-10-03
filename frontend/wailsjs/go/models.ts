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
	export class SystemStatus {
	    dataDir: string;
	    schemaVersion: number;
	    secretsPersistent: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dataDir = source["dataDir"];
	        this.schemaVersion = source["schemaVersion"];
	        this.secretsPersistent = source["secretsPersistent"];
	        this.error = source["error"];
	    }
	}

}

