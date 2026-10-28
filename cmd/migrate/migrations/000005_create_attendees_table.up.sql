CREATE TABLE attendees (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id  INTEGER NOT NULL,
    user_id    INTEGER NOT NULL,
    event_id   INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT attendees_tenant_event_user_unique
        UNIQUE (tenant_id, event_id, user_id),
    CONSTRAINT attendees_event_same_tenant_fk
        FOREIGN KEY (tenant_id, event_id)
        REFERENCES events (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT attendees_user_same_tenant_fk
        FOREIGN KEY (tenant_id, user_id)
        REFERENCES users (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX attendees_tenant_user_idx
    ON attendees (tenant_id, user_id, event_id);
CREATE INDEX attendees_tenant_event_idx
    ON attendees (tenant_id, event_id, user_id);
