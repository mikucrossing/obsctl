export namespace btsync {
	
	export class PeerStatus {
	    peer_id: string;
	    name: string;
	    platform: string;
	    connected: boolean;
	    last_seen_unix_ms?: number;
	    last_ack_unix_ms?: number;
	    last_ack_status?: string;
	    last_latency_ms?: number;
	
	    static createFrom(source: any = {}) {
	        return new PeerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peer_id = source["peer_id"];
	        this.name = source["name"];
	        this.platform = source["platform"];
	        this.connected = source["connected"];
	        this.last_seen_unix_ms = source["last_seen_unix_ms"];
	        this.last_ack_unix_ms = source["last_ack_unix_ms"];
	        this.last_ack_status = source["last_ack_status"];
	        this.last_latency_ms = source["last_latency_ms"];
	    }
	}
	export class Status {
	    supported: boolean;
	    supported_role_parent: boolean;
	    supported_role_child: boolean;
	    unsupported_reason?: string;
	    running: boolean;
	    enabled: boolean;
	    role: string;
	    device_name: string;
	    connected_peers: number;
	    parent_connected: boolean;
	    pairing_code_active: boolean;
	    pairing_code_expires_unix_ms?: number;
	    last_error?: string;
	    peers: PeerStatus[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.supported_role_parent = source["supported_role_parent"];
	        this.supported_role_child = source["supported_role_child"];
	        this.unsupported_reason = source["unsupported_reason"];
	        this.running = source["running"];
	        this.enabled = source["enabled"];
	        this.role = source["role"];
	        this.device_name = source["device_name"];
	        this.connected_peers = source["connected_peers"];
	        this.parent_connected = source["parent_connected"];
	        this.pairing_code_active = source["pairing_code_active"];
	        this.pairing_code_expires_unix_ms = source["pairing_code_expires_unix_ms"];
	        this.last_error = source["last_error"];
	        this.peers = this.convertValues(source["peers"], PeerStatus);
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

export namespace config {
	
	export class TrustedPeer {
	    peer_id: string;
	    name: string;
	    secret: string;
	    last_seen: string;
	    platform: string;
	
	    static createFrom(source: any = {}) {
	        return new TrustedPeer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peer_id = source["peer_id"];
	        this.name = source["name"];
	        this.secret = source["secret"];
	        this.last_seen = source["last_seen"];
	        this.platform = source["platform"];
	    }
	}
	export class BluetoothSyncConfig {
	    enabled: boolean;
	    role: string;
	    device_name: string;
	    lead_time_ms: number;
	    pairing_code_ttl_sec: number;
	    accept_late_ms: number;
	    max_nodes: number;
	    auto_reconnect: boolean;
	    drop_missed_events: boolean;
	    trusted_peers: TrustedPeer[];
	
	    static createFrom(source: any = {}) {
	        return new BluetoothSyncConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.role = source["role"];
	        this.device_name = source["device_name"];
	        this.lead_time_ms = source["lead_time_ms"];
	        this.pairing_code_ttl_sec = source["pairing_code_ttl_sec"];
	        this.accept_late_ms = source["accept_late_ms"];
	        this.max_nodes = source["max_nodes"];
	        this.auto_reconnect = source["auto_reconnect"];
	        this.drop_missed_events = source["drop_missed_events"];
	        this.trusted_peers = this.convertValues(source["trusted_peers"], TrustedPeer);
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
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.addr = source["addr"];
	        this.enabled = source["enabled"];
	        this.password = source["password"];
	    }
	}
	export class Config {
	    connections: Connection[];
	    common_password: string;
	    import_defaults: ImportDefaults;
	    midi: MidiConfig;
	    bluetooth: BluetoothSyncConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connections = this.convertValues(source["connections"], Connection);
	        this.common_password = source["common_password"];
	        this.import_defaults = this.convertValues(source["import_defaults"], ImportDefaults);
	        this.midi = this.convertValues(source["midi"], MidiConfig);
	        this.bluetooth = this.convertValues(source["bluetooth"], BluetoothSyncConfig);
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

