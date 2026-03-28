CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS tailnets (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS servers (
    id BIGSERIAL PRIMARY KEY,
    tailnet_id BIGINT NOT NULL REFERENCES tailnets(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    tailscale_ip INET NOT NULL,
    ssh_user TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    UNIQUE (tailnet_id, name),
    UNIQUE (tailnet_id, tailscale_ip)
);

CREATE TABLE IF NOT EXISTS tags (
    id BIGSERIAL PRIMARY KEY,
    tailnet_id BIGINT NOT NULL REFERENCES tailnets(id) ON DELETE CASCADE,
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tailnet_id, value)
);

CREATE TABLE IF NOT EXISTS server_tags (
    server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (server_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_servers_tailnet_name ON servers (tailnet_id, name);
CREATE INDEX IF NOT EXISTS idx_servers_tailnet_ssh_user ON servers (tailnet_id, ssh_user);
CREATE INDEX IF NOT EXISTS idx_server_tags_tag_id ON server_tags (tag_id);
CREATE INDEX IF NOT EXISTS idx_tags_tailnet_value ON tags (tailnet_id, value);

CREATE INDEX IF NOT EXISTS idx_servers_name_trgm
    ON servers USING GIN (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_servers_ssh_user_trgm
    ON servers USING GIN (ssh_user gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_servers_tailscale_ip_trgm
    ON servers USING GIN ((host(tailscale_ip)) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_tags_value_trgm
    ON tags USING GIN (value gin_trgm_ops);
