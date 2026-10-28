CREATE TABLE events (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id        INTEGER NOT NULL,
    owner_id         INTEGER NOT NULL,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL,
    image_url        TEXT NOT NULL DEFAULT '',
    location         TEXT NOT NULL,
    start_date_time  TEXT NOT NULL,
    end_date_time    TEXT NOT NULL,
    category_id      TEXT NOT NULL,
    url              TEXT NOT NULL DEFAULT '',
    CONSTRAINT events_tenant_id_id_unique UNIQUE (tenant_id, id),
    CONSTRAINT events_owner_same_tenant_fk
        FOREIGN KEY (tenant_id, owner_id)
        REFERENCES users (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT events_category_same_tenant_fk
        FOREIGN KEY (tenant_id, category_id)
        REFERENCES categories (tenant_id, id) ON DELETE NO ACTION
);

CREATE INDEX events_tenant_start_time_idx
    ON events (tenant_id, start_date_time);
CREATE INDEX events_tenant_owner_idx
    ON events (tenant_id, owner_id);
CREATE INDEX events_tenant_category_idx
    ON events (tenant_id, category_id);