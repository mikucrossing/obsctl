export namespace config {
	
	export class MidiConfig {
	    enabled: boolean;
	    device: string;
	    channel: string;
	    debounce: string;
	    rate_limit: string;
	    mappings: string[];
	
	    static createFrom(source: any = {}) {
	        return new MidiConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.device = source["device"];
	        this.channel = source["channel"];
	        this.debounce = source["debounce"];
	        this.rate_limit = source["rate_limit"];
	        this.mappings = source["mappings"];
	    }
	}
	export class ImportDefaults {
	    loop: boolean;
	    activate: boolean;
	    transition: string;
	    monitoring: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportDefaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loop = source["loop"];
	        this.activate = source["activate"];
	        this.transition = source["transition"];
	        this.monitoring = source["monitoring"];
	    }
	}
	export class Connection {
	    name: string;
	    addr: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.addr = source["addr"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Config {
	    connections: Connection[];
	    common_password: string;
	    import_defaults: ImportDefaults;
	    midi: MidiConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connections = this.convertValues(source["connections"], Connection);
	        this.common_password = source["common_password"];
	        this.import_defaults = this.convertValues(source["import_defaults"], ImportDefaults);
	        this.midi = this.convertValues(source["midi"], MidiConfig);
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
	
	

}

