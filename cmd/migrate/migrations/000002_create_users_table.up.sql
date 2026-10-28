CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id     INTEGER NOT NULL,
    email         TEXT NOT NULL COLLATE NOCASE UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    bio           TEXT NOT NULL DEFAULT '',
    avatar_url    TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'member'
                  CHECK (role IN ('owner', 'member')),
    status        TEXT NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'disabled')),
    created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT users_tenant_id_id_unique UNIQUE (tenant_id, id),
    CONSTRAINT users_tenant_fk
        FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE
);

CREATE INDEX users_tenant_role_status_idx
    ON users (tenant_id, role, status);
