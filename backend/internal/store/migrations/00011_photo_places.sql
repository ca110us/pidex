-- Where a photo was taken, in words (country, region, city, district, a
-- named place), as the phone named its coordinates. Photos are found and
-- grouped by place and day, not only by subject: a trip to Dalian, a site,
-- a client's home. Coordinates are in latitude and longitude already.

-- +goose Up
ALTER TABLE photos ADD COLUMN place jsonb;

-- +goose Down
ALTER TABLE photos DROP COLUMN place;
