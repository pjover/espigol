-- +goose Up
ALTER TABLE partner ADD COLUMN nick_name TEXT NOT NULL DEFAULT '';
UPDATE partner SET nick_name = name;

-- +goose Down
ALTER TABLE partner DROP COLUMN nick_name;
