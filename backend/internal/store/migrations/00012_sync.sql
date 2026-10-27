-- +goose Up
-- Phones keep a copy of each collection and ask only for what changed
-- since they last asked. Every write to a photo or subject, by whatever
-- code path, stamps it with the transaction that made it; a change to a
-- photo's tags, or to the name of the subject it is filed under, stamps the
-- photo too; deleting one leaves a tombstone. So no change can be missed.
-- Stamps are transaction ids as bigint (xid8 counts up and never wraps).
ALTER TABLE photos ADD COLUMN sync_xid bigint NOT NULL DEFAULT (pg_current_xact_id()::text::bigint);
ALTER TABLE subjects ADD COLUMN sync_xid bigint NOT NULL DEFAULT (pg_current_xact_id()::text::bigint);
CREATE INDEX photos_sync_idx ON photos (team_id, sync_xid);
CREATE INDEX subjects_sync_idx ON subjects (team_id, sync_xid);

CREATE TABLE sync_tombstones (
    team_id    uuid NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('photo', 'subject')),
    id         uuid NOT NULL,
    sync_xid   bigint NOT NULL DEFAULT (pg_current_xact_id()::text::bigint),
    deleted_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sync_tombstones_team_idx ON sync_tombstones (team_id, sync_xid);
CREATE INDEX sync_tombstones_age_idx ON sync_tombstones (deleted_at);

-- +goose StatementBegin
CREATE FUNCTION sync_stamp() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.sync_xid := pg_current_xact_id()::text::bigint;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER photos_sync_stamp BEFORE UPDATE ON photos FOR EACH ROW EXECUTE FUNCTION sync_stamp();
CREATE TRIGGER subjects_sync_stamp BEFORE UPDATE ON subjects FOR EACH ROW EXECUTE FUNCTION sync_stamp();

-- Setting the stamp to anything has the stamp trigger set it right.
-- +goose StatementBegin
CREATE FUNCTION sync_stamp_tagged() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE photos SET sync_xid = 0 WHERE id IN (SELECT DISTINCT photo_id FROM changed);
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER photo_tags_sync_insert AFTER INSERT ON photo_tags
    REFERENCING NEW TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION sync_stamp_tagged();
CREATE TRIGGER photo_tags_sync_update AFTER UPDATE ON photo_tags
    REFERENCING NEW TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION sync_stamp_tagged();
CREATE TRIGGER photo_tags_sync_delete AFTER DELETE ON photo_tags
    REFERENCING OLD TABLE AS changed FOR EACH STATEMENT EXECUTE FUNCTION sync_stamp_tagged();

-- Descriptions call a photo's subject by name.
-- +goose StatementBegin
CREATE FUNCTION sync_stamp_renamed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE photos SET sync_xid = 0 WHERE subject_id = NEW.id;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER subjects_sync_renamed AFTER UPDATE OF display_name ON subjects
    FOR EACH ROW WHEN (OLD.display_name IS DISTINCT FROM NEW.display_name) EXECUTE FUNCTION sync_stamp_renamed();

-- +goose StatementBegin
CREATE FUNCTION sync_tombstone() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO sync_tombstones (team_id, kind, id) SELECT team_id, TG_ARGV[0], id FROM gone;
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER photos_sync_tombstone AFTER DELETE ON photos
    REFERENCING OLD TABLE AS gone FOR EACH STATEMENT EXECUTE FUNCTION sync_tombstone('photo');
CREATE TRIGGER subjects_sync_tombstone AFTER DELETE ON subjects
    REFERENCING OLD TABLE AS gone FOR EACH STATEMENT EXECUTE FUNCTION sync_tombstone('subject');

-- +goose Down
DROP TRIGGER subjects_sync_tombstone ON subjects;
DROP TRIGGER photos_sync_tombstone ON photos;
DROP TRIGGER subjects_sync_renamed ON subjects;
DROP TRIGGER photo_tags_sync_delete ON photo_tags;
DROP TRIGGER photo_tags_sync_update ON photo_tags;
DROP TRIGGER photo_tags_sync_insert ON photo_tags;
DROP TRIGGER subjects_sync_stamp ON subjects;
DROP TRIGGER photos_sync_stamp ON photos;
DROP FUNCTION sync_tombstone, sync_stamp_renamed, sync_stamp_tagged, sync_stamp;
DROP TABLE sync_tombstones;
ALTER TABLE subjects DROP COLUMN sync_xid;
ALTER TABLE photos DROP COLUMN sync_xid;
