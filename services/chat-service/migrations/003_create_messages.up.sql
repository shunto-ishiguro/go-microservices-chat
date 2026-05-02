CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id         UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    sender_id       UUID NOT NULL,
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- (room_id, created_at, id) で履歴の cursor pagination を高速化。
-- ListByRoom が (created_at, id) DESC で並べる + (created_at, id) < cursor の WHERE を打つので、
-- room_id で絞り込んだ後 created_at + id でインデックススキャンが効く。
CREATE INDEX idx_messages_room_created ON messages(room_id, created_at DESC, id DESC);
