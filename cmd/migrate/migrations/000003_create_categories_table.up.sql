CREATE TABLE categories (
    tenant_id  INTEGER NOT NULL,
    id         TEXT NOT NULL,
    name       TEXT NOT NULL,
    name_key   TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0
               CHECK (is_default IN (0, 1)),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT categories_tenant_name_key_unique UNIQUE (tenant_id, name_key),
    CONSTRAINT categories_tenant_fk
        FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE
);

CREATE INDEX categories_tenant_default_name_idx
    ON categories (tenant_id, is_default DESC, name COLLATE NOCASE ASC);
