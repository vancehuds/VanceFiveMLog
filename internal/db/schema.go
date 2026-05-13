package db

const schemaSQL = `
CREATE TABLE IF NOT EXISTS admins (
	id BIGSERIAL PRIMARY KEY,
	username TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'admin',
	active BOOLEAN NOT NULL DEFAULT true,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS servers (
	id BIGSERIAL PRIMARY KEY,
	name TEXT UNIQUE NOT NULL,
	api_key_hash TEXT NOT NULL,
	active BOOLEAN NOT NULL DEFAULT true,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	rotated_at TIMESTAMPTZ,
	last_seen_at TIMESTAMPTZ,
	last_event_at TIMESTAMPTZ
);

ALTER TABLE servers ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS last_event_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS log_events (
	id BIGSERIAL PRIMARY KEY,
	server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	event_type TEXT NOT NULL,
	severity TEXT NOT NULL DEFAULT 'info',
	player_source INTEGER,
	player_name TEXT,
	license TEXT,
	discord TEXT,
	steam TEXT,
	citizenid TEXT,
	resource TEXT,
	message TEXT NOT NULL,
	coords_x DOUBLE PRECISION,
	coords_y DOUBLE PRECISION,
	coords_z DOUBLE PRECISION,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS log_event_reviews (
	event_id BIGINT PRIMARY KEY REFERENCES log_events(id) ON DELETE CASCADE,
	status TEXT NOT NULL DEFAULT 'normal',
	note TEXT NOT NULL DEFAULT '',
	archived_at TIMESTAMPTZ,
	archived_by BIGINT REFERENCES admins(id) ON DELETE SET NULL,
	updated_by BIGINT REFERENCES admins(id) ON DELETE SET NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS log_admin_audit (
	id BIGSERIAL PRIMARY KEY,
	admin_id BIGINT REFERENCES admins(id) ON DELETE SET NULL,
	admin_username TEXT NOT NULL,
	action TEXT NOT NULL,
	event_id BIGINT,
	event_ids BIGINT[] NOT NULL DEFAULT '{}'::bigint[],
	query JSONB NOT NULL DEFAULT '{}'::jsonb,
	details JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ai_json_methods (
	id BIGSERIAL PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT 'metadata',
	event_type TEXT,
	resource TEXT,
	prompt TEXT NOT NULL DEFAULT '',
	spec JSONB NOT NULL DEFAULT '{}'::jsonb,
	active BOOLEAN NOT NULL DEFAULT true,
	created_by BIGINT REFERENCES admins(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_log_events_occurred_id_desc ON log_events (occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_log_events_server_occurred_id_desc ON log_events (server_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_log_events_type_occurred_id_desc ON log_events (event_type, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_log_events_severity_occurred ON log_events (severity, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_log_events_resource_occurred ON log_events (resource, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_log_events_retention ON log_events (occurred_at ASC, id ASC);
CREATE INDEX IF NOT EXISTS idx_log_events_license ON log_events (license);
CREATE INDEX IF NOT EXISTS idx_log_events_citizenid ON log_events (citizenid);
CREATE INDEX IF NOT EXISTS idx_log_events_discord ON log_events (discord);
CREATE INDEX IF NOT EXISTS idx_log_events_steam ON log_events (steam);
CREATE INDEX IF NOT EXISTS idx_log_events_metadata ON log_events USING GIN (metadata);
CREATE INDEX IF NOT EXISTS idx_servers_api_key_hash_active ON servers (api_key_hash) WHERE active = true;
CREATE INDEX IF NOT EXISTS idx_servers_last_seen ON servers (last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_log_event_reviews_status ON log_event_reviews (status);
CREATE INDEX IF NOT EXISTS idx_log_event_reviews_archived ON log_event_reviews (archived_at DESC) WHERE archived_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_log_admin_audit_event ON log_admin_audit (event_id);
CREATE INDEX IF NOT EXISTS idx_log_admin_audit_created ON log_admin_audit (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_json_methods_active ON ai_json_methods (active, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_json_methods_scope ON ai_json_methods (event_type, resource) WHERE active = true;

INSERT INTO settings (key, value)
VALUES ('retention_days', '180')
ON CONFLICT (key) DO NOTHING;
`
